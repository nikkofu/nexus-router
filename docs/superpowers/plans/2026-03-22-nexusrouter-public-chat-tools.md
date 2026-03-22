# NexusRouter Public Chat Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose managed function tools on `POST /v1/chat/completions` while keeping `POST /v1/responses` tool requests rejected and preserving the shared OpenAI-compatible public contract.

**Architecture:** This slice keeps the existing single execution core and extends only the managed compatibility layer around it. Public chat decoding starts accepting the chosen tools subset, provider encoders learn to forward normalized tool data to OpenAI and Anthropic upstreams, and the public service gate becomes endpoint-aware so chat tools pass while responses tools still fail with `unsupported_capability`.

**Tech Stack:** Go, standard library `net/http` and `encoding/json`, existing internal `canonical`, `capabilities`, `service`, `providers`, `streaming`, `httptest`

---

**Execution notes:**

- Work on `main` is allowed in this repository.
- Use `@superpowers/test-driven-development` before each implementation task.
- Keep the public contract narrow for this slice:
  - `chat.completions` accepts function `tools`
  - `chat.completions` accepts `tool_choice` only when omitted, `"auto"`, or named function form
  - `responses` still rejects tools with `unsupported_capability`
- Do not add public vision, structured outputs, router-side tool execution, or tool-result continuation in this plan.
- Prefer keeping `canonical.ToolChoice` as named-choice-only unless a failing test proves a richer internal representation is necessary.

## Planned File Structure

### Public request decoding and validation

- Modify: `internal/httpapi/openai/chat_encode.go`
- Modify: `internal/httpapi/openai/chat_types.go`
- Modify: `internal/capabilities/validate.go`
- Modify: `internal/service/execute.go`

### Provider request encoding

- Modify: `internal/providers/openai/request.go`
- Modify: `internal/providers/anthropic/request.go`

### Integration coverage

- Modify: `tests/integration/chat_openai_test.go`
- Modify: `tests/integration/anthropic_translation_test.go`
- Modify: `tests/integration/tools_vision_test.go`

### End-to-end coverage and fixtures

- Modify: `tests/e2e/http_test_helpers.go`
- Modify: `tests/e2e/service_test.go`
- Modify: `tests/e2e/chat_http_test.go`
- Modify: `tests/e2e/failover_http_test.go`
- Modify: `tests/e2e/responses_http_test.go`

## Responsibility Map

- `internal/httpapi/openai/chat_encode.go`: Decode public `chat.completions` tools and the supported `tool_choice` subset into canonical request fields, rejecting unsupported shapes as `invalid_request`.
- `internal/capabilities/validate.go`: Enforce the public endpoint phase contract so chat tools are allowed but responses tools still fail early with `unsupported_capability`.
- `internal/service/execute.go`: Keep the public gate ahead of shared capability validation and orchestration without duplicating provider-specific logic.
- `internal/providers/openai/request.go`: Forward normalized tools and named tool-choice to OpenAI chat upstream requests.
- `internal/providers/anthropic/request.go`: Translate normalized tools and named tool-choice into Anthropic Messages request semantics.
- `tests/integration/*`: Lock down decode rules and provider payload encoding details.
- `tests/e2e/*`: Lock down the public HTTP contract, especially Anthropic tool-call streaming and continued responses rejection.

### Task 1: Decode Managed Chat Tools and Tool Choice

**Files:**
- Modify: `internal/httpapi/openai/chat_encode.go`
- Modify: `internal/httpapi/openai/chat_types.go`
- Test: `tests/integration/chat_openai_test.go`

- [ ] **Step 1: Write the failing decode tests**

Add focused tests in `tests/integration/chat_openai_test.go` for:

```go
func TestChatHandlerDecodesManagedTools(t *testing.T) {
	reqBody := `{
		"model":"openai/gpt-4.1",
		"stream":true,
		"messages":[{"role":"user","content":"weather?"}],
		"tools":[{
			"type":"function",
			"function":{
				"name":"lookup_weather",
				"parameters":{"type":"object","properties":{"city":{"type":"string"}}}
			}
		}]
	}`

	got, err := openaiapi.DecodeChatCompletionRequest(strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("DecodeChatCompletionRequest() error = %v", err)
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "lookup_weather" {
		t.Fatalf("tools = %#v, want lookup_weather", got.Tools)
	}
}
```

Also add tests proving:
- `"tool_choice":"auto"` is accepted
- named function `tool_choice` populates `canonical.ToolChoice{Name: "..."}`
- `"tool_choice":"required"` and `"tool_choice":"none"` fail with a local decode error
- malformed named tool-choice objects fail with a local decode error

- [ ] **Step 2: Run the decode tests to verify RED**

Run: `go test ./tests/integration -run 'TestChatHandlerDecodesManagedTools|TestChatHandlerAcceptsAutoToolChoice|TestChatHandlerDecodesNamedToolChoice|TestChatHandlerRejectsUnsupportedToolChoice' -count=1`
Expected: FAIL because chat decoding does not yet handle `tool_choice`

- [ ] **Step 3: Implement minimal chat decode support**

Add a small helper in `internal/httpapi/openai/chat_encode.go` that:
- preserves function tools exactly as today
- accepts empty `tool_choice`
- accepts `"auto"` and maps it to the zero-value canonical tool choice
- accepts `{"type":"function","function":{"name":"..."}}` and maps the name into `canonical.ToolChoice`
- rejects unsupported strings and malformed objects with stable `invalid_request`-style errors

Keep `internal/httpapi/openai/chat_types.go` minimal; use `json.RawMessage` for `tool_choice` unless a failing test proves a typed struct is clearer.

- [ ] **Step 4: Re-run the decode tests to verify GREEN**

Run: `go test ./tests/integration -run 'TestChatHandlerDecodesManagedTools|TestChatHandlerAcceptsAutoToolChoice|TestChatHandlerDecodesNamedToolChoice|TestChatHandlerRejectsUnsupportedToolChoice' -count=1`
Expected: PASS

- [ ] **Step 5: Commit the decode slice**

```bash
git add internal/httpapi/openai/chat_encode.go internal/httpapi/openai/chat_types.go tests/integration/chat_openai_test.go
git commit -m "feat: decode managed chat tools"
```

### Task 2: Encode Tools for OpenAI and Anthropic Upstreams

**Files:**
- Modify: `internal/providers/openai/request.go`
- Modify: `internal/providers/anthropic/request.go`
- Test: `tests/integration/chat_openai_test.go`
- Test: `tests/integration/anthropic_translation_test.go`
- Test: `tests/integration/tools_vision_test.go`

- [ ] **Step 1: Write the failing provider-encoding tests**

Add targeted tests for OpenAI request capture:

```go
func TestOpenAIAdapterEncodesManagedTools(t *testing.T) {
	server, capture := newOpenAICaptureStubServer(t, "chat_stream")
	adapter := openai.NewAdapter(server.Client())

	_, err := adapter.Execute(context.Background(), config.ProviderConfig{
		BaseURL: server.URL,
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "openai/gpt-4.1",
		Tools: []canonical.Tool{{Name: "lookup_weather", Schema: map[string]any{"type":"object"}}},
		Stream: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(capture.Body(), `"tools":[`) {
		t.Fatalf("captured body missing tools: %s", capture.Body())
	}
}
```

Add tests proving:
- OpenAI named `ToolChoice` becomes chat payload `tool_choice`
- Anthropic named `ToolChoice` becomes Messages `tool_choice`
- Anthropic tool encoding still includes `input_schema`
- existing tool stream translation remains green

- [ ] **Step 2: Run the provider-encoding tests to verify RED**

Run: `go test ./tests/integration -run 'TestOpenAIAdapterEncodesManagedTools|TestOpenAIAdapterEncodesNamedToolChoice|TestAnthropicRouteEncodesNamedToolChoice|TestToolCallStreamsInOpenAICompatibleShape' -count=1`
Expected: FAIL because OpenAI encoding omits tools/tool-choice and Anthropic encoding omits tool-choice

- [ ] **Step 3: Implement minimal upstream payload support**

In `internal/providers/openai/request.go`:
- add `tools` for chat-completions requests
- add named `tool_choice` when `req.ToolChoice.Name != ""`
- keep responses encoding unchanged for this slice

In `internal/providers/anthropic/request.go`:
- keep existing `tools` translation
- add Anthropic `tool_choice` only for named function selection:

```go
payload["tool_choice"] = map[string]any{
	"type": "tool",
	"name": req.ToolChoice.Name,
}
```

Do not force `"auto"` upstream; omission should preserve default managed behavior on both providers.

- [ ] **Step 4: Re-run the provider-encoding tests to verify GREEN**

Run: `go test ./tests/integration -run 'TestOpenAIAdapterEncodesManagedTools|TestOpenAIAdapterEncodesNamedToolChoice|TestAnthropicRouteEncodesNamedToolChoice|TestToolCallStreamsInOpenAICompatibleShape' -count=1`
Expected: PASS

- [ ] **Step 5: Commit the provider-encoding slice**

```bash
git add internal/providers/openai/request.go internal/providers/anthropic/request.go tests/integration/chat_openai_test.go tests/integration/anthropic_translation_test.go tests/integration/tools_vision_test.go
git commit -m "feat: encode managed tool requests upstream"
```

### Task 3: Make the Public Gate Endpoint-Aware

**Files:**
- Modify: `internal/capabilities/validate.go`
- Modify: `internal/service/execute.go`
- Test: `tests/e2e/service_test.go`

- [ ] **Step 1: Write the failing service tests**

Replace the old blanket tools-rejection case with two explicit service tests:

```go
func TestPublicTextServiceAllowsChatTools(t *testing.T) {
	planner := &stubPlanner{plan: router.Plan{Attempts: []router.Attempt{{Upstream: "anthropic-main"}}}}
	executor := &stubExecutor{result: providers.Result{}}
	svc := service.NewExecuteService(capabilities.DefaultRegistry(), planner, executor)

	_, _, err := svc.Execute(context.Background(), allowedPolicy(), canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "anthropic/claude-sonnet-4-5",
		Tools:        []canonical.Tool{{Name: "lookup_weather", Schema: map[string]any{"type":"object"}}},
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}
```

Also add a companion test proving:
- `EndpointKindResponses` plus tools still returns `service.ErrUnsupportedCapability`
- image content still fails
- structured output still fails

- [ ] **Step 2: Run the service tests to verify RED**

Run: `go test ./tests/e2e -run 'TestPublicTextServiceAllowsChatTools|TestPublicTextServiceRejectsResponsesTools|TestPublicTextServiceRejectsUnsupportedCapabilities' -count=1`
Expected: FAIL because `ValidatePublicTextOnly` still rejects all tools

- [ ] **Step 3: Implement endpoint-aware public validation**

Refactor `internal/capabilities/validate.go` so the public gate:
- allows `req.Tools` when `req.EndpointKind == canonical.EndpointKindChatCompletions`
- rejects `req.Tools` with `ErrUnsupportedCapability` when `req.EndpointKind == canonical.EndpointKindResponses`
- continues rejecting image content and structured outputs on both endpoints

Keep `internal/service/execute.go` calling the public gate before `ValidateRequest` so `unsupported_capability` remains reserved for public-surface phase mismatches.

- [ ] **Step 4: Re-run the service tests to verify GREEN**

Run: `go test ./tests/e2e -run 'TestPublicTextServiceAllowsChatTools|TestPublicTextServiceRejectsResponsesTools|TestPublicTextServiceRejectsUnsupportedCapabilities' -count=1`
Expected: PASS

- [ ] **Step 5: Commit the public-gate slice**

```bash
git add internal/capabilities/validate.go internal/service/execute.go tests/e2e/service_test.go
git commit -m "feat: allow managed tools on public chat endpoint"
```

### Task 4: Lock the Public HTTP Contract with Tool E2E Tests

**Files:**
- Modify: `tests/e2e/http_test_helpers.go`
- Modify: `tests/e2e/chat_http_test.go`
- Modify: `tests/e2e/failover_http_test.go`
- Modify: `tests/e2e/responses_http_test.go`

- [ ] **Step 1: Write the failing HTTP tests and fixtures**

Extend the HTTP stub fixtures with an Anthropic tool-use scenario, for example:

```go
case "anthropic_tool_use":
	if r.URL.Path != "/v1/messages" {
		http.Error(w, "unexpected path", http.StatusNotFound)
		return true
	}
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = fmt.Fprint(w, "event: content_block_start\ndata: {\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"lookup_weather\"}}\n\n")
	_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"delta\":{\"partial_json\":\"{\\\"city\\\":\\\"Shanghai\\\"}\"}}\n\n")
	_, _ = fmt.Fprint(w, "event: message_delta\ndata: {\"delta\":{\"stop_reason\":\"tool_use\"}}\n\n")
	_, _ = fmt.Fprint(w, "event: message_stop\ndata: {}\n\n")
```

Add HTTP tests proving:
- `POST /v1/chat/completions` with tools against an Anthropic route returns `200` and includes OpenAI-compatible `"tool_calls"`
- the captured upstream body includes `"tools":[` and, for a named tool-choice request, the provider-specific selection payload
- `POST /v1/responses` with tools still returns `400` with `error.type == "unsupported_capability"`

Update the old blanket-rejection chat test in `tests/e2e/failover_http_test.go` so it no longer expects chat-tools rejection.

- [ ] **Step 2: Run the HTTP tests to verify RED**

Run: `go test ./tests/e2e -run 'TestChatCompletionsHTTPStreamingAnthropicTools|TestResponsesHTTPRejectsToolsOnPublicSurface|TestChatHTTPRejectsToolsOnPublicTextPath' -count=1`
Expected: FAIL because no Anthropic tool fixture exists and chat tools are still rejected or unverified

- [ ] **Step 3: Implement the minimal HTTP fixture and request helpers**

In `tests/e2e/http_test_helpers.go`:
- add a scenario and route config for Anthropic tool-use streaming
- add helper request builders for chat tools, named `tool_choice`, and responses tools

Do not add public `responses` tool execution support; the rejection test is part of the contract.

- [ ] **Step 4: Re-run the HTTP tests to verify GREEN**

Run: `go test ./tests/e2e -run 'TestChatCompletionsHTTPStreamingAnthropicTools|TestResponsesHTTPRejectsToolsOnPublicSurface' -count=1`
Expected: PASS

- [ ] **Step 5: Commit the HTTP contract slice**

```bash
git add tests/e2e/http_test_helpers.go tests/e2e/chat_http_test.go tests/e2e/failover_http_test.go tests/e2e/responses_http_test.go
git commit -m "test: cover public chat tool contract"
```

### Task 5: Run Full Verification and Clean Up

**Files:**
- Modify: `docs/superpowers/specs/2026-03-22-nexusrouter-public-chat-tools-design.md` only if implementation reveals a real contract correction
- Verify: `tests/integration/*`
- Verify: `tests/e2e/*`
- Verify: full repository test suite

- [ ] **Step 1: Run the focused integration suite**

Run: `go test ./tests/integration -count=1`
Expected: PASS

- [ ] **Step 2: Run the focused e2e suite**

Run: `go test ./tests/e2e -count=1`
Expected: PASS

- [ ] **Step 3: Run the full repository suite**

Run: `go test ./... -count=1`
Expected: PASS

- [ ] **Step 4: Run whitespace / patch hygiene checks**

Run: `git diff --check`
Expected: no output

- [ ] **Step 5: Commit any final fixups**

```bash
git add docs/superpowers/specs/2026-03-22-nexusrouter-public-chat-tools-design.md
git commit -m "docs: align public chat tools spec" # only if docs changed
```

- [ ] **Step 6: Prepare handoff notes**

Document:
- whether explicit `"auto"` is normalized to omission internally
- which endpoint still rejects tools
- which upstream payload shapes are now guaranteed by tests
