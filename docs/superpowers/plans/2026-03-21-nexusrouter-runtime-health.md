# NexusRouter Runtime Health Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add active upstream probing, unified in-memory runtime health state, route eligibility, runtime-aware readiness, and `/admin/upstreams` visibility to NexusRouter without leaving the current single-process architecture.

**Architecture:** This slice adds a new `health.Runtime` read/write model and a real `health.Prober`, then wires planner, handlers, and request execution feedback to consume or update that shared runtime state. The implementation keeps the current app/router/service stack intact while replacing passive, fragmented health behavior with one snapshot-driven contract.

**Tech Stack:** Go, standard library `net/http`, `time`, `context`, `sync`, `httptest`, existing `config`, `health`, `router`, `service`, `runtime`, `providers`

---

## Planned File Structure

### Runtime health core

- Create: `internal/health/types.go`
- Create: `internal/health/runtime.go`
- Modify: `internal/health/prober.go`

### Config and boot wiring

- Modify: `internal/config/config.go`
- Modify: `internal/config/validate.go`
- Create: `internal/config/validate_test.go`
- Modify: `internal/app/app.go`
- Modify: `internal/app/server.go`
- Modify: `configs/nexus-router.example.yaml`

### HTTP and planner consumption

- Modify: `internal/httpapi/handlers/health.go`
- Modify: `internal/httpapi/handlers/admin.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/router/planner.go`

### Runtime feedback integration

- Modify: `internal/runtime/executor.go`
- Modify: `internal/service/execute.go`

### Test coverage

- Create: `internal/health/runtime_test.go`
- Create: `internal/health/prober_test.go`
- Modify: `tests/e2e/http_test_helpers.go`
- Create: `tests/e2e/health_http_test.go`
- Create: `tests/e2e/runtime_feedback_test.go`
- Modify: `tests/integration/health_admin_test.go`
- Modify: `tests/integration/example_config_test.go`

## Responsibility Map

- `internal/config/*`: parse and validate probe and recovery settings.
- `internal/health/runtime.go`: own upstream state machine, snapshot view, and eligibility.
- `internal/health/prober.go`: run lightweight HTTP probes and write outcomes into runtime.
- `internal/httpapi/handlers/*`: expose readiness and admin snapshot from runtime without re-deriving state.
- `internal/runtime/executor.go` and `internal/service/execute.go`: write request success/failure feedback into runtime.
- `tests/e2e/*` and `internal/health/*_test.go`: prove runtime transitions, readiness behavior, admin JSON, and request-feedback-driven ejection.

### Task 1: Add Probe Config Schema and Validation

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/validate.go`
- Create: `internal/config/validate_test.go`
- Modify: `configs/nexus-router.example.yaml`
- Modify: `tests/integration/example_config_test.go`

- [ ] **Step 1: Write the failing config tests**

```go
func TestValidateAcceptsProviderProbeOverrides(t *testing.T) {
	cfg := Config{
		Auth: AuthConfig{ClientKeys: []ClientKeyConfig{{ID: "dev", Secret: "sk", Active: true}}},
		Models: []ModelConfig{{Pattern: "openai/gpt-*", RouteGroup: "openai-family"}},
		Providers: []ProviderConfig{{
			Name: "openai-main", Provider: "openai", BaseURL: "https://api.openai.com",
			Probe: ProbeConfig{
				Method: "GET",
				Path: "/v1/models",
				ExpectedStatuses: []int{200},
				Interval: "10s",
				Timeout: "2s",
			},
		}},
		Routing: RoutingConfig{RouteGroups: []RouteGroupConfig{{Name: "openai-family", Primary: "openai-main"}}},
		Health: HealthConfig{ProbeInterval: "15s", ProbeTimeout: "2s", RequireInitialProbe: true},
		Breaker: BreakerConfig{FailureThreshold: 3, OpenInterval: "30s", RecoverySuccessThreshold: 1},
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
```

- [ ] **Step 2: Run config tests to verify failure**

Run: `go test ./internal/config -run 'TestValidateAcceptsProviderProbeOverrides|TestValidateRejectsInvalidProbeMethod|TestValidateRejectsRecoverySuccessThresholdZero' -count=1`
Expected: FAIL because `ProbeConfig`, `RequireInitialProbe`, and `RecoverySuccessThreshold` do not exist yet

- [ ] **Step 3: Extend config types**

Add exact fields:

```go
type ProbeConfig struct {
	Method           string            `yaml:"method"`
	Path             string            `yaml:"path"`
	Headers          map[string]string `yaml:"headers"`
	ExpectedStatuses []int             `yaml:"expected_statuses"`
	Interval         string            `yaml:"interval"`
	Timeout          string            `yaml:"timeout"`
}
```

Extend:

```go
type ProviderConfig struct {
	...
	Probe ProbeConfig `yaml:"probe"`
}

type HealthConfig struct {
	ProbeInterval       string `yaml:"probe_interval"`
	ProbeTimeout        string `yaml:"probe_timeout"`
	RequireInitialProbe bool   `yaml:"require_initial_probe"`
}

type BreakerConfig struct {
	FailureThreshold         int    `yaml:"failure_threshold"`
	OpenInterval             string `yaml:"open_interval"`
	RecoverySuccessThreshold int    `yaml:"recovery_success_threshold"`
}
```

- [ ] **Step 4: Add validation rules**

Validation must cover:

- `failure_threshold >= 1`
- `recovery_success_threshold >= 1`
- duration strings in `health` and per-provider `probe` parse successfully when present
- `probe.method` is either empty or a supported HTTP method
- `probe.path`, if set, starts with `/`
- `expected_statuses`, if set, only contains valid HTTP status codes

- [ ] **Step 5: Update example config**

Add:

```yaml
health:
  probe_interval: 5s
  probe_timeout: 1s
  require_initial_probe: true

breaker:
  failure_threshold: 3
  open_interval: 30s
  recovery_success_threshold: 1
```

- [ ] **Step 6: Run config and example boot tests**

Run: `go test ./internal/config ./tests/integration -run 'TestValidate|TestExampleConfigBoots' -count=1`
Expected: PASS

- [ ] **Step 7: Commit the config slice**

```bash
git add internal/config configs/nexus-router.example.yaml tests/integration/example_config_test.go
git commit -m "feat: add runtime health probe config"
```

### Task 2: Add the Runtime Health State Model

**Files:**
- Create: `internal/health/types.go`
- Create: `internal/health/runtime.go`
- Create: `internal/health/runtime_test.go`

- [ ] **Step 1: Write the failing runtime state tests**

```go
func TestRuntimeOpensAfterFailureThreshold(t *testing.T) {
	rt := NewRuntime(RuntimeOptions{
		FailureThreshold:         3,
		RecoverySuccessThreshold: 1,
		OpenInterval:             30 * time.Second,
		Now: func() time.Time { return fixed },
	})

	rt.RecordProbeFailure("openai-main", fixed, "timeout")
	rt.RecordProbeFailure("openai-main", fixed, "timeout")
	rt.RecordProbeFailure("openai-main", fixed, "timeout")

	snap := rt.Snapshot()
	if snap.Upstreams[0].State != StateOpen {
		t.Fatalf("state = %q, want %q", snap.Upstreams[0].State, StateOpen)
	}
}
```

- [ ] **Step 2: Run runtime tests to verify failure**

Run: `go test ./internal/health -run 'TestRuntime' -count=1`
Expected: FAIL because `Runtime` does not exist yet

- [ ] **Step 3: Define shared runtime types**

In `types.go`, add:

- `State` enum: `unknown`, `healthy`, `open`, `half_open`
- `BreakerState` enum: `closed`, `open`, `half_open`
- `Source` enum: `startup`, `probe`, `request`
- `RuntimeSnapshot`
- `UpstreamStatus`

- [ ] **Step 4: Implement `health.Runtime`**

`Runtime` must support:

- `IsEligible(upstream string) bool`
- `Snapshot() RuntimeSnapshot`
- `MarkStarted()`
- `MarkInitialProbeComplete()`
- `RecordProbeSuccess(...)`
- `RecordProbeFailure(...)`
- `RecordRequestSuccess(...)`
- `RecordRequestFailure(...)`

Keep state machine behavior aligned with the spec:

- only `healthy` is eligible
- failure threshold transitions to `open`
- cooldown expiry transitions `open -> half_open`
- half-open success count must honor `recovery_success_threshold`
- post-output request failures must not advance ejection state

- [ ] **Step 5: Pin the admin snapshot contract in tests**

Test exact field semantics for:

- `state`
- `breaker_state`
- `source`
- `eligible`
- `ejected_until`

- [ ] **Step 6: Run runtime state tests**

Run: `go test ./internal/health -run 'TestRuntime' -count=1`
Expected: PASS

- [ ] **Step 7: Commit the runtime slice**

```bash
git add internal/health/types.go internal/health/runtime.go internal/health/runtime_test.go
git commit -m "feat: add runtime health state model"
```

### Task 3: Implement the Background Prober and Provider Defaults

**Files:**
- Modify: `internal/health/prober.go`
- Create: `internal/health/prober_test.go`
- Modify: `internal/app/app.go`
- Modify: `internal/app/server.go`

- [ ] **Step 1: Write the failing prober tests**

```go
func TestProberMarksInitialSweepCompleteAfterAllUpstreamsRun(t *testing.T) {
	rt := NewRuntime(...)
	prober := NewProber(rt, []config.ProviderConfig{
		{Name: "openai-main", Provider: "openai", BaseURL: stubURL, APIKeyEnv: "OPENAI_API_KEY"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go prober.Run(ctx)

	waitForInitialSweep(t, rt)

	if !rt.Snapshot().InitialProbeComplete {
		t.Fatal("InitialProbeComplete = false, want true")
	}
}
```

- [ ] **Step 2: Run prober tests to verify failure**

Run: `go test ./internal/health -run 'TestProber' -count=1`
Expected: FAIL because `Prober` is still empty

- [ ] **Step 3: Implement effective probe configuration**

Rules:

- every configured upstream participates
- per-provider `Probe` values override defaults
- provider-family default auth/version headers merge with custom headers
- custom headers must augment non-sensitive defaults rather than remove required auth headers

Recommended defaults:

- OpenAI: `GET /v1/models`
- Anthropic: `GET /v1/models` with `anthropic-version: 2023-06-01`

- [ ] **Step 4: Implement the probe loop**

The prober must:

- run one loop per upstream
- prevent overlapping probes per upstream
- use context cancellation for shutdown
- mark runtime started
- mark initial sweep complete after every upstream has at least one probe result

- [ ] **Step 5: Wire the prober into app startup dependencies**

`app.New()` should build:

- runtime health state
- planner with runtime eligibility
- runtime executor / service with runtime feedback hooks
- prober instance owned by the service
- a service-owned context/cancel pair for background runtime work

Because current tests and helper flows call `app.New()` and then use `Handler()` directly, the service must make runtime probing active immediately after construction. The plan should therefore:

- start the prober background loop from `app.New()`
- store the cancel function on `Service`
- stop the loop in `Service.Shutdown()`

This is the authoritative lifecycle rule for this slice. Do not defer probe startup to `Serve()` only, or existing `Handler()`-based flows will observe an unstarted runtime.

- [ ] **Step 6: Run prober tests**

Run: `go test ./internal/health -run 'TestProber' -count=1`
Expected: PASS

- [ ] **Step 7: Commit the prober slice**

```bash
git add internal/health/prober.go internal/health/prober_test.go internal/app/app.go internal/app/server.go
git commit -m "feat: add background upstream prober"
```

### Task 4: Expose Runtime Health Through `/readyz` and `/admin/upstreams`

**Files:**
- Modify: `internal/httpapi/handlers/health.go`
- Modify: `internal/httpapi/handlers/admin.go`
- Modify: `internal/httpapi/router.go`
- Modify: `tests/e2e/http_test_helpers.go`
- Create: `tests/e2e/health_http_test.go`
- Modify: `tests/integration/health_admin_test.go`

- [ ] **Step 1: Write the failing HTTP health/admin tests**

```go
func TestReadyzReturnsUnavailableUntilInitialProbeCompletes(t *testing.T) {
	env := startHealthHTTPTestEnv(t, "initial_probe_pending")
	defer env.Close()

	resp := get(t, env.Client, env.BaseURL+"/readyz")
	assertStatus(t, resp, http.StatusServiceUnavailable)
}
```

- [ ] **Step 2: Run health/admin tests to verify failure**

Run: `go test ./tests/e2e ./tests/integration -run 'TestReadyz|TestAdminUpstreams' -count=1`
Expected: FAIL because handlers do not consume runtime state yet

- [ ] **Step 3: Implement runtime-aware `/readyz`**

Readiness rules:

- `200` only when runtime started, initial probe complete if required, and every required route group has an eligible upstream
- `503` otherwise

Implementation note:

- derive route-group readiness from `Snapshot().Upstreams` joined with configured route-group membership from `cfg.Models`, `cfg.Routing.RouteGroups`, and `cfg.Providers`
- do not treat the top-level `has_eligible_upstream` snapshot flag as sufficient for `/readyz`

- [ ] **Step 4: Add `/admin/upstreams`**

Pin exact JSON field names in both handler and tests:

```json
{
  "started": true,
  "initial_probe_complete": true,
  "has_eligible_upstream": true,
  "upstreams": [...]
}
```

Assert every spec-required upstream field in tests:

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

- [ ] **Step 5: Keep `/admin/routes` unchanged in responsibility**

Do not merge route summary and runtime health into a single endpoint.

- [ ] **Step 6: Run health/admin tests**

Run: `go test ./tests/e2e ./tests/integration -run 'TestReadyz|TestAdminUpstreams|TestAdminRoutes' -count=1`
Expected: PASS

- [ ] **Step 7: Commit the HTTP surface slice**

```bash
git add internal/httpapi/handlers/health.go internal/httpapi/handlers/admin.go internal/httpapi/router.go tests/e2e/http_test_helpers.go tests/e2e/health_http_test.go tests/integration/health_admin_test.go
git commit -m "feat: expose runtime health through readyz and admin"
```

### Task 5: Feed Real Request Outcomes Back Into Runtime Health

**Files:**
- Modify: `internal/runtime/executor.go`
- Modify: `internal/service/execute.go`
- Modify: `internal/app/app.go`
- Create: `tests/e2e/runtime_feedback_test.go`
- Modify: `tests/e2e/http_test_helpers.go`

- [ ] **Step 1: Write the failing runtime feedback tests**

```go
func TestPreOutputRetryableFailureEjectsPrimaryAndEnablesFallback(t *testing.T) {
	env := startHealthHTTPTestEnv(t, "primary_rate_limit_then_backup_success")
	defer env.Close()

	waitForHealthy(t, env, "openai-backup")
	postChat(t, env, ...)

	admin := getAdminUpstreams(t, env)
	assertUpstreamState(t, admin, "openai-main", "open")
}
```

- [ ] **Step 2: Run feedback tests to verify failure**

Run: `go test ./tests/e2e -run 'TestPreOutputRetryableFailure|TestPostOutputFailureDoesNotEject' -count=1`
Expected: FAIL because real execution results are not yet written into runtime

- [ ] **Step 3: Add runtime feedback hooks**

On real execution:

- success -> `RecordRequestSuccess`
- retryable pre-output failure -> `RecordRequestFailure(..., retryable=true, outputCommitted=false, ...)`
- post-output failure -> record error summary only, without advancing ejection

- [ ] **Step 4: Preserve existing failover boundaries**

Do not change orchestrator semantics:

- fail over before output commit
- do not fail over after output commit

- [ ] **Step 5: Run feedback tests**

Run: `go test ./tests/e2e -run 'TestPreOutputRetryableFailure|TestPostOutputFailureDoesNotEject' -count=1`
Expected: PASS

- [ ] **Step 6: Commit the feedback slice**

```bash
git add internal/runtime/executor.go internal/service/execute.go internal/app/app.go tests/e2e/http_test_helpers.go tests/e2e/runtime_feedback_test.go
git commit -m "feat: feed request outcomes into runtime health"
```

### Task 6: Final Verification and Recovery Matrix

**Files:**
- Modify: `tests/e2e/health_http_test.go`
- Modify: `tests/e2e/runtime_feedback_test.go`
- Modify: `tests/e2e/http_test_helpers.go`
- Modify: `tests/integration/example_config_test.go`

- [ ] **Step 1: Tighten the end-to-end runtime matrix**

Cover:

- initial probe pending -> `/readyz` 503
- all required route groups healthy -> `/readyz` 200
- one required route group exhausted -> `/readyz` 503
- `/admin/upstreams` exact snapshot fields
- primary ejected -> planner skips it
- half-open probe recovery -> upstream returns to `healthy`

Keep runtime-health tests hermetic:

- use local stub probe servers instead of real provider base URLs
- override example/test configs inside tests when background probes would otherwise target public upstream hosts
- ensure `app.New()`-started probe goroutines are canceled through service shutdown in every test that constructs a service

- [ ] **Step 2: Run dedicated runtime-health tests**

Run: `go test ./internal/health ./tests/e2e ./tests/integration -run 'TestRuntime|TestProber|TestReadyz|TestAdminUpstreams|TestPreOutputRetryableFailure|TestPostOutputFailureDoesNotEject|TestExampleConfigBoots' -count=1`
Expected: PASS

- [ ] **Step 3: Run the full repository test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 4: Build the production binary**

Run: `go build -o /tmp/nexus-router ./cmd/nexus-router`
Expected: PASS

- [ ] **Step 5: Re-run example config boot verification**

Run: `go test ./tests/integration -run TestExampleConfigBoots -count=1`
Expected: PASS

- [ ] **Step 6: Commit final verification adjustments**

```bash
git add tests/e2e tests/integration
git commit -m "test: verify runtime health and readiness behavior"
```

## Final Verification Checklist

Before calling this slice complete, run all of the following fresh:

- `go test ./internal/health -count=1`
- `go test ./tests/e2e/... -count=1`
- `go test ./tests/integration -count=1`
- `go test ./...`
- `go build -o /tmp/nexus-router ./cmd/nexus-router`
- `go test ./tests/integration -run TestExampleConfigBoots -count=1`

The slice only counts as complete if:

- background probes run for configured upstreams
- runtime health is stored in one in-memory model
- planner eligibility comes from runtime state
- `/readyz` reflects route-group-level upstream availability
- `/admin/upstreams` returns stable runtime JSON
- pre-output retryable request failures influence runtime health
- post-output failures do not cause incorrect proactive ejection
