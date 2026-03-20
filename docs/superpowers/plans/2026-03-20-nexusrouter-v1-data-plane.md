# NexusRouter V1 Data Plane Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single-binary Go data-plane router that serves OpenAI-compatible `chat.completions` and `responses`, routes to managed OpenAI and Anthropic upstreams, preserves streaming semantics, and enforces a tested compatibility contract without external runtime dependencies.

**Architecture:** The implementation uses one shared canonical execution pipeline behind two public HTTP endpoints. Public requests are normalized into a canonical request model, validated by a code-owned capability registry, routed through health-aware and breaker-aware provider selection, then executed via OpenAI passthrough or Anthropic translation adapters. Client-visible output is always shaped by NexusRouter encoders rather than raw provider events.

**Tech Stack:** Go, standard library `net/http`, `http.Client`, `httptest`, `encoding/json`, `crypto/tls`, `log/slog`, `gopkg.in/yaml.v3`

---

**Execution notes:**

- Work on `main` is allowed for this repo.
- During implementation, use `@superpowers/test-driven-development` before each feature slice.
- If any test or behavior diverges unexpectedly, use `@superpowers/systematic-debugging`.
- Before claiming a task is complete, use `@superpowers/verification-before-completion`.
- Keep files focused; do not collapse the whole router into `main.go`.

## Planned File Structure

### Bootstrap and app wiring

- Create: `go.mod`
- Create: `go.sum`
- Create: `cmd/nexus-router/main.go`
- Create: `internal/app/app.go`
- Create: `internal/app/server.go`

### Configuration and policies

- Create: `internal/config/config.go`
- Create: `internal/config/load.go`
- Create: `internal/config/validate.go`
- Create: `internal/auth/policy.go`
- Create: `internal/auth/keys.go`
- Create: `configs/nexus-router.example.yaml`

### HTTP API and encoders

- Create: `internal/httpapi/router.go`
- Create: `internal/httpapi/middleware.go`
- Create: `internal/httpapi/handlers/health.go`
- Create: `internal/httpapi/handlers/admin.go`
- Create: `internal/httpapi/handlers/chat_completions.go`
- Create: `internal/httpapi/handlers/responses.go`
- Create: `internal/httpapi/openai/errors.go`
- Create: `internal/httpapi/openai/chat_types.go`
- Create: `internal/httpapi/openai/responses_types.go`
- Create: `internal/httpapi/openai/chat_encode.go`
- Create: `internal/httpapi/openai/responses_encode.go`

### Canonical models and capability registry

- Create: `internal/canonical/request.go`
- Create: `internal/canonical/content.go`
- Create: `internal/canonical/events.go`
- Create: `internal/capabilities/registry.go`
- Create: `internal/capabilities/validate.go`
- Create: `internal/capabilities/schema_subset.go`

### Routing and upstream state

- Create: `internal/router/planner.go`
- Create: `internal/router/selection.go`
- Create: `internal/health/manager.go`
- Create: `internal/health/prober.go`
- Create: `internal/circuitbreaker/breaker.go`

### Provider execution

- Create: `internal/providers/types.go`
- Create: `internal/providers/openai/adapter.go`
- Create: `internal/providers/openai/request.go`
- Create: `internal/providers/openai/stream.go`
- Create: `internal/providers/anthropic/adapter.go`
- Create: `internal/providers/anthropic/request.go`
- Create: `internal/providers/anthropic/stream.go`

### Streaming, orchestration, and usage

- Create: `internal/streaming/relay.go`
- Create: `internal/streaming/sse.go`
- Create: `internal/usage/usage.go`
- Create: `internal/orchestrator/runner.go`
- Create: `internal/observability/logger.go`

### Provider stubs and tests

- Create: `tests/testdata/config/minimal-openai.yaml`
- Create: `tests/testdata/config/openai-anthropic.yaml`
- Create: `tests/stubs/openai_stub_test.go`
- Create: `tests/stubs/anthropic_stub_test.go`
- Create: `tests/integration/bootstrap_test.go`
- Create: `tests/integration/health_admin_test.go`
- Create: `tests/integration/chat_openai_test.go`
- Create: `tests/integration/responses_openai_test.go`
- Create: `tests/integration/anthropic_translation_test.go`
- Create: `tests/integration/failover_test.go`
- Create: `tests/integration/tools_vision_test.go`
- Create: `tests/integration/structured_output_test.go`
- Create: `tests/integration/https_test.go`
- Create: `tests/conformance/conformance_test.go`

## Responsibility Map

- `internal/config/*`: Parse YAML, validate route groups, provider declarations, limits, TLS settings, and client-key policies.
- `internal/auth/*`: Resolve bearer tokens to client policy and feature permissions.
- `internal/httpapi/*`: Decode public OpenAI-compatible requests, attach auth context, and encode OpenAI-compatible success or error responses.
- `internal/canonical/*`: Own the provider-neutral request, content block, and event models.
- `internal/capabilities/*`: Own the code-defined compatibility profiles and reject unsupported requests before execution.
- `internal/router/*`: Convert public model selection plus runtime state into ordered upstream attempts.
- `internal/health/*`: Run active probes and track temporary ejection.
- `internal/circuitbreaker/*`: Track per-upstream open, half-open, and closed state.
- `internal/providers/openai/*`: Perform validated OpenAI-native upstream execution and convert stream/output into canonical events.
- `internal/providers/anthropic/*`: Translate canonical requests to Anthropic, then convert Anthropic output into canonical events.
- `internal/streaming/*`: Flush SSE data without buffering generated content while preserving canonical event ordering.
- `internal/orchestrator/*`: Coordinate validation, route attempts, failover boundaries, and final response assembly.
- `tests/stubs/*`: Provide deterministic fake providers for success, timeout, 429, 5xx, partial-stream, tool-call, vision, and structured-output scenarios.

### Task 1: Bootstrap the Go Service Skeleton

**Files:**
- Create: `go.mod`
- Create: `cmd/nexus-router/main.go`
- Create: `internal/app/app.go`
- Create: `internal/app/server.go`
- Create: `internal/config/config.go`
- Create: `internal/observability/logger.go`
- Test: `tests/integration/bootstrap_test.go`

- [ ] **Step 1: Write the failing bootstrap test**

```go
package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nikkofu/nexus-router/internal/app"
	"github.com/nikkofu/nexus-router/internal/config"
)

func TestNewServerExposesLiveness(t *testing.T) {
	cfg := config.Config{
		Server: config.ServerConfig{ListenAddr: "127.0.0.1:0"},
	}

	srv, err := app.New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./tests/integration -run TestNewServerExposesLiveness -v`
Expected: FAIL with missing module, missing packages, or undefined `app.New`

- [ ] **Step 3: Write the minimal bootstrap implementation**

```go
// cmd/nexus-router/main.go
package main

import (
	"log"

	"github.com/nikkofu/nexus-router/internal/app"
	"github.com/nikkofu/nexus-router/internal/config"
)

func main() {
	srv, err := app.New(config.Config{})
	if err != nil {
		log.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
}
```

```go
// internal/config/config.go
package config

type Config struct {
	Server ServerConfig
}

type ServerConfig struct {
	ListenAddr string
}
```

```go
// internal/app/app.go
package app

import (
	"net/http"

	"github.com/nikkofu/nexus-router/internal/config"
)

type Service struct {
	handler http.Handler
}

func New(cfg config.Config) (*Service, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return &Service{handler: mux}, nil
}

func (s *Service) Handler() http.Handler { return s.handler }
```

- [ ] **Step 4: Run the targeted test to verify it passes**

Run: `go test ./tests/integration -run TestNewServerExposesLiveness -v`
Expected: PASS

- [ ] **Step 5: Expand bootstrap to production-ready skeleton**

Add:
- `go mod init github.com/nikkofu/nexus-router`
- `internal/app/server.go` with a real `http.Server`
- `internal/config/config.go` with the minimal root config structs used by tests
- `internal/observability/logger.go` with `slog.Logger`
- graceful startup and shutdown stubs

- [ ] **Step 6: Run the full bootstrap test set**

Run: `go test ./tests/integration -run 'TestNewServerExposesLiveness|TestMainBinaryBuilds' -v`
Expected: PASS

- [ ] **Step 7: Commit bootstrap slice**

```bash
git add go.mod go.sum cmd/nexus-router/main.go internal/app internal/config/config.go internal/observability tests/integration/bootstrap_test.go
git commit -m "feat: bootstrap nexus-router service skeleton"
```

### Task 2: Add YAML Config Loading and Validation

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/load.go`
- Create: `internal/config/validate.go`
- Create: `configs/nexus-router.example.yaml`
- Create: `tests/testdata/config/minimal-openai.yaml`
- Create: `tests/testdata/config/openai-anthropic.yaml`
- Modify: `internal/app/app.go`
- Test: `tests/integration/bootstrap_test.go`

- [ ] **Step 1: Write failing config tests**

```go
func TestLoadConfigRejectsUnknownRouteGroup(t *testing.T) {
	_, err := config.Load(strings.NewReader(`
server:
  listen_addr: 127.0.0.1:8080
models:
  - pattern: openai/gpt-*
    route_group: missing
`))
	if err == nil {
		t.Fatal("expected validation error")
	}
}
```

- [ ] **Step 2: Run config tests to verify failure**

Run: `go test ./... -run TestLoadConfigRejectsUnknownRouteGroup -v`
Expected: FAIL because `config.Load` does not exist

- [ ] **Step 3: Implement config structs and loader**

```go
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	Models   []ModelConfig  `yaml:"models"`
	Providers []ProviderConfig `yaml:"providers"`
	Routing  RoutingConfig  `yaml:"routing"`
	Health   HealthConfig   `yaml:"health"`
	Breaker  BreakerConfig  `yaml:"breaker"`
	Limits   LimitsConfig   `yaml:"limits"`
}

func Load(r io.Reader) (Config, error) {
	var cfg Config
	if err := yaml.NewDecoder(r).Decode(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, Validate(cfg)
}
```

- [ ] **Step 4: Implement explicit validation rules**

Validation must reject:
- undefined route groups
- duplicate public model patterns
- provider instances without `name`, `provider`, or `base_url`
- invalid TLS mode combinations
- empty client-key policy

- [ ] **Step 5: Add runnable example and test configs**

`configs/nexus-router.example.yaml` should include:
- one client key
- one OpenAI upstream
- one Anthropic upstream
- one OpenAI family route group
- one Anthropic family route group

`tests/testdata/config/minimal-openai.yaml` should include:
- one client key
- one OpenAI upstream
- one OpenAI route group

`tests/testdata/config/openai-anthropic.yaml` should include:
- one client key
- one OpenAI route group
- one Anthropic route group
- both upstream families

- [ ] **Step 6: Add the YAML dependency and run config-related tests**

Run: `go get gopkg.in/yaml.v3 && go test ./... -run 'TestLoadConfigRejectsUnknownRouteGroup|TestLoadConfigAcceptsExample' -v`
Expected: PASS

- [ ] **Step 7: Commit config slice**

```bash
git add internal/config configs/nexus-router.example.yaml tests/testdata/config internal/app/app.go go.sum tests/integration/bootstrap_test.go
git commit -m "feat: add config loading and validation"
```

### Task 3: Add Client Auth, Health Endpoints, and Read-Only Admin Surface

**Files:**
- Create: `internal/auth/policy.go`
- Create: `internal/auth/keys.go`
- Create: `internal/httpapi/router.go`
- Create: `internal/httpapi/middleware.go`
- Create: `internal/httpapi/handlers/health.go`
- Create: `internal/httpapi/handlers/admin.go`
- Modify: `internal/app/app.go`
- Test: `tests/integration/health_admin_test.go`

- [ ] **Step 1: Write failing admin and auth tests**

```go
func TestChatEndpointRejectsMissingBearerToken(t *testing.T) {
	resp := performRequest(t, http.MethodPost, "/v1/chat/completions", nil, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestAdminRoutesExposeRuntimeSummary(t *testing.T) {
	resp := performRequest(t, http.MethodGet, "/admin/routes", nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./tests/integration -run 'TestChatEndpointRejectsMissingBearerToken|TestAdminRoutesExposeRuntimeSummary' -v`
Expected: FAIL because routes and middleware do not exist

- [ ] **Step 3: Implement key resolution and policy checks**

```go
type ClientPolicy struct {
	ID                   string
	AllowedModelPatterns []string
	AllowStreaming       bool
	AllowTools           bool
	AllowVision          bool
	AllowStructured      bool
}

func (r Resolver) ResolveBearer(token string) (ClientPolicy, bool) {
	// compare against configured keys or hashes
}
```

- [ ] **Step 4: Implement health and admin handlers**

Admin output must include:
- config summary
- public model mapping
- upstream health state
- breaker state

- [ ] **Step 5: Wire auth middleware only onto protected public endpoints**

Public endpoint behavior:
- `/v1/chat/completions`: protected
- `/v1/responses`: protected
- `/livez`: unprotected
- `/readyz`: unprotected
- `/admin/*`: unprotected by default in tests, but mounted on a separate admin listener or restricted address in config

- [ ] **Step 6: Run the health/admin/auth test suite**

Run: `go test ./tests/integration -run 'TestChatEndpointRejectsMissingBearerToken|TestAdminRoutesExposeRuntimeSummary|TestReadyzFailsWithoutEligibleUpstream' -v`
Expected: PASS

- [ ] **Step 7: Commit auth and admin slice**

```bash
git add internal/auth internal/httpapi internal/app/app.go tests/integration/health_admin_test.go
git commit -m "feat: add auth middleware and admin endpoints"
```

### Task 4: Add Health Manager, Breaker, and Route Planner

**Files:**
- Create: `internal/router/planner.go`
- Create: `internal/router/selection.go`
- Create: `internal/health/manager.go`
- Create: `internal/health/prober.go`
- Create: `internal/circuitbreaker/breaker.go`
- Modify: `internal/httpapi/handlers/admin.go`
- Test: `tests/integration/failover_test.go`

- [ ] **Step 1: Write failing route and breaker tests**

```go
func TestPlannerSkipsEjectedUpstream(t *testing.T) {
	plan := planner.Plan("openai/gpt-4.1")
	if len(plan.Attempts) != 1 || plan.Attempts[0].Upstream != "openai-backup" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestBreakerOpensAfterThreshold(t *testing.T) {
	b := circuitbreaker.New(3, time.Minute)
	for i := 0; i < 3; i++ {
		b.RecordFailure()
	}
	if state := b.State(); state != circuitbreaker.StateOpen {
		t.Fatalf("state = %v, want open", state)
	}
}
```

- [ ] **Step 2: Run targeted tests to verify failure**

Run: `go test ./tests/integration -run 'TestPlannerSkipsEjectedUpstream|TestBreakerOpensAfterThreshold' -v`
Expected: FAIL because planner and breaker types do not exist

- [ ] **Step 3: Implement the in-memory breaker**

```go
type State string

const (
	StateClosed   State = "closed"
	StateOpen     State = "open"
	StateHalfOpen State = "half_open"
)
```

Breaker behavior:
- open on classified failure threshold
- allow limited probes in half-open state
- close after configured recovery success threshold

- [ ] **Step 4: Implement health manager and temporary ejection**

The manager should track:
- last probe result
- last failure reason
- ejected-until timestamp
- current breaker state

- [ ] **Step 5: Implement route planning**

Planner rules:
- match public model pattern to route group
- keep route groups within one provider family
- skip ejected or open-breaker upstreams
- preserve configured fallback ordering

- [ ] **Step 6: Run failover and planner tests**

Run: `go test ./tests/integration -run 'TestPlannerSkipsEjectedUpstream|TestBreakerOpensAfterThreshold|TestPlannerReturnsOrderedFallbacks' -v`
Expected: PASS

- [ ] **Step 7: Commit routing state slice**

```bash
git add internal/router internal/health internal/circuitbreaker internal/httpapi/handlers/admin.go tests/integration/failover_test.go
git commit -m "feat: add health-aware route planning and breaker state"
```

### Task 5: Add Canonical Models and Capability Validation

**Files:**
- Create: `internal/canonical/request.go`
- Create: `internal/canonical/content.go`
- Create: `internal/canonical/events.go`
- Create: `internal/capabilities/registry.go`
- Create: `internal/capabilities/validate.go`
- Create: `internal/capabilities/schema_subset.go`
- Test: `tests/integration/tools_vision_test.go`
- Test: `tests/integration/structured_output_test.go`

- [ ] **Step 1: Write failing capability tests**

```go
func TestRejectsUnsupportedSchemaKeyword(t *testing.T) {
	err := capabilities.ValidateRequest(registry, canonical.Request{
		PublicModel: "anthropic/claude-sonnet-4-5",
		ResponseContract: canonical.ResponseContract{
			Kind: canonical.ResponseContractJSONSchema,
			Schema: map[string]any{"oneOf": []any{}},
		},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
```

- [ ] **Step 2: Run capability tests to verify failure**

Run: `go test ./tests/integration -run 'TestRejectsUnsupportedSchemaKeyword|TestRejectsVisionWhenPolicyDisallowsIt' -v`
Expected: FAIL because canonical and capability packages do not exist

- [ ] **Step 3: Implement canonical request and event types**

```go
type Request struct {
	EndpointKind     EndpointKind
	PublicModel      string
	Conversation     []Turn
	Generation       Generation
	Tools            []Tool
	ToolChoice       ToolChoice
	ResponseContract ResponseContract
	Stream           bool
	Metadata         map[string]string
}
```

- [ ] **Step 4: Implement a code-owned capability registry**

Registry must define support for:
- `openai/gpt-*`
- `anthropic/claude-*`

Registry dimensions:
- endpoint kinds
- streaming
- tools
- vision
- structured outputs
- schema constraints

- [ ] **Step 5: Implement request validation**

Validation must reject:
- unsupported endpoint for a model family
- vision without policy permission
- tools without policy permission
- unsupported JSON Schema keywords
- unsupported tool schema constructs

- [ ] **Step 6: Run the capability-focused test suite**

Run: `go test ./tests/integration -run 'TestRejectsUnsupportedSchemaKeyword|TestRejectsVisionWhenPolicyDisallowsIt|TestAcceptsManagedModelFamily' -v`
Expected: PASS

- [ ] **Step 7: Commit canonical and capability slice**

```bash
git add internal/canonical internal/capabilities tests/integration/tools_vision_test.go tests/integration/structured_output_test.go
git commit -m "feat: add canonical request model and capability registry"
```

### Task 6: Add Public OpenAI-Compatible Decoders and Error Encoders

**Files:**
- Create: `internal/httpapi/openai/errors.go`
- Create: `internal/httpapi/openai/chat_types.go`
- Create: `internal/httpapi/openai/responses_types.go`
- Create: `internal/httpapi/openai/chat_encode.go`
- Create: `internal/httpapi/openai/responses_encode.go`
- Create: `internal/httpapi/handlers/chat_completions.go`
- Create: `internal/httpapi/handlers/responses.go`
- Test: `tests/integration/chat_openai_test.go`
- Test: `tests/integration/responses_openai_test.go`

- [ ] **Step 1: Write failing decoder tests**

```go
func TestChatHandlerNormalizesMessagesIntoCanonicalRequest(t *testing.T) {
	reqBody := `{"model":"openai/gpt-4.1","messages":[{"role":"user","content":"hello"}]}`
	got := captureCanonicalRequestFromChatHandler(t, reqBody)
	if got.PublicModel != "openai/gpt-4.1" {
		t.Fatalf("model = %q", got.PublicModel)
	}
}
```

- [ ] **Step 2: Run decoder tests to verify failure**

Run: `go test ./tests/integration -run 'TestChatHandlerNormalizesMessagesIntoCanonicalRequest|TestResponsesHandlerNormalizesInputIntoCanonicalRequest' -v`
Expected: FAIL because handlers and decoder types do not exist

- [ ] **Step 3: Implement public request decoding**

Minimum normalization coverage:
- text messages
- image content items
- tools and tool choice
- response format / schema contract
- streaming flag

- [ ] **Step 4: Implement OpenAI-compatible error responses**

```go
func WriteError(w http.ResponseWriter, code int, typ, message string) {
	// write {"error":{"type":typ,"message":message}}
}
```

- [ ] **Step 5: Implement response encoders for both public endpoints**

Encoders must support:
- non-streaming final JSON
- streaming SSE framing
- normalized finish reasons

- [ ] **Step 6: Run endpoint decoder and error tests**

Run: `go test ./tests/integration -run 'TestChatHandlerNormalizesMessagesIntoCanonicalRequest|TestResponsesHandlerNormalizesInputIntoCanonicalRequest|TestUnsupportedCapabilityReturnsOpenAIError' -v`
Expected: PASS

- [ ] **Step 7: Commit public API codec slice**

```bash
git add internal/httpapi/openai internal/httpapi/handlers tests/integration/chat_openai_test.go tests/integration/responses_openai_test.go
git commit -m "feat: add public OpenAI-compatible decoders and encoders"
```

### Task 7: Add OpenAI Upstream Adapter and Streaming Relay

**Files:**
- Create: `internal/providers/types.go`
- Create: `internal/providers/openai/adapter.go`
- Create: `internal/providers/openai/request.go`
- Create: `internal/providers/openai/stream.go`
- Create: `internal/streaming/relay.go`
- Create: `internal/streaming/sse.go`
- Create: `internal/usage/usage.go`
- Create: `tests/stubs/openai_stub_test.go`
- Test: `tests/integration/chat_openai_test.go`
- Test: `tests/integration/responses_openai_test.go`

- [ ] **Step 1: Write failing OpenAI upstream tests**

```go
func TestChatStreamingRelaysOpenAIChunks(t *testing.T) {
	resp := performStreamingChatRequest(t, openAIStubURL(t))
	assertSSEContains(t, resp.Events, "chat.completion.chunk")
}
```

- [ ] **Step 2: Run OpenAI upstream tests to verify failure**

Run: `go test ./tests/integration -run 'TestChatStreamingRelaysOpenAIChunks|TestResponsesStreamingRelaysOpenAIEvents' -v`
Expected: FAIL because provider adapter and stubs do not exist

- [ ] **Step 3: Implement a deterministic OpenAI provider stub**

Stub cases:
- plain JSON success
- SSE success
- `429`
- `503`
- disconnect before body
- disconnect after one chunk

- [ ] **Step 4: Implement the OpenAI adapter**

Adapter responsibilities:
- inject upstream API key
- select `/v1/chat/completions` or `/v1/responses`
- convert upstream output into canonical events
- classify retryable versus non-retryable failures

- [ ] **Step 5: Implement streaming relay**

Relay rules:
- flush each SSE event as it is ready
- do not buffer generated text
- preserve canonical event ordering

- [ ] **Step 6: Run OpenAI adapter and streaming tests**

Run: `go test ./tests/integration -run 'TestChatStreamingRelaysOpenAIChunks|TestResponsesStreamingRelaysOpenAIEvents|TestOpenAIAdapterClassifiesRetryable429' -v`
Expected: PASS

- [ ] **Step 7: Commit OpenAI upstream slice**

```bash
git add internal/providers/types.go internal/providers/openai internal/streaming internal/usage tests/stubs/openai_stub_test.go tests/integration/chat_openai_test.go tests/integration/responses_openai_test.go
git commit -m "feat: add OpenAI upstream adapter and streaming relay"
```

### Task 8: Add Anthropic Translation Adapter

**Files:**
- Create: `internal/providers/anthropic/adapter.go`
- Create: `internal/providers/anthropic/request.go`
- Create: `internal/providers/anthropic/stream.go`
- Create: `tests/stubs/anthropic_stub_test.go`
- Test: `tests/integration/anthropic_translation_test.go`

- [ ] **Step 1: Write failing Anthropic translation tests**

```go
func TestAnthropicRouteTranslatesChatRequest(t *testing.T) {
	resp := performChatRequestAgainstModel(t, "anthropic/claude-sonnet-4-5")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run Anthropic translation tests to verify failure**

Run: `go test ./tests/integration -run 'TestAnthropicRouteTranslatesChatRequest|TestAnthropicStreamBecomesOpenAIChunks' -v`
Expected: FAIL because Anthropic adapter does not exist

- [ ] **Step 3: Implement a deterministic Anthropic provider stub**

Stub cases:
- text success
- SSE message events
- tool-use stream
- image acceptance response
- timeout
- `429`
- `503`

- [ ] **Step 4: Implement canonical-to-Anthropic request translation**

Translation rules:
- extract system prompt
- map content blocks
- map tools and tool choice
- map supported structured-output subset strategy

- [ ] **Step 5: Implement Anthropic stream-to-canonical conversion**

Conversion must normalize:
- text deltas
- tool-use deltas
- usage summary
- finish reason

- [ ] **Step 6: Run Anthropic integration tests**

Run: `go test ./tests/integration -run 'TestAnthropicRouteTranslatesChatRequest|TestAnthropicStreamBecomesOpenAIChunks|TestAnthropicResponsesRouteNormalizesOutput' -v`
Expected: PASS

- [ ] **Step 7: Commit Anthropic translation slice**

```bash
git add internal/providers/anthropic tests/stubs/anthropic_stub_test.go tests/integration/anthropic_translation_test.go
git commit -m "feat: add Anthropic translation adapter"
```

### Task 9: Add Orchestrator and Failover Boundary Enforcement

**Files:**
- Create: `internal/orchestrator/runner.go`
- Modify: `internal/httpapi/handlers/chat_completions.go`
- Modify: `internal/httpapi/handlers/responses.go`
- Modify: `internal/router/planner.go`
- Modify: `internal/providers/types.go`
- Test: `tests/integration/failover_test.go`

- [ ] **Step 1: Write failing failover-boundary tests**

```go
func TestFailoverOccursBeforeAnyOutput(t *testing.T) {
	resp := performStreamingChatRequestWithFirstUpstream429(t)
	assertSSEContains(t, resp.Events, "chat.completion.chunk")
	assertRouteAttempted(t, resp.Meta, "openai-primary", "openai-backup")
}

func TestNoFailoverAfterPartialStream(t *testing.T) {
	resp := performStreamingChatRequestWithMidstreamDisconnect(t)
	assertRouteAttempted(t, resp.Meta, "openai-primary")
	assertRouteNotAttempted(t, resp.Meta, "openai-backup")
}
```

- [ ] **Step 2: Run failover tests to verify failure**

Run: `go test ./tests/integration -run 'TestFailoverOccursBeforeAnyOutput|TestNoFailoverAfterPartialStream' -v`
Expected: FAIL because no orchestration layer enforces attempt boundaries

- [ ] **Step 3: Implement orchestrator attempt loop**

Core algorithm:
- validate request once
- build ordered attempt list
- try each upstream until success or non-retryable failure
- mark response as committed once any client-visible event is flushed
- stop failover after commitment

- [ ] **Step 4: Implement retry classification hooks**

Retryable before output:
- dial failure
- timeout
- `429`
- `502`
- `503`
- `504`

Non-retryable or no-failover-after-commit:
- partial stream interruption
- invalid request from upstream due to translation bug
- content already emitted

- [ ] **Step 5: Expose route-attempt metadata in logs and admin summaries**

This does not have to be a public response header in production, but test instrumentation must be able to assert attempt order.

- [ ] **Step 6: Run failover integration suite**

Run: `go test ./tests/integration -run 'TestFailoverOccursBeforeAnyOutput|TestNoFailoverAfterPartialStream|TestPlannerReturnsOrderedFallbacks' -v`
Expected: PASS

- [ ] **Step 7: Commit orchestration and failover slice**

```bash
git add internal/orchestrator internal/httpapi/handlers internal/router internal/providers/types.go tests/integration/failover_test.go
git commit -m "feat: enforce failover boundaries in orchestrator"
```

### Task 10: Add Tools, Vision, and Structured Output Support

**Files:**
- Modify: `internal/canonical/content.go`
- Modify: `internal/canonical/request.go`
- Modify: `internal/capabilities/registry.go`
- Modify: `internal/capabilities/schema_subset.go`
- Modify: `internal/httpapi/openai/chat_types.go`
- Modify: `internal/httpapi/openai/responses_types.go`
- Modify: `internal/providers/openai/request.go`
- Modify: `internal/providers/anthropic/request.go`
- Modify: `internal/providers/anthropic/stream.go`
- Test: `tests/integration/tools_vision_test.go`
- Test: `tests/integration/structured_output_test.go`

- [ ] **Step 1: Write failing feature-slice tests**

```go
func TestToolCallStreamsInOpenAICompatibleShape(t *testing.T) {
	resp := performToolChatRequest(t, "anthropic/claude-sonnet-4-5")
	assertSSEContains(t, resp.Events, "tool_calls")
}

func TestStructuredOutputRejectsOneOfSchemas(t *testing.T) {
	resp := performStructuredOutputRequest(t, map[string]any{"oneOf": []any{{}}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run feature tests to verify failure**

Run: `go test ./tests/integration -run 'TestToolCallStreamsInOpenAICompatibleShape|TestStructuredOutputRejectsOneOfSchemas|TestVisionRequestMapsToAnthropicBlocks' -v`
Expected: FAIL because full feature normalization is incomplete

- [ ] **Step 3: Implement tool schema subset validation**

Allowed subset:
- object roots
- explicit properties
- required fields
- scalar enums
- bounded nesting

Rejected subset:
- recursion
- `oneOf`
- `anyOf`
- `allOf`
- `patternProperties`
- unbounded `additionalProperties`

- [ ] **Step 4: Implement vision normalization**

Support:
- image URLs
- base64/data URLs when enabled by limits and policy

Reject:
- unsupported MIME types
- too many images
- image payload above configured size

- [ ] **Step 5: Implement provider-specific request and stream mapping**

OpenAI route:
- preserve validated request shape where possible

Anthropic route:
- map tool definitions to Anthropic tools
- map image blocks to Anthropic image input
- normalize returned tool-use events into OpenAI-compatible output

- [ ] **Step 6: Run tools, vision, and structured-output tests**

Run: `go test ./tests/integration -run 'TestToolCallStreamsInOpenAICompatibleShape|TestStructuredOutputRejectsOneOfSchemas|TestVisionRequestMapsToAnthropicBlocks|TestStructuredOutputAcceptsManagedSubset' -v`
Expected: PASS

- [ ] **Step 7: Commit advanced feature slice**

```bash
git add internal/canonical internal/capabilities internal/httpapi/openai internal/providers/openai internal/providers/anthropic tests/integration/tools_vision_test.go tests/integration/structured_output_test.go
git commit -m "feat: add tools vision and structured output support"
```

### Task 11: Add HTTPS Mode and Example Deployment Coverage

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/app/server.go`
- Modify: `configs/nexus-router.example.yaml`
- Test: `tests/integration/https_test.go`
- Test: `tests/integration/bootstrap_test.go`

- [ ] **Step 1: Write failing HTTPS tests**

```go
func TestServerStartsWithTLSConfig(t *testing.T) {
	baseURL := startTLSServerFromExampleConfig(t)
	resp, err := http.Get(baseURL + "/livez")
	if err != nil {
		t.Fatalf("GET /livez error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run HTTPS tests to verify failure**

Run: `go test ./tests/integration -run TestServerStartsWithTLSConfig -v`
Expected: FAIL because TLS config path handling is incomplete

- [ ] **Step 3: Implement TLS server mode**

Support config fields for:
- disabled
- direct TLS cert/key paths

Keep reverse-proxy deployment simple by allowing plain HTTP mode from the same server wiring.

- [ ] **Step 4: Update example config and startup docs in comments**

Example config should clearly show:
- plain HTTP local mode
- commented TLS fields for direct HTTPS mode

- [ ] **Step 5: Run HTTPS and bootstrap tests**

Run: `go test ./tests/integration -run 'TestServerStartsWithTLSConfig|TestMainBinaryBuilds' -v`
Expected: PASS

- [ ] **Step 6: Commit HTTPS support slice**

```bash
git add internal/config/config.go internal/app/server.go configs/nexus-router.example.yaml tests/integration/https_test.go tests/integration/bootstrap_test.go
git commit -m "feat: add configurable https mode"
```

### Task 12: Add End-to-End Conformance Suite and Final Verification

**Files:**
- Create: `tests/conformance/conformance_test.go`
- Modify: `tests/integration/chat_openai_test.go`
- Modify: `tests/integration/responses_openai_test.go`
- Modify: `tests/integration/anthropic_translation_test.go`
- Modify: `tests/integration/tools_vision_test.go`
- Modify: `tests/integration/structured_output_test.go`
- Modify: `tests/integration/failover_test.go`

- [ ] **Step 1: Write the failing conformance harness**

```go
func TestConformanceMatrix(t *testing.T) {
	cases := []struct{
		name  string
		model string
	}{
		{"chat_text", "openai/gpt-4.1"},
		{"chat_text", "anthropic/claude-sonnet-4-5"},
	}

	for _, tc := range cases {
		t.Run(tc.name+"_"+sanitize(tc.model), func(t *testing.T) {
			out := runConformanceCase(t, tc.name, tc.model)
			assertConforms(t, tc.name, out)
		})
	}
}
```

- [ ] **Step 2: Run conformance test to verify failure**

Run: `go test ./tests/conformance -run TestConformanceMatrix -v`
Expected: FAIL because the harness and/or one or more behaviors are incomplete

- [ ] **Step 3: Implement the conformance matrix**

Required case families:
- chat text
- responses text
- streaming text
- tool call stream
- vision request
- supported structured output
- rejected unsupported schema
- pre-output failover
- no failover after partial output

- [ ] **Step 4: Normalize test assertions around client-visible semantics**

Assert:
- response shape
- event order
- finish reason semantics
- tool-call structure
- normalized error type

Do not assert identical generated text across provider families.

- [ ] **Step 5: Run the full repository test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: Build the production binary**

Run: `go build ./cmd/nexus-router`
Expected: PASS

- [ ] **Step 7: Run the example-config smoke tests**

Run: `go test ./tests/integration -run 'TestExampleConfigBoots|TestConformanceMatrix' -v`
Expected: PASS

- [ ] **Step 8: Commit final verification slice**

```bash
git add tests/conformance tests/integration
git commit -m "test: add conformance suite and final verification coverage"
```

## Final Verification Checklist

Before calling the project complete, run all of the following fresh:

- `go test ./...`
- `go build ./cmd/nexus-router`

The final implementation only counts as complete if:

- both public endpoints work,
- OpenAI upstream works,
- Anthropic translation works,
- streaming works on both public endpoints,
- tools, vision, and the managed structured-output subset are covered,
- health checks, breaker state, temporary ejection, and failover work,
- HTTP and HTTPS modes both boot,
- admin inspection endpoints are readable,
- the conformance matrix passes.
