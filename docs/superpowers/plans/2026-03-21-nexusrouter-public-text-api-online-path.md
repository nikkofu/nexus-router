# NexusRouter Public Text API Online Path Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire `POST /v1/chat/completions` and `POST /v1/responses` into a real online text-path execution chain so they work over HTTP against OpenAI and Anthropic upstreams with both streaming and non-streaming responses.

**Architecture:** This slice adds a runtime executor and a thin application service layer on top of the existing planner, orchestrator, and provider adapters. Public handlers decode OpenAI-compatible requests, enforce the public text-only contract, invoke the shared execution service, and then either stream canonical events or aggregate them into final JSON responses.

**Tech Stack:** Go, standard library `net/http`, `context`, `httptest`, `encoding/json`, existing internal `canonical`, `capabilities`, `router`, `orchestrator`, `providers`, `streaming`

---

**Execution notes:**

- Work on `main` is allowed in this repository.
- Use `@superpowers/test-driven-development` before each feature slice.
- Keep provider adapters reusable; do not duplicate their logic in handlers.
- The public text-only restriction is a handler/service concern for this slice, not a deletion of internal capability support.
- Do not add public support for tools, vision, or structured outputs in this plan.

## Planned File Structure

### Runtime and service wiring

- Create: `internal/runtime/executor.go`
- Create: `internal/runtime/registry.go`
- Create: `internal/service/execute.go`

### Public handler and response finalization

- Modify: `internal/httpapi/middleware.go`
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/handlers/chat_completions.go`
- Modify: `internal/httpapi/handlers/responses.go`
- Create: `internal/httpapi/openai/finalize_chat.go`
- Create: `internal/httpapi/openai/finalize_responses.go`

### Supporting provider types and app wiring

- Modify: `internal/providers/types.go`
- Modify: `internal/app/app.go`

### End-to-end test coverage

- Create: `tests/e2e/http_test_helpers.go`
- Create: `tests/e2e/runtime_test.go`
- Create: `tests/e2e/service_test.go`
- Create: `tests/e2e/middleware_test.go`
- Create: `tests/e2e/finalize_test.go`
- Create: `tests/e2e/chat_http_test.go`
- Create: `tests/e2e/responses_http_test.go`
- Create: `tests/e2e/failover_http_test.go`

## Responsibility Map

- `internal/runtime/*`: Resolve upstream names to configured provider instances and dispatch attempts to the correct provider adapter.
- `internal/service/execute.go`: Enforce the public text-path gate, call capability validation, invoke orchestrator, and return canonical events plus route metadata.
- `internal/httpapi/handlers/*`: Read auth policy from request context, decode public requests, call the execution service, and encode stream/non-stream responses.
- `internal/httpapi/openai/finalize_*`: Convert canonical text events into final OpenAI-compatible JSON responses for non-stream requests.
- `tests/e2e/*`: Validate the real online request path through HTTP against local stub upstreams.

### Task 1: Add Runtime Provider Registry and Executor

**Files:**
- Create: `internal/runtime/registry.go`
- Create: `internal/runtime/executor.go`
- Modify: `internal/providers/types.go`
- Test: `tests/e2e/runtime_test.go`

- [ ] **Step 1: Write the failing runtime executor tests**

```go
func TestRuntimeExecutorDispatchesOpenAIUpstream(t *testing.T) {
	registry := runtime.NewRegistry([]config.ProviderConfig{
		{Name: "openai-main", Provider: "openai", BaseURL: stubURL},
	})
	executor := runtime.NewExecutor(registry, openAIAdapter, anthropicAdapter)

	_, err := executor.Execute(context.Background(), "openai-main", canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "openai/gpt-4.1",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}
```

- [ ] **Step 2: Run the runtime tests to verify failure**

Run: `go test ./tests/e2e -run 'TestRuntimeExecutorDispatchesOpenAIUpstream|TestRuntimeExecutorDispatchesAnthropicUpstream|TestRuntimeExecutorRejectsUnknownUpstream' -v`
Expected: FAIL because `internal/runtime` does not exist yet

- [ ] **Step 3: Implement the provider registry**

```go
type Registry struct {
	providers map[string]config.ProviderConfig
}

func NewRegistry(cfgs []config.ProviderConfig) Registry {
	// build name -> config map
}
```

- [ ] **Step 4: Implement the runtime executor**

Runtime executor responsibilities:
- resolve upstream name to `config.ProviderConfig`
- dispatch to OpenAI adapter when `provider == "openai"`
- dispatch to Anthropic adapter when `provider == "anthropic"`
- wrap unknown upstream and unknown provider errors

- [ ] **Step 5: Extend provider execution types only if needed**

If route metadata or execution errors need a more formal carrier, update `internal/providers/types.go` without changing orchestrator semantics.

- [ ] **Step 6: Run the runtime executor test suite**

Run: `go test ./tests/e2e -run 'TestRuntimeExecutorDispatchesOpenAIUpstream|TestRuntimeExecutorDispatchesAnthropicUpstream|TestRuntimeExecutorRejectsUnknownUpstream' -v`
Expected: PASS

- [ ] **Step 7: Commit the runtime slice**

```bash
git add internal/runtime internal/providers/types.go tests/e2e/runtime_test.go tests/e2e/http_test_helpers.go
git commit -m "feat: add runtime provider executor"
```

### Task 2: Add the Public Text-Path Execution Service

**Files:**
- Create: `internal/service/execute.go`
- Modify: `internal/capabilities/validate.go`
- Test: `tests/e2e/service_test.go`

- [ ] **Step 1: Write the failing service tests**

```go
func TestExecutionServiceRejectsPublicToolsRequest(t *testing.T) {
	service := newTestService(t)
	_, err := service.Execute(context.Background(), auth.ClientPolicy{AllowTools: true}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "openai/gpt-4.1",
		Tools: []canonical.Tool{{Name: "lookup_weather"}},
	})
	if err == nil {
		t.Fatal("expected unsupported capability error")
	}
}
```

- [ ] **Step 2: Run the service tests to verify failure**

Run: `go test ./tests/e2e -run 'TestExecutionServiceRejectsPublicToolsRequest|TestExecutionServiceRejectsPublicVisionRequest|TestExecutionServiceReturnsCanonicalEventsForTextRequest' -v`
Expected: FAIL because `internal/service` does not exist yet

- [ ] **Step 3: Implement the public text-only gate**

Reject at this layer:
- any `tools`
- any image content
- any structured output contract

Return a stable internal error that the handlers can map to `unsupported_capability`.

- [ ] **Step 4: Implement the execution service**

Service flow:
- apply public text-only gate
- call capability validation
- call orchestrator with runtime executor
- return `providers.Result` plus route-attempt metadata

- [ ] **Step 5: Keep capability registry semantics intact**

Do not remove existing internal capability support. This layer only narrows the public surface for this slice.

- [ ] **Step 6: Run the service test suite**

Run: `go test ./tests/e2e -run 'TestExecutionServiceRejectsPublicToolsRequest|TestExecutionServiceRejectsPublicVisionRequest|TestExecutionServiceReturnsCanonicalEventsForTextRequest' -v`
Expected: PASS

- [ ] **Step 7: Commit the service slice**

```bash
git add internal/service tests/e2e/service_test.go tests/e2e/http_test_helpers.go
git commit -m "feat: add public text-path execution service"
```

### Task 3: Store Client Policy in Request Context

**Files:**
- Modify: `internal/httpapi/middleware.go`
- Test: `tests/e2e/middleware_test.go`

- [ ] **Step 1: Write the failing middleware tests**

```go
func TestAuthMiddlewareStoresClientPolicyInContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer sk-nx-local-dev")

	policy, status := invokeProtectedHandlerAndCapturePolicy(t, req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if policy.ID != "local-dev" {
		t.Fatalf("policy.ID = %q, want %q", policy.ID, "local-dev")
	}
}
```

- [ ] **Step 2: Run the middleware tests to verify failure**

Run: `go test ./tests/e2e -run 'TestAuthMiddlewareStoresClientPolicyInContext|TestAuthMiddlewareRejectsMissingBearer' -v`
Expected: FAIL because policy context storage is not implemented

- [ ] **Step 3: Add auth context helpers**

Add helpers such as:

```go
type contextKey string

const clientPolicyKey contextKey = "client_policy"

func WithClientPolicy(ctx context.Context, policy auth.ClientPolicy) context.Context
func ClientPolicyFromContext(ctx context.Context) (auth.ClientPolicy, bool)
```

- [ ] **Step 4: Update `RequireBearer` to store the resolved policy**

After token resolution, wrap the request with a context that contains the `auth.ClientPolicy`.

- [ ] **Step 5: Run the middleware tests to verify green**

Run: `go test ./tests/e2e -run 'TestAuthMiddlewareStoresClientPolicyInContext|TestAuthMiddlewareRejectsMissingBearer' -v`
Expected: PASS

- [ ] **Step 6: Commit the middleware slice**

```bash
git add internal/httpapi/middleware.go tests/e2e/middleware_test.go tests/e2e/http_test_helpers.go
git commit -m "feat: store auth policy in request context"
```

### Task 4: Add Final Response Aggregators for Non-Streaming Text Responses

**Files:**
- Create: `internal/httpapi/openai/finalize_chat.go`
- Create: `internal/httpapi/openai/finalize_responses.go`
- Test: `tests/e2e/finalize_test.go`

- [ ] **Step 1: Write the failing finalizer tests**

```go
func TestAggregateChatCompletionBuildsAssistantMessage(t *testing.T) {
	resp := openai.AggregateChatCompletion([]canonical.Event{
		{Type: canonical.EventContentDelta, Data: map[string]any{"text": "hel"}},
		{Type: canonical.EventContentDelta, Data: map[string]any{"text": "lo"}},
		{Type: canonical.EventMessageStop},
	}, canonical.Request{PublicModel: "openai/gpt-4.1"})

	if resp.Choices[0].Message.Content != "hello" {
		t.Fatalf("content = %q, want %q", resp.Choices[0].Message.Content, "hello")
	}
}
```

- [ ] **Step 2: Run the finalizer tests to verify failure**

Run: `go test ./tests/e2e -run 'TestAggregateChatCompletionBuildsAssistantMessage|TestAggregateResponsesBuildsOutputText' -v`
Expected: FAIL because finalizers do not exist yet

- [ ] **Step 3: Implement chat finalization**

Required fields:
- generated response ID
- `object: "chat.completion"`
- `model`
- one assistant choice with aggregated text
- normalized finish reason

- [ ] **Step 4: Implement responses finalization**

Required fields:
- generated response ID
- `object: "response"`
- `model`
- aggregated text output item
- completion status

- [ ] **Step 5: Keep finalizers pure**

These functions should not read HTTP request state or mutate global state.

- [ ] **Step 6: Run the finalizer tests to verify green**

Run: `go test ./tests/e2e -run 'TestAggregateChatCompletionBuildsAssistantMessage|TestAggregateResponsesBuildsOutputText' -v`
Expected: PASS

- [ ] **Step 7: Commit the finalizer slice**

```bash
git add internal/httpapi/openai/finalize_chat.go internal/httpapi/openai/finalize_responses.go tests/e2e/finalize_test.go tests/e2e/http_test_helpers.go
git commit -m "feat: add non-stream response finalizers"
```

### Task 5: Replace `NotImplemented` Handlers with Real Chat and Responses Execution

**Files:**
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/handlers/chat_completions.go`
- Modify: `internal/httpapi/handlers/responses.go`
- Modify: `internal/app/app.go`
- Test: `tests/e2e/chat_http_test.go`
- Test: `tests/e2e/responses_http_test.go`

- [ ] **Step 1: Write the failing public HTTP tests**

```go
func TestChatCompletionsHTTPStreamingOpenAI(t *testing.T) {
	env := startHTTPTestEnv(t, "openai_chat_stream")

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/chat/completions", env.Token, map[string]any{
		"model": "openai/gpt-4.1",
		"stream": true,
		"messages": []map[string]any{
			{"role": "user", "content": "hello"},
		},
	})

	assertStatus(t, resp, http.StatusOK)
	assertBodyContains(t, resp, "chat.completion.chunk")
}
```

- [ ] **Step 2: Run the HTTP tests to verify failure**

Run: `go test ./tests/e2e -run 'TestChatCompletionsHTTPStreamingOpenAI|TestChatCompletionsHTTPNonStreamingAnthropic|TestResponsesHTTPStreamingOpenAI|TestResponsesHTTPNonStreamingAnthropic' -v`
Expected: FAIL because public handlers are not yet wired into the execution service

- [ ] **Step 3: Inject the execution service into router/app wiring**

`app.New()` should build:
- auth resolver
- runtime registry/executor
- execution service
- public handlers

`router.go` should register real handlers, not `NotImplemented()`.

- [ ] **Step 4: Implement `chat_completions` handler**

Handler flow:
- read policy from request context
- decode with `DecodeChatCompletionRequest`
- call execution service
- on `stream=true`, write chat SSE
- on `stream=false`, aggregate final chat JSON
- map errors to OpenAI-style responses

- [ ] **Step 5: Implement `responses` handler**

Handler flow:
- read policy from request context
- decode with `DecodeResponsesRequest`
- call execution service
- on `stream=true`, write response SSE
- on `stream=false`, aggregate final responses JSON
- map errors to OpenAI-style responses

- [ ] **Step 6: Set correct streaming headers**

On streaming responses, set:
- `Content-Type: text/event-stream`
- `Cache-Control: no-cache`
- `Connection: keep-alive`

- [ ] **Step 7: Run the HTTP public endpoint tests**

Run: `go test ./tests/e2e -run 'TestChatCompletionsHTTPStreamingOpenAI|TestChatCompletionsHTTPNonStreamingAnthropic|TestResponsesHTTPStreamingOpenAI|TestResponsesHTTPNonStreamingAnthropic' -v`
Expected: PASS

- [ ] **Step 8: Commit the public handler slice**

```bash
git add internal/app/app.go internal/httpapi/router.go internal/httpapi/handlers internal/httpapi/openai tests/e2e/chat_http_test.go tests/e2e/responses_http_test.go tests/e2e/http_test_helpers.go
git commit -m "feat: wire public text APIs into the execution chain"
```

### Task 6: Add Real HTTP Failover and Public Rejection E2E Coverage

**Files:**
- Modify: `tests/e2e/http_test_helpers.go`
- Create: `tests/e2e/failover_http_test.go`
- Modify: `internal/httpapi/handlers/chat_completions.go`
- Modify: `internal/httpapi/handlers/responses.go`

- [ ] **Step 1: Write the failing failover/rejection E2E tests**

```go
func TestChatHTTPFailsOverBeforeOutputCommit(t *testing.T) {
	env := startHTTPTestEnv(t, "primary_429_backup_success")
	resp := postJSON(t, env.Client, env.BaseURL+"/v1/chat/completions", env.Token, chatTextRequest())
	assertStatus(t, resp, http.StatusOK)
	assertBodyContains(t, resp, "chat.completion.chunk")
}

func TestChatHTTPRejectsToolsOnPublicTextPath(t *testing.T) {
	env := startHTTPTestEnv(t, "openai_chat_stream")
	resp := postJSON(t, env.Client, env.BaseURL+"/v1/chat/completions", env.Token, chatToolsRequest())
	assertStatus(t, resp, http.StatusBadRequest)
	assertJSONErrorType(t, resp, "unsupported_capability")
}
```

- [ ] **Step 2: Run the failover/rejection tests to verify failure**

Run: `go test ./tests/e2e -run 'TestChatHTTPFailsOverBeforeOutputCommit|TestChatHTTPDoesNotFailOverAfterOutputCommit|TestChatHTTPRejectsToolsOnPublicTextPath|TestResponsesHTTPRejectsVisionOnPublicTextPath|TestResponsesHTTPRejectsStructuredOutputOnPublicTextPath' -v`
Expected: FAIL because the public text-path gate and/or HTTP failover path is incomplete

- [ ] **Step 3: Add real HTTP test scenarios**

Extend stub helpers to support:
- primary `429`, backup success
- primary partial stream interruption after output
- tools request payload
- vision request payload
- structured output payload

- [ ] **Step 4: Ensure handlers map public text-path rejections before execution**

Rejections must be:
- stable
- explicit
- OpenAI-style

- [ ] **Step 5: Ensure pre-output failover is visible at the HTTP layer**

The response should succeed when fallback occurs before output commitment.

- [ ] **Step 6: Ensure post-output failover does not occur**

The HTTP test should prove only the primary attempt is used after commitment.

- [ ] **Step 7: Run the failover/rejection E2E suite**

Run: `go test ./tests/e2e -run 'TestChatHTTPFailsOverBeforeOutputCommit|TestChatHTTPDoesNotFailOverAfterOutputCommit|TestChatHTTPRejectsToolsOnPublicTextPath|TestResponsesHTTPRejectsVisionOnPublicTextPath|TestResponsesHTTPRejectsStructuredOutputOnPublicTextPath' -v`
Expected: PASS

- [ ] **Step 8: Commit the HTTP failover/rejection slice**

```bash
git add internal/httpapi/handlers tests/e2e/failover_http_test.go tests/e2e/http_test_helpers.go
git commit -m "test: add HTTP failover and rejection coverage"
```

### Task 7: Final Verification for the Public Text API Slice

**Files:**
- Modify: `tests/e2e/chat_http_test.go`
- Modify: `tests/e2e/responses_http_test.go`
- Modify: `tests/e2e/failover_http_test.go`
- Modify: `tests/e2e/http_test_helpers.go`

- [ ] **Step 1: Write or tighten the full online-path matrix**

Ensure the public online matrix covers:
- chat + OpenAI + stream
- chat + OpenAI + non-stream
- chat + Anthropic + stream
- chat + Anthropic + non-stream
- responses + OpenAI + stream
- responses + OpenAI + non-stream
- responses + Anthropic + stream
- responses + Anthropic + non-stream

- [ ] **Step 2: Run the dedicated E2E suite**

Run: `go test ./tests/e2e/... -v`
Expected: PASS

- [ ] **Step 3: Run the full repository test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 4: Build the production binary**

Run: `go build ./cmd/nexus-router`
Expected: PASS

- [ ] **Step 5: Re-run example config boot verification**

Run: `go test ./tests/integration -run TestExampleConfigBoots -v`
Expected: PASS

- [ ] **Step 6: Commit final verification adjustments**

```bash
git add tests/e2e
git commit -m "test: verify public text API online path"
```

## Final Verification Checklist

Before calling this slice complete, run all of the following fresh:

- `go test ./tests/e2e/... -v`
- `go test ./...`
- `go build ./cmd/nexus-router`
- `go test ./tests/integration -run TestExampleConfigBoots -v`

The slice only counts as complete if:

- `/v1/chat/completions` works online for both `stream=true` and `stream=false`
- `/v1/responses` works online for both `stream=true` and `stream=false`
- both OpenAI and Anthropic text paths work online
- unsupported public tools/vision/structured-output requests are rejected with stable errors
- pre-output failover works online
- post-output failover does not occur
- final non-stream responses are aggregated from canonical events
