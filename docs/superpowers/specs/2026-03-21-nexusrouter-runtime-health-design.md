# NexusRouter Runtime Health and Probe Design

**Date:** 2026-03-21
**Status:** Draft for review
**Scope:** Next mainline slice after the public text API online path
**Depends on:** `docs/superpowers/specs/2026-03-20-nexusrouter-v1-data-plane-design.md`, `docs/superpowers/specs/2026-03-21-nexusrouter-public-text-api-design.md`

## 1. Summary

The next mainline slice is to turn NexusRouter's current passive failover foundation into an active runtime health loop.

At the end of this slice, NexusRouter must:

- actively probe configured upstreams in the background
- keep a unified in-memory health snapshot per upstream
- use that snapshot for route eligibility
- expose runtime health through `/readyz` and `/admin/upstreams`
- short-eject unhealthy upstreams and recover them through half-open probe trials
- combine probe results and real request execution feedback into one state machine

This slice stays inside the current single-process, single-binary architecture. It does not introduce Redis, PostgreSQL, distributed coordination, or a control plane.

## 2. Goal

Build a runtime health subsystem that makes upstream failover proactive, observable, and testable without changing NexusRouter's single-node in-memory operating model.

## 3. Non-Goals

This slice does not:

- expose public tools, vision, or structured outputs
- add distributed health state or multi-instance coordination
- add metrics exporters, dashboards, or alerting systems
- add manual admin mutation APIs for force-eject or force-recover
- add quota, billing, session stickiness, semantic caching, or passthrough mode
- redesign the whole service into a separate runtime supervisor architecture
- replace the current planner/orchestrator/provider adapter split

## 4. Fixed Scope Decisions

The following decisions are already locked for this slice:

- Health checks use lightweight HTTP probes, not real inference probes
- Upstreams are short-ejected after consecutive failures, then recovered through half-open probe trials
- `/readyz` reflects runtime health, not just successful startup
- A new read-only `/admin/upstreams` endpoint exposes runtime health state
- Probe configuration is overrideable per upstream, with provider-family defaults
- The slice lands as an incremental extension of the current single-process service, while keeping interface boundaries clean enough for later supervisor-style refactoring

## 5. Problem Statement

The current codebase already has the foundations for failover:

- route planning with ordered fallbacks
- orchestration boundaries that stop failover after output commitment
- an in-memory `health.Manager`
- an in-memory `circuitbreaker`
- admin and readiness endpoints

But those parts are not yet connected into a real runtime loop:

- health probing is not implemented
- readiness does not reflect actual upstream availability
- admin surfaces do not show runtime health
- route eligibility is not driven by a continuously updated health snapshot
- real request failures and background health checks do not converge on one state machine

This makes failover functional but still largely reactive and opaque.

## 6. Architecture Overview

### 6.1 Runtime Health Model

This slice introduces a single runtime health model shared by:

- the background prober
- real request execution feedback
- route eligibility checks
- `/readyz`
- `/admin/upstreams`

The key design rule is:

> There must be one runtime truth per upstream, consumed through one read model and updated through a small set of write events.

### 6.2 Incremental Architecture Choice

The implementation follows the existing single-process design rather than introducing a full supervisor layer.

However, it borrows one constraint from a future supervisor architecture:

- planner, readiness, and admin consumers read through a unified runtime interface
- they do not inspect probe internals or infer health separately

This keeps the current slice small while reducing the chance of health logic fragmenting across packages.

### 6.3 Main Components

The runtime health subsystem is composed of:

- `health.Runtime`
  - owns per-upstream in-memory state
  - accepts normalized success/failure events
  - exposes eligibility and snapshot reads
- `health.Prober`
  - background loop that periodically probes upstreams
  - writes probe outcomes into `health.Runtime`
- `router/planner`
  - consumes only `IsEligible(upstream)`
- `http handlers`
  - consume `Snapshot()` for `/readyz` and `/admin/upstreams`
- `service` or `runtime executor`
  - writes real request success/failure feedback into `health.Runtime`

## 7. Component Boundaries

### 7.1 `health.Runtime`

`health.Runtime` is the sole owner of upstream runtime state.

Responsibilities:

- store the latest health state per upstream
- track consecutive failures
- track ejection windows
- track breaker-like open and half-open status
- remember probe timestamps and error summaries
- return consistent read snapshots
- answer route eligibility with one method

Non-responsibilities:

- building HTTP probe requests
- scheduling probe intervals
- selecting route attempts
- encoding admin HTTP responses

### 7.2 `health.Prober`

`health.Prober` is a scheduling and probing component.

Responsibilities:

- build effective probe config per upstream
- run periodic probe loops
- ensure a single in-flight probe per upstream
- push probe success/failure events into `health.Runtime`
- mark the initial probe sweep complete

Non-responsibilities:

- route planning
- readiness decisions
- admin JSON formatting
- direct breaker policy evaluation outside the runtime write APIs

### 7.3 `router/planner`

Planner remains thin.

Responsibilities:

- ask whether an upstream is eligible
- skip upstreams that are not eligible
- preserve the configured primary and fallback ordering among eligible upstreams

Planner must not know:

- probe timing
- ejection reasons
- failure counters
- admin state

### 7.4 HTTP Handlers

`/readyz` and `/admin/upstreams` are read-only consumers of the runtime snapshot.

They must not:

- run probes inline
- mutate health state
- compute eligibility on their own
- expose sensitive upstream secrets or credentials

## 8. Configuration Model

### 8.1 Global Health Defaults

The existing `health` section becomes the home of global probe defaults:

- `probe_interval`
- `probe_timeout`
- `require_initial_probe`

Recommended defaults:

- `probe_interval: 15s`
- `probe_timeout: 2s`
- `require_initial_probe: true`

### 8.2 Breaker Defaults

The existing `breaker` section continues to define failure and recovery timing:

- `failure_threshold`
- `open_interval`
- `recovery_success_threshold`

Recommended defaults:

- `failure_threshold: 3`
- `open_interval: 30s`
- `recovery_success_threshold: 1`

`recovery_success_threshold` defines how many consecutive successful half-open probes are required before an upstream returns to `healthy`.

Examples:

- `1`
  - the first successful half-open probe restores `healthy`
- `2`
  - the upstream stays `half_open` after the first success and only restores `healthy` after the second consecutive successful half-open probe

This threshold applies to probe-based half-open recovery in this slice.

### 8.3 Per-Upstream Probe Overrides

Each provider entry may define an optional `probe` block:

```yaml
providers:
  - name: openai-main
    provider: openai
    base_url: https://api.openai.com
    api_key_env: OPENAI_API_KEY
    probe:
      method: GET
      path: /v1/models
      expected_statuses: [200]
      interval: 10s
      timeout: 2s
      headers:
        OpenAI-Organization: org_xxx
```

Supported fields:

- `method`
- `path`
- `headers`
- `expected_statuses`
- `interval`
- `timeout`

This slice does not support disabling probes per upstream. If an upstream exists in configuration, it participates in the runtime health loop.

### 8.4 Provider-Family Defaults

If an upstream does not define a probe block, runtime falls back to provider-family defaults compiled into the binary.

Recommended defaults:

- OpenAI family
  - `GET /v1/models`
  - `Authorization: Bearer <key>`
- Anthropic family
  - `GET /v1/models`
  - `x-api-key: <key>`
  - `anthropic-version: 2023-06-01`

The provider family decides the default probe contract. Configuration may override it per upstream when needed.

### 8.5 Configuration Rules

This slice keeps configuration intentionally narrow:

- probe settings can customize transport details
- probe settings cannot define new health semantics
- breaker timing still comes from the global breaker policy
- all runtime state remains in memory

## 9. State Model

### 9.1 Runtime States

Each upstream may be in one of four states:

- `unknown`
- `healthy`
- `open`
- `half_open`

Definitions:

- `unknown`
  - runtime has started but no decisive probe result exists yet
- `healthy`
  - upstream is eligible for routing
- `open`
  - upstream is temporarily ejected
- `half_open`
  - cooldown expired, but only the next probe may test recovery

### 9.2 Eligibility Rule

Only `healthy` is route-eligible.

The following states are not route-eligible:

- `unknown`
- `open`
- `half_open`

This avoids putting real user traffic through speculative recovery attempts.

### 9.3 State Transitions

The runtime transitions follow this model:

- startup
  - upstreams begin in `unknown`
- probe success
  - `unknown -> healthy`
  - `half_open -> healthy`
- failure threshold reached
  - `healthy -> open`
  - `unknown -> open`
- cooldown expiry
  - `open -> half_open`
- half-open probe failure
  - `half_open -> open`

### 9.4 Half-Open Policy

Half-open recovery uses probe traffic only.

Real user requests are not sent to half-open upstreams in this slice.

That choice is intentional because it:

- keeps planner behavior deterministic
- avoids using user traffic as a canary
- aligns recovery with the active health-check model

This is an intentional refinement of the earlier passive failover assumption from the 2026-03-20 design. For this slice, probe-driven half-open recovery is the authoritative rule.

## 10. Runtime Write Events

The runtime must accept normalized write events from two sources:

- probes
- real requests

Recommended write API shape:

- `RecordProbeSuccess(upstream, at)`
- `RecordProbeFailure(upstream, at, errSummary)`
- `RecordRequestSuccess(upstream, at)`
- `RecordRequestFailure(upstream, at, retryable, outputCommitted, errSummary)`

This design keeps the state machine centralized while allowing multiple event producers.

## 11. Probe Loop Semantics

### 11.1 Scheduling

The prober must:

- run independently of the public request path
- maintain one loop per configured upstream
- prevent overlapping probes for the same upstream
- respect per-upstream effective interval and timeout

### 11.2 Probe Evaluation

A probe counts as success when:

- the request completes before timeout
- the status code is within `expected_statuses`

A probe counts as failure when:

- the request times out
- DNS/TCP/TLS/HTTP transport fails
- the returned status code is unexpected

### 11.3 Initial Probe Sweep

If `require_initial_probe=true`, the service is not ready until the first complete probe sweep finishes.

This means:

- startup success alone is not enough for readiness
- readiness reflects actual runtime reachability

## 12. Real Request Feedback Rules

### 12.1 Request Success

Successful real execution updates runtime state by:

- clearing consecutive failure count
- refreshing the last-success information
- optionally restoring `healthy` when the upstream is not currently ejected

### 12.2 Request Failure

Real request failures do not all have the same runtime meaning.

Failures should only count against upstream health when:

- the failure is retryable
- output has not yet been committed

This matches the existing orchestrator failover boundary:

- pre-output retryable failure means the upstream may really be unavailable
- post-output failure must not trigger unsafe failover

### 12.3 Post-Output Failures

Failures after output commitment should:

- update last error information
- remain visible in logs and runtime snapshot
- not advance the consecutive-failure state machine used for ejection

This prevents transient stream interruptions from over-ejecting an otherwise healthy upstream.

### 12.4 Non-Retryable Failures

Non-retryable request failures should not count toward health ejection by default.

Examples:

- bad upstream request format
- client-caused incompatibility
- other request-specific 4xx-style failures

These indicate contract mismatch, not necessarily upstream unavailability.

## 13. Read Model

### 13.1 Eligibility Interface

The runtime must expose:

- `IsEligible(upstream string) bool`

This is the only interface planner needs for runtime health decisions.

### 13.2 Snapshot Interface

The runtime must also expose:

- `Snapshot() RuntimeSnapshot`

`RuntimeSnapshot` should contain:

- `started`
- `initial_probe_complete`
- `has_eligible_upstream`
- `upstreams`

Each upstream entry should include:

- `name`
- `provider`
- `state`
- `eligible`
- `consecutive_failures`
- `ejected_until`
- `last_probe_at`
- `last_probe_ok`
- `last_error`
- `breaker_state`
- `source`

Where `source` identifies whether the latest state transition was driven primarily by:

- `startup`
- `probe`
- `request`

`state` is the human-facing runtime state and uses:

- `unknown`
- `healthy`
- `open`
- `half_open`

`breaker_state` is the narrower breaker-style view and uses:

- `closed`
- `open`
- `half_open`

Recommended mapping:

- `state=unknown -> breaker_state=closed`
- `state=healthy -> breaker_state=closed`
- `state=open -> breaker_state=open`
- `state=half_open -> breaker_state=half_open`

## 14. HTTP Surface Semantics

### 14.1 `/readyz`

`/readyz` answers one question only:

> Can this instance currently serve real traffic for every configured route group that the service is expected to support?

Return `200` only when:

- runtime has started
- if required, the initial probe sweep has completed
- for every route group referenced by configured public model patterns, at least one upstream in that group is currently eligible

Otherwise return `503`.

### 14.2 `/admin/upstreams`

Add a new read-only endpoint:

- `GET /admin/upstreams`

It should return the current runtime snapshot in a stable JSON shape suitable for:

- manual debugging
- test assertions
- lightweight operational inspection

It must not expose:

- upstream API keys
- `api_key_env`
- full authorization headers
- other sensitive secrets

### 14.3 `/admin/routes`

The existing `/admin/routes` remains a route summary endpoint.

This slice does not merge route configuration and runtime health into one response.

The route-group-based readiness contract from the 2026-03-20 data-plane design remains in force. This spec refines how eligibility is computed, not what readiness means operationally.

## 15. File Plan

Expected new files:

- `internal/health/runtime.go`
- `internal/health/types.go`

Expected modified files:

- `internal/health/prober.go`
- `internal/httpapi/handlers/health.go`
- `internal/httpapi/handlers/admin.go`
- `internal/httpapi/router.go`
- `internal/app/app.go`
- `internal/router/planner.go`
- `internal/runtime/executor.go` and/or `internal/service/execute.go`
- config types and validation files as needed for probe configuration
- integration/e2e tests covering readiness, admin snapshot, probe-driven ejection, and recovery

## 16. Test Strategy

### 16.1 Unit Coverage

Unit tests should cover:

- runtime state transitions
- failure-threshold opening
- cooldown and half-open recovery
- eligibility semantics
- snapshot stability

### 16.2 Integration and E2E Coverage

Integration or e2e tests should prove:

- successful initial probe leads to readiness
- all-upstream failure leads to `503 /readyz`
- `/admin/upstreams` exposes runtime state
- planner skips ejected upstreams
- fallback occurs when the primary is ejected
- cooldown plus successful probe returns an upstream to eligibility
- pre-output retryable request failures contribute to ejection
- post-output failures do not incorrectly trigger route-wide ejection

### 16.3 Regression Requirements

This slice must preserve:

- current public text API behavior
- existing orchestrator failover boundaries
- current single-process deployment mode

## 17. Risks

Primary risks in this slice:

- letting probe logic and request-feedback logic diverge
- duplicating health interpretation across planner, readiness, and admin handlers
- over-ejecting upstreams on stream failures after output commitment
- accidentally exposing sensitive upstream config on admin endpoints
- turning this health slice into a premature runtime architecture rewrite

## 18. Completion Criteria

This slice is complete only when all of the following are true:

- background probes actively run against configured upstreams
- runtime health is stored in one in-memory model
- planner route eligibility is driven by runtime health
- `/readyz` reflects runtime upstream availability
- `/admin/upstreams` exposes stable runtime health state
- unhealthy upstreams are short-ejected after consecutive failures
- recovery occurs through half-open probe trials
- real request pre-output retryable failures influence runtime health
- post-output failures do not cause incorrect proactive ejection
- `go test ./...` continues to pass

## 19. Planning Handoff

The implementation plan for this slice should:

- keep the single-binary architecture intact
- add the smallest possible runtime health model that unifies probe and request feedback
- keep planner consumption narrow through `IsEligible`
- keep readiness and admin views snapshot-based
- introduce probe configuration without opening the door to arbitrary health semantics
- land tests early so runtime state behavior is validated before broad integration
