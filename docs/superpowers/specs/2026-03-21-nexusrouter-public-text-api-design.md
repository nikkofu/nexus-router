# NexusRouter Public Text API Online Path Design

**Date:** 2026-03-21
**Status:** Draft for review
**Scope:** Next mainline slice after the initial data-plane foundation
**Depends on:** `docs/superpowers/specs/2026-03-20-nexusrouter-v1-data-plane-design.md`

## 1. Summary

The next mainline slice is to convert NexusRouter from a collection of validated internal layers into a real online API service for the public text path.

At the end of this slice, the public endpoints:

- `POST /v1/chat/completions`
- `POST /v1/responses`

must both work through a full runtime chain:

- client auth
- request decode
- public-surface validation
- capability validation
- route planning
- provider execution
- failover handling
- streaming or non-stream response encoding

This slice is deliberately narrower than "make everything public." It only exposes the stable text path through the public HTTP surface. Existing internal support for tools, vision, and structured outputs remains in the codebase and test matrix, but those capabilities are explicitly rejected at the public endpoints in this phase.

## 2. Goal

Build a real online request path for the public OpenAI-compatible APIs so a client can send a text request to NexusRouter over HTTP and receive a valid streamed or non-streamed response from either an OpenAI or Anthropic upstream through one shared execution core.

## 3. Non-Goals

This slice does not:

- publicly expose tools
- publicly expose vision
- publicly expose structured outputs
- add config hot reload
- add new provider families
- redesign the provider adapters already built
- add new billing or quota functionality
- add advanced admin dashboards
- split execution into separate kernels for `chat.completions` and `responses`

## 4. Locked Scope Decisions

The following decisions are already fixed for this slice:

- The public endpoints must both support `stream=true` and `stream=false`
- The slice must be validated with real HTTP end-to-end tests, not just handler or package tests
- Only the text path is publicly enabled in this slice
- `stream=false` must be built by aggregating canonical events, not by introducing a second provider-native shortcut path
- Client debugging details remain in logs and `/admin/*`, not in public response headers
- Both `openai/gpt-*` and `anthropic/claude-*` public model families must work through the public text path

## 5. Problem Statement

The current codebase has the necessary building blocks:

- auth policy resolution
- OpenAI-compatible request decoders
- canonical request and event models
- capability registry
- route planner
- health manager and breaker state
- OpenAI and Anthropic provider adapters
- orchestrator-level failover boundary enforcement
- streaming encoders

But the public HTTP endpoints are still not wired into these capabilities. The current handlers do not yet execute the real request path. This means the service is internally capable but externally incomplete.

This slice closes that gap.

## 6. Architecture Overview

### 6.1 New Runtime Layer

Add a runtime executor layer that translates route attempts into real provider calls.

Responsibilities:

- maintain `upstream name -> provider config` mapping
- hold provider-family adapters
- dispatch each attempt to the correct adapter based on configured provider family
- normalize provider adapter failures into `providers.ExecutionError`

This layer should live under a dedicated namespace such as:

- `internal/runtime/executor.go`
- `internal/runtime/registry.go`

### 6.2 New Application Service Layer

Add a thin application service layer between handlers and the runtime/orchestrator stack.

Responsibilities:

- accept decoded `canonical.Request`
- enforce public text-only surface rules for this slice
- run capability validation
- call orchestrator with the runtime executor
- return canonical events or a normalized internal error
- attach route-attempt metadata for logs/admin visibility

This layer should expose one shared execution contract, with endpoint-specific entry methods if helpful:

- `ExecuteChat(...)`
- `ExecuteResponses(...)`

or a single generic method driven by `EndpointKind`.

### 6.3 Existing Layers That Remain

The following existing layers remain intact and are not redesigned:

- capability registry
- route planner
- health manager
- breaker
- OpenAI provider adapter
- Anthropic provider adapter
- orchestrator attempt loop

The work in this slice is primarily composition and public surface wiring, not architecture replacement.

## 7. Handler Boundary Rules

### 7.1 Handler Responsibilities

Each public handler should do exactly these things:

1. read the client policy from request context after auth middleware resolution
2. decode the public OpenAI-compatible request into `canonical.Request`
3. apply public text-path restrictions for this slice
4. invoke the execution service
5. encode the result as stream or non-stream response
6. map failures into OpenAI-compatible error responses

### 7.2 Handler Non-Responsibilities

Handlers must not:

- select route attempts directly
- decide which provider adapter to use
- implement failover logic
- re-implement capability logic
- keep independent execution kernels for chat vs responses
- aggregate final responses inline with ad hoc local variables

## 8. Public Text-Path Contract

### 8.1 `POST /v1/chat/completions`

Publicly accepted fields in this slice:

- `model`
- `messages`
  - `system`
  - `user`
  - `assistant`
- `stream`
- basic generation controls:
  - `temperature`
  - `top_p`
  - `max_tokens`
  - `max_completion_tokens`
  - `stop`

Publicly rejected in this slice:

- `tools`
- `tool_choice`
- image content
- structured output contracts
- audio and non-text modalities

### 8.2 `POST /v1/responses`

Publicly accepted fields in this slice:

- `model`
- `input`
  - text content items
- `stream`
- basic generation controls
- simple metadata if already harmless to preserve

Publicly rejected in this slice:

- input image items
- tool-related structures
- structured output format contracts
- other non-text modalities

### 8.3 Public Rejection Strategy

This slice introduces explicit public-surface rejection before full capability execution for features intentionally deferred from the public surface.

Rejected requests must return:

- HTTP `400`
- OpenAI-style error object
- error type `unsupported_capability`

This is a phase-specific surface contract, not a deletion of internal capability machinery.

## 9. Execution Flow

Every public online request should follow this path:

1. HTTP request arrives
2. auth middleware resolves client token and stores the client policy in request context
3. endpoint handler decodes the request into `canonical.Request`
4. public text-path gate rejects unsupported public-surface features
5. capability registry validates the request
6. planner builds attempt order
7. orchestrator calls runtime executor per attempt
8. runtime executor dispatches to OpenAI or Anthropic adapter
9. provider adapter returns canonical events
10. handler either:
   - streams canonical events to the client, or
   - aggregates canonical events into a final JSON response

This flow must be shared across both public endpoints.

## 10. Streaming and Non-Streaming Strategy

### 10.1 Shared Event Pipeline

All execution paths continue to produce canonical events.

This is a hard rule:

- `stream=true` consumes canonical events as stream output
- `stream=false` aggregates canonical events into final JSON

The system must not create a separate provider-native non-stream fast path for this slice.

The runtime implementation may still choose to drive upstream adapters through an event-producing path even when the public request uses `stream=false`. The public contract is about client output shape, not about forcing a separate upstream request mode.

### 10.2 Streaming Responses

For `stream=true`, handlers must:

- set `Content-Type: text/event-stream`
- set `Cache-Control: no-cache`
- set `Connection: keep-alive`
- flush streamed chunks as events are encoded

Once the first event has been written to the client, the response is considered committed. After commitment, errors may not be rewritten into ordinary JSON responses.

### 10.3 Non-Streaming Responses

For `stream=false`, handlers should aggregate canonical events using dedicated pure functions, for example:

- `AggregateChatCompletion(events, req)`
- `AggregateResponses(events, req)`

This keeps the execution core shared and isolates response finalization logic from the handler.

## 11. Final Response Aggregation

### 11.1 Chat Completions Finalization

The final non-stream chat response should be a stable OpenAI-compatible shape containing at least:

- local NexusRouter-generated response ID
- `object: "chat.completion"`
- `model`
- one assistant choice
- aggregated assistant text content
- normalized finish reason
- optional usage if available

### 11.2 Responses Finalization

The final non-stream responses object should contain at least:

- local NexusRouter-generated response ID
- `object: "response"`
- `model`
- aggregated text output blocks
- completion status
- optional usage if available

### 11.3 ID Policy

The final response ID should be generated by NexusRouter, not copied directly from a provider-native ID. This avoids leaking provider-specific identity semantics into the public consistency contract.

## 12. Runtime Executor Design

The runtime executor is the missing link between planner attempts and real provider calls.

It should:

- accept `upstream name` and `canonical.Request`
- resolve provider config by name
- identify provider family from config
- dispatch to:
  - OpenAI adapter
  - Anthropic adapter
- wrap adapter failures into normalized execution errors where needed

This executor should be the only runtime component that knows how upstream names map to provider families.

## 13. Error Model

### 13.1 Public Error Categories

Public handlers should normalize failures into these categories:

- `auth_error`
- `invalid_request`
- `unsupported_capability`
- `upstream_error`

### 13.2 Category Mapping

Examples:

- missing or invalid bearer token -> `auth_error`
- malformed JSON -> `invalid_request`
- tools/vision/structured output on public text path -> `unsupported_capability`
- no upstream available, timeout, route exhausted, post-commit stream interruption -> `upstream_error`

### 13.3 Public Error Shape

Errors must be returned as:

```json
{
  "error": {
    "type": "unsupported_capability",
    "message": "vision is not enabled on the public text path"
  }
}
```

Provider-native error payloads must not be exposed directly as the public contract.

## 14. Observability for This Slice

### 14.1 Logs

At minimum, per-request logs should include:

- request ID
- endpoint kind
- public model
- selected route group
- route attempts
- winning upstream if successful
- whether failover occurred
- whether response was stream or non-stream
- normalized public error type if failed

### 14.2 Public Headers

This slice does not add public debug headers such as:

- `X-Nexus-Upstream`
- `X-Nexus-Attempts`

Debugging remains internal to logs and admin endpoints.

### 14.3 Admin Endpoints

This slice may strengthen the read-only admin endpoints, but only modestly.

Useful additions include:

- route-group to upstream mapping summary
- provider family per upstream
- current health and breaker state visibility
- public text-path restriction summary

## 15. End-to-End Test Strategy

This slice requires real HTTP end-to-end tests that start NexusRouter and send requests through its public endpoints to local stub upstreams.

Recommended test structure:

- `tests/e2e/http_test_helpers.go`
- `tests/e2e/chat_http_test.go`
- `tests/e2e/responses_http_test.go`
- `tests/e2e/failover_http_test.go`

### 15.1 Required E2E Coverage

The minimum required public online coverage:

- `chat.completions` + OpenAI upstream + `stream=true`
- `chat.completions` + OpenAI upstream + `stream=false`
- `chat.completions` + Anthropic upstream + `stream=true`
- `chat.completions` + Anthropic upstream + `stream=false`
- `responses` + OpenAI upstream + `stream=true`
- `responses` + OpenAI upstream + `stream=false`
- `responses` + Anthropic upstream + `stream=true`
- `responses` + Anthropic upstream + `stream=false`
- failover before output commit
- no failover after output commit
- public rejection of tools
- public rejection of vision
- public rejection of structured outputs

## 16. File Plan

Expected new files:

- `internal/runtime/executor.go`
- `internal/runtime/registry.go`
- `internal/service/execute.go`
- `internal/httpapi/openai/finalize_chat.go`
- `internal/httpapi/openai/finalize_responses.go`
- `tests/e2e/http_test_helpers.go`
- `tests/e2e/chat_http_test.go`
- `tests/e2e/responses_http_test.go`
- `tests/e2e/failover_http_test.go`

Expected modified files:

- `internal/app/app.go`
- `internal/httpapi/router.go`
- `internal/httpapi/middleware.go`
- `internal/httpapi/handlers/chat_completions.go`
- `internal/httpapi/handlers/responses.go`
- `internal/providers/types.go` if route-attempt metadata needs a more formal carrier

## 17. Risks

Primary risks in this slice:

- re-implementing execution logic independently in each handler
- introducing a second non-stream execution path
- letting public-surface rules and capability rules become inconsistent
- leaking provider-specific response shapes back to clients
- building happy-path HTTP handlers without real failover coverage

## 18. Completion Criteria

This slice is complete only when all of the following are true:

- `POST /v1/chat/completions` works online over HTTP
- `POST /v1/chat/completions` supports `stream=true`
- `POST /v1/chat/completions` supports `stream=false`
- `POST /v1/responses` works online over HTTP
- `POST /v1/responses` supports `stream=true`
- `POST /v1/responses` supports `stream=false`
- `openai/gpt-*` public text path works online
- `anthropic/claude-*` public text path works online
- failover before output commitment works online
- failover after output commitment does not occur
- tools are rejected on the public text path
- vision is rejected on the public text path
- structured outputs are rejected on the public text path
- final non-stream responses are produced by canonical event aggregation
- new e2e tests pass
- full `go test ./...` continues to pass

## 19. Planning Handoff

The implementation plan for this slice should:

- keep the shared execution kernel intact
- add the smallest possible runtime composition layer
- keep public text-path rules explicit and local to this slice
- land real e2e tests early so the online path is continuously verified
- avoid any scope creep into public tools, public vision, or public structured outputs
