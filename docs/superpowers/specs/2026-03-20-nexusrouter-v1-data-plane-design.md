# NexusRouter V1 Data Plane Design

**Date:** 2026-03-20
**Status:** Draft for review
**Scope:** Phase 1 only, focused on the data plane

## 1. Summary

NexusRouter v1 is a single-binary, configuration-driven data-plane router that exposes OpenAI-compatible APIs while routing requests to managed upstream OpenAI and Anthropic providers.

This phase does not build the control plane. There is no dashboard, billing system, user lifecycle, distributed quota store, or semantic cache in v1. The goal is to prove the core router architecture: OpenAI-compatible ingress, managed compatibility enforcement, high-quality streaming, provider translation, health-aware failover, and strong conformance testing.

The v1 contract is intentionally narrower than "proxy anything to any model." NexusRouter accepts only requests that pass a local compatibility gate. Once a request is accepted, NexusRouter guarantees a managed OpenAI-compatible contract across OpenAI and Anthropic upstreams for the supported feature matrix. Unsupported combinations must be rejected before execution rather than attempted as best effort.

## 2. Product Goal

Build a production-oriented first-phase data plane that:

- Serves `POST /v1/chat/completions`
- Serves `POST /v1/responses`
- Authenticates clients using NexusRouter-owned static API keys
- Routes to managed OpenAI and Anthropic upstreams
- Preserves low-latency streaming behavior
- Applies provider health checks, circuit breaking, temporary ejection, and ordered failover
- Presents a strong, explicit compatibility contract rather than ad hoc provider passthrough

## 3. Non-Goals

The following are explicitly out of scope for v1:

- Dashboard, account system, project or organization management
- Billing, balance deduction, recharge, invoice generation
- Tenant-facing RPM or token quota products such as 5-hour or daily policy enforcement
- Distributed quota/rate-limit policy enforcement across instances
- Redis, PostgreSQL, Kafka, ClickHouse, or any required external dependency
- Semantic caching
- Auto-routing by cost/latency preference such as `mode=auto`
- PII masking middleware
- Arbitrary raw provider passthrough for ungoverned model names
- Full vendor-native parity for every OpenAI or Anthropic feature
- Full JSON Schema support for structured outputs
- Stored response-resource retrieval, deletion, or cancel endpoints beyond request creation

V1 may still expose local safety limits such as maximum request body size, maximum image bytes, and per-process concurrency caps. Those are runtime protection controls, not product quota controls.

## 4. Key Decisions Already Locked

### 4.1 Delivery Slice

Phase 1 is the data plane only.

### 4.2 Public API

V1 supports both:

- `POST /v1/chat/completions`
- `POST /v1/responses`

### 4.3 Auth Model

Clients authenticate with NexusRouter-owned static API keys. Valid client keys are mapped to policy entries in local configuration. Upstream provider credentials are never supplied by clients.

### 4.4 Routing Model

Routing is configuration-driven. A visible model family maps to a route group that has a primary upstream and ordered fallbacks. Health checks, circuit breakers, and temporary ejection influence provider selection.

### 4.5 Process Model

V1 is one Go binary with no mandatory external runtime dependency.

### 4.6 Compatibility Model

NexusRouter uses a managed compatibility layer:

- Client-facing models look like provider family names such as `openai/gpt-*` and `anthropic/claude-*`
- Internally, every accepted request must pass a capability registry and normalization pipeline
- OpenAI upstream uses conservative passthrough after validation
- Anthropic upstream uses strict translation from the canonical request model

### 4.7 Consistency Contract

The user-selected contract is strong consistency:

- If NexusRouter accepts a request, the output must conform to the same managed client contract for both OpenAI and Anthropic routes within the supported matrix
- Unsupported or non-verifiable combinations must be rejected locally

## 5. Research Inputs

This design is based on the official provider protocols as they exist on 2026-03-20. The critical implication is that OpenAI `Responses`, OpenAI `Chat Completions`, and Anthropic `Messages` are not naturally isomorphic, especially for streaming, tool use, vision inputs, and structured outputs. NexusRouter therefore cannot achieve a credible "strong consistency" promise without a canonical intermediate representation and an explicit capability matrix.

Relevant official references:

- OpenAI Responses migration guide: <https://developers.openai.com/api/docs/guides/migrate-to-responses>
- OpenAI streaming responses: <https://developers.openai.com/api/docs/guides/streaming-responses>
- OpenAI function calling: <https://developers.openai.com/api/docs/guides/function-calling>
- OpenAI vision: <https://developers.openai.com/api/docs/guides/images-vision>
- OpenAI structured outputs: <https://developers.openai.com/api/docs/guides/structured-outputs>
- Anthropic tool use overview: <https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/overview>
- Anthropic streaming: <https://docs.anthropic.com/en/docs/build-with-claude/streaming>
- Anthropic vision: <https://docs.anthropic.com/en/docs/build-with-claude/vision>

## 6. System Boundary

### 6.1 What V1 Is

NexusRouter v1 is a single-process HTTP service that:

- accepts OpenAI-compatible requests,
- validates them against a managed compatibility contract,
- normalizes them into a canonical internal request model,
- translates them to the selected upstream protocol,
- relays or reshapes streaming events,
- applies health-aware failover before output begins,
- returns OpenAI-compatible responses or errors.

### 6.2 What V1 Is Not

V1 is not:

- a general-purpose reverse proxy,
- a control plane,
- a distributed policy engine,
- a billing pipeline,
- a semantic router,
- a transparent mirror of all raw provider APIs.

## 7. External API Contract

### 7.1 Public Endpoints

Required endpoints:

- `POST /v1/chat/completions`
- `POST /v1/responses`
- `GET /livez`
- `GET /readyz`

Required internal read-only admin endpoints for v1:

- `GET /admin/config`
- `GET /admin/routes`
- `GET /admin/upstreams`

Admin endpoints are for local or restricted-network observability. They are not public control-plane APIs.

### 7.2 Client Authentication

Clients must provide a NexusRouter API key in the standard bearer token position.

The configured client key policy controls:

- whether the key is active,
- which public model patterns are allowed,
- whether streaming is allowed,
- whether tools are allowed,
- whether vision input is allowed,
- whether structured outputs are allowed,
- default request timeout or policy overrides.

### 7.3 Public Model Naming

V1 exposes provider-family-looking names such as:

- `openai/gpt-*`
- `anthropic/claude-*`

These names are managed aliases, not unrestricted raw provider passthrough. A model name being syntactically valid does not guarantee acceptance. The request must still pass capability validation and route-group resolution.

## 8. Managed Compatibility Contract

### 8.1 Contract Rule

The central product rule is:

If NexusRouter accepts a request for execution, the request must already be proven to fit within a supported compatibility slice. Unsupported slices must fail locally with an OpenAI-compatible client error.

NexusRouter must not rely on "try the provider and see what happens" for compatibility-sensitive requests.

### 8.2 Why This Contract Exists

OpenAI `chat.completions`, OpenAI `responses`, and Anthropic `messages` differ in:

- request shape,
- streaming event model,
- tool-call lifecycle,
- image block format,
- structured-output control mechanisms,
- error taxonomy.

Without an explicit compatibility gate, "strong consistency" would be untestable.

### 8.3 Supported Capability Philosophy

V1 supports the largest feature subset that can be validated through a conformance suite. It does not promise support for every provider-native edge case.

When in doubt, the router should reject at validation time rather than silently degrade in a way that breaks the consistency contract.

## 9. Internal Architecture

### 9.1 Execution Pipeline

Every request follows this high-level pipeline:

1. transport accept
2. request ID and log context initialization
3. client API key authentication
4. endpoint-specific decode
5. canonical request normalization
6. capability resolution and local validation
7. route planning against upstream health and breaker state
8. provider-specific request compilation
9. upstream execution
10. canonical event streaming or canonical response aggregation
11. OpenAI-compatible response encoding
12. request finalization with metrics and logs

### 9.2 Required Internal Components

- `config` for startup configuration parsing and validation
- `auth` for client key policy checks
- `canonical` for internal request, response, and event models
- `capabilities` for model-family matching and feature validation
- `router` for route planning and fallback ordering
- `health` for active probes and temporary ejection state
- `circuitbreaker` for local breaker state
- `providers/openai` for OpenAI-native upstream execution
- `providers/anthropic` for Anthropic translation and execution
- `streaming` for canonical event relay and SSE flush behavior
- `httpapi` for endpoint decoding and response encoding
- `admin` for read-only runtime introspection
- `observability` for logs and metrics

### 9.3 Module Boundary Rule

`httpapi`, `canonical`, `capabilities`, and `providers/*` must remain separated.

The HTTP layer must not contain provider translation logic.
Capability filtering must not be embedded in provider adapters.
Provider adapters must not read raw client JSON directly.

## 10. Canonical Request Model

### 10.1 Purpose

NexusRouter needs a provider-neutral internal representation. This is the only credible way to unify:

- `chat.completions`,
- `responses`,
- OpenAI upstream,
- Anthropic upstream,
- streaming and non-streaming paths.

### 10.2 CanonicalRequest Fields

The exact struct names may differ, but the canonical request model must capture at least:

- `endpoint_kind`
- `public_model`
- `conversation`
- `modalities`
- `generation`
- `tools`
- `tool_choice`
- `response_contract`
- `stream`
- `metadata`

### 10.3 Conversation Representation

Conversation is stored as ordered turns with content blocks. The canonical block types for v1 are:

- text
- image
- tool_call
- tool_result

The internal conversation model must preserve order, role, and content-block boundaries across all translations.

### 10.4 Response Contract Representation

`response_contract` defines what the caller expects from generation:

- plain text
- JSON object
- supported JSON Schema subset

This field exists because structured outputs cannot be delegated to provider adapters as an afterthought. They materially change validation, translation, and conformance testing.

## 11. Canonical Streaming Model

### 11.1 Need for a Canonical Event Stream

All streaming responses must pass through a canonical event model before client encoding. Direct provider event passthrough is incompatible with the strong consistency contract.

### 11.2 Minimum Event Types

The canonical stream must represent at least:

- `message_start`
- `content_block_start`
- `content_delta`
- `tool_call_start`
- `tool_call_delta`
- `content_block_stop`
- `message_stop`
- `usage`
- `error`

### 11.3 Downstream Encoding

From the canonical event stream:

- `chat.completions` produces OpenAI chat chunk responses
- `responses` produces OpenAI responses-style stream events
- non-streaming requests aggregate the same canonical events into a final JSON object

This allows one execution core with multiple wire encoders.

## 12. Capability Registry

### 12.1 Role

The capability registry is the enforcement point for the managed compatibility contract.

It maps public model patterns to:

- provider family,
- supported endpoint kinds,
- supported modalities,
- supported tool behavior,
- supported structured output behavior,
- validation constraints,
- normalization rules,
- degradation rules that are explicitly allowed.

The capability registry is owned by code, not arbitrary user configuration. Configuration may enable or disable already-supported public model patterns and bind them to route groups, but configuration must not define new untested compatibility profiles. This keeps the public contract aligned with the conformance suite.

### 12.2 Capability Decisions Are Explicit

For every managed model family, the registry must define:

- what is accepted,
- what is rejected,
- what may be normalized,
- what may be safely degraded,
- what must never be attempted.

### 12.3 Example Capability Dimensions

The registry must be able to express at least:

- `chat_completions`
- `responses`
- streaming
- tools
- tool-choice modes
- vision URL input
- vision base64 input
- JSON object outputs
- JSON Schema subset outputs
- system instructions

### 12.4 Request Validation Rule

No request reaches an upstream adapter unless:

- the public model name matches a configured route group,
- the capability registry accepts the requested feature combination,
- the client key policy permits the requested feature combination,
- the request shape satisfies local safety limits.

## 13. Routing, Health, and Failover

### 13.1 Route Model

Public model patterns map to route groups. Route groups reference a primary upstream and ordered fallbacks.

Routing decisions are deterministic given:

- requested public model,
- route-group configuration,
- client policy,
- health state,
- breaker state,
- temporary ejection state.

In v1, route groups do not cross provider families. A public pattern under `openai/gpt-*` routes only to OpenAI upstream instances, and a public pattern under `anthropic/claude-*` routes only to Anthropic upstream instances. Strong consistency is an API-contract property, not a promise that a provider-family model name can fail over to a different provider family.

### 13.2 Health Checks

V1 runs active health checks against configured upstreams. Probes determine whether an upstream remains eligible for new requests.

Health checks must support:

- probe interval,
- timeout,
- failure threshold,
- temporary ejection duration,
- recovery probe behavior.

### 13.3 Circuit Breakers

V1 uses local in-memory circuit breakers per upstream instance. The breaker must support:

- closed
- open
- half-open

The breaker opens on repeated classified failures. Half-open requests determine recovery. Because v1 is single-process with no external state, breaker state is local to that process only.

### 13.4 Failover Policy

Failover is allowed only before irreversible response output begins.

Retryable failover examples:

- connect failure
- DNS or dial failure
- TLS handshake failure
- upstream timeout before response body output
- upstream `429`
- upstream `502`, `503`, `504`
- upstream disconnect before headers or before response content is emitted

Failover must not happen silently after meaningful output has been flushed to the client.

### 13.5 No Mid-Generation Provider Stitching

Once the client has begun receiving response content, tool-call deltas, or partial structured-output content, NexusRouter must not switch providers and continue the same logical generation. That would violate the strong consistency contract and corrupt response semantics.

If the active upstream fails after output begins:

- the request terminates with an appropriate client-visible error or stream interruption,
- the upstream state is updated for future routing decisions,
- the next request may route elsewhere.

## 14. Provider Adapter Design

### 14.1 OpenAI Upstream

OpenAI upstream should use conservative passthrough after normalization and validation.

The adapter still remains responsible for:

- authentication header injection,
- timeout control,
- error classification,
- stream reading,
- canonical event conversion,
- usage capture,
- final response shaping where required for contract consistency.

### 14.2 Anthropic Upstream

Anthropic upstream is always a translation path from the canonical request model.

It is responsible for:

- system prompt extraction and mapping,
- message content-block translation,
- tool definition translation,
- tool-choice translation,
- vision input translation,
- structured output strategy within the supported subset,
- streaming event conversion into canonical events.

### 14.3 Provider Adapters Must Be Dumb About Policy

Adapters should not decide whether a request is supported. They should assume the request already passed capability validation. Their job is execution, translation, and error reporting.

## 15. API-Specific Mapping Rules

### 15.1 Chat Completions Normalization

`/v1/chat/completions` requests normalize into the canonical model by:

- converting `messages` into ordered conversation turns,
- folding generation options into `generation`,
- mapping `tools` and `tool_choice`,
- mapping `response_format` into `response_contract`,
- normalizing token-limit fields into `max_output_tokens`.

### 15.2 Responses Normalization

`/v1/responses` requests normalize by:

- converting `input` content to the same canonical conversation model,
- preserving multimodal content ordering,
- mapping text or schema requirements into `response_contract`,
- mapping stream behavior into the same canonical streaming pipeline.

### 15.3 Internal Execution Rule

`chat.completions` and `responses` may have separate decoders and encoders, but they must not have separate execution cores. The canonical execution pipeline is shared.

## 16. Vision Contract

V1 supports vision only within a managed contract.

Accepted image inputs may include:

- remote URL images,
- base64 or data-URL images, if enabled for the route and client policy.

Validation must enforce:

- maximum image count,
- maximum total image bytes,
- allowed MIME types,
- order preservation,
- provider-family support constraints.

If a provider family or managed model pattern does not support the requested image combination, NexusRouter must reject locally.

## 17. Tools Contract

V1 supports tools only within the normalized compatibility model.

The contract must define:

- supported tool schema subset,
- supported `tool_choice` modes,
- ordering guarantees for tool-call stream events,
- how tool-call IDs are generated or normalized,
- how tool result turns are represented in canonical conversation state.

For v1, the supported tool schema subset should align with the structured-output subset where practical:

- top-level `type: object`,
- explicit `properties`,
- `required`,
- `description`,
- primitive property types,
- arrays with a single `items` schema,
- nested objects up to a bounded depth,
- enums on scalar fields.

The following should be rejected in v1:

- recursive schemas,
- `oneOf`, `anyOf`, `allOf`, `not`,
- `patternProperties`,
- unconstrained additional object shapes,
- deep polymorphism that cannot be translated deterministically across providers.

Parallel tool calls may be represented in the capability registry but should only be enabled where conformance tests prove stable cross-provider behavior.

## 18. Structured Outputs Contract

### 18.1 Core Principle

V1 must not claim support for arbitrary JSON Schema.

### 18.2 Supported Scope

V1 supports a restricted JSON Schema subset that is:

- validated locally,
- translated deterministically,
- covered by conformance tests against both OpenAI and Anthropic routes.

The minimum supported subset for v1 should be:

- top-level `type: object`,
- explicit `properties`,
- `required`,
- `description`,
- scalar leaf types,
- arrays with a single concrete `items` schema,
- nested objects up to a bounded depth,
- scalar `enum`.

The following are out of scope and must be rejected:

- recursive refs,
- `oneOf`, `anyOf`, `allOf`, `not`,
- `patternProperties`,
- unbounded `additionalProperties`,
- schema constructs that require provider-specific interpretation to preserve semantics.

### 18.3 Rejection Rule

If the requested schema exceeds the supported subset, NexusRouter rejects the request before provider execution.

This restriction is required because provider-native structured-output controls are not fully isomorphic.

## 19. Error Model

NexusRouter must normalize internal and provider errors into a stable OpenAI-compatible error shape.

At minimum, internal classification should distinguish:

- authentication errors,
- invalid request errors,
- unsupported capability errors,
- upstream rate-limit errors,
- upstream unavailable errors,
- upstream timeout errors,
- stream interruption errors,
- internal router errors.

Provider-native error payloads may be logged internally, but the public client contract must be normalized.

## 20. Configuration Design

### 20.1 Format

V1 uses a YAML configuration file. Environment variables may supply secrets such as upstream API keys, but the configuration file defines the model, route, and policy graph.

### 20.2 Top-Level Sections

The configuration must be able to represent:

- `server`
- `auth`
- `models`
- `providers`
- `routing`
- `health`
- `breaker`
- `limits`
- `observability`
- `compatibility`

### 20.3 Model Exposure Separation

Public model patterns must remain separate from upstream instance definitions.

This means:

- `models` decides what public patterns exist,
- `routing` decides how accepted patterns map to route groups,
- `providers` defines actual upstream instances,
- `health` and `breaker` decide runtime eligibility.

This avoids mixing public contract decisions with operational endpoint wiring.

Configuration also must not invent new capability semantics. It can only select from capability profiles compiled into the binary and validated by tests.

### 20.4 Client Key Policy

Each configured client key should define:

- key ID,
- key material or hash reference,
- active status,
- allowed public model patterns,
- feature permissions such as tools, vision, structured outputs, and streaming,
- default timeout or policy overrides.

## 21. Runtime Interfaces

### 21.1 Public Listener

The service must support:

- plain HTTP mode,
- HTTPS termination mode via configuration.

This allows local development and direct deployment in environments that either do or do not place a reverse proxy in front of the router.

### 21.2 Readiness Semantics

`/readyz` should reflect whether:

- configuration loaded successfully,
- at least one eligible upstream exists for required route groups,
- critical internal subsystems have started.

`/livez` should be a simpler process-health signal.

### 21.3 Admin Semantics

Read-only admin endpoints should expose:

- current config summary,
- public model pattern to route-group mapping,
- upstream health state,
- breaker state,
- temporary ejection state,
- compatibility summaries.

## 22. Observability

V1 must emit structured logs with request context and upstream routing decisions.

Minimum observability requirements:

- request ID on every request,
- selected public model and route group,
- selected upstream and failover attempts,
- breaker transitions,
- health probe transitions,
- client-visible error classification,
- stream interruption events.

Metrics and pprof may be included if they do not distort v1 scope, but logs and admin inspection are mandatory.

## 23. Repository and Code Layout

Recommended structure:

- `cmd/nexus-router/main.go`
- `internal/app/`
- `internal/config/`
- `internal/httpapi/`
- `internal/httpapi/handlers/`
- `internal/httpapi/openai/`
- `internal/auth/`
- `internal/canonical/`
- `internal/capabilities/`
- `internal/router/`
- `internal/health/`
- `internal/circuitbreaker/`
- `internal/providers/openai/`
- `internal/providers/anthropic/`
- `internal/streaming/`
- `internal/usage/`
- `internal/admin/`
- `internal/observability/`
- `configs/nexus-router.example.yaml`
- `tests/integration/`
- `tests/e2e/`

Exact file names may evolve during implementation planning, but the responsibility boundaries should remain.

## 24. Testing Strategy

### 24.1 Test Layers

V1 requires four test layers:

- unit tests,
- adapter tests,
- integration tests with provider stubs,
- conformance tests.

### 24.2 Provider Stubs

The test suite must not depend on real OpenAI or Anthropic APIs.

At minimum, provider stubs must simulate:

- normal JSON responses,
- normal SSE responses,
- upstream timeouts,
- `429`,
- `5xx`,
- disconnects before headers,
- disconnects after partial output,
- tool-call streaming,
- vision request acceptance,
- structured-output responses within the supported subset.

### 24.3 Conformance Tests

Conformance tests are the proof mechanism for the strong consistency contract.

They should execute equivalent canonical scenarios through:

- OpenAI-routed execution,
- Anthropic-routed execution,

and compare the client-visible result for:

- response shape,
- streaming event order,
- finish reason semantics,
- tool-call structure,
- supported structured-output contract,
- normalized error categories.

The tests do not need to assert identical generated text. They must assert equivalent client-visible semantics.

## 25. Completion Criteria

V1 is complete only when all of the following are true:

- the service boots from a documented example config,
- `POST /v1/chat/completions` works,
- `POST /v1/responses` works,
- OpenAI upstream execution works,
- Anthropic translation execution works,
- text, tools, vision, and supported structured outputs have explicit acceptance and rejection behavior,
- streaming works for both public endpoints,
- health checks, breakers, temporary ejection, and ordered fallback work,
- plain HTTP and HTTPS modes both work,
- read-only admin inspection endpoints work,
- conformance tests pass for the supported matrix.

An implementation that only proxies a few happy-path requests is not considered complete.

## 26. Delivery Phases for Implementation

Implementation should proceed in slices that each produce runnable, testable software:

1. bootstrap service, config loading, auth, health endpoints, admin skeleton
2. `chat.completions` with OpenAI upstream and streaming
3. `responses` with OpenAI upstream
4. Anthropic text translation path
5. tools and vision translation path
6. structured-output subset
7. conformance suite and hardening of failover behavior

## 27. Risks and Design Constraints

Primary v1 risks:

- over-claiming compatibility beyond what can be tested,
- accidental divergence between `chat.completions` and `responses`,
- leaking provider-native event semantics into the public contract,
- failover after output begins,
- letting provider adapter code absorb policy logic that belongs in capability validation,
- trying to support full JSON Schema too early.

The plan for implementation must preserve YAGNI: build only the supported matrix needed for the managed contract and reject the rest explicitly.

## 28. Planning Handoff

The implementation plan must assume:

- one repo,
- work on `main` is allowed,
- no external dependencies are required for the first runnable system,
- tests rely on local stubs,
- the compatibility matrix is part of the product, not documentation garnish.

The plan should decompose work into test-first slices and keep provider translation, capability validation, and streaming mechanics independently testable.
