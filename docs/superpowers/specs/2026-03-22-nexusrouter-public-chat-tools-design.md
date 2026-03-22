# NexusRouter Public Chat Tools Online Path Design

**Date:** 2026-03-22
**Status:** Accepted and implemented on `main`
**Scope:** Incremental extension after the public text online path and upstream auth slices
**Depends on:** `docs/superpowers/specs/2026-03-20-nexusrouter-v1-data-plane-design.md`, `docs/superpowers/specs/2026-03-21-nexusrouter-public-text-api-design.md`

## 1. Summary

NexusRouter already has internal tool-capability building blocks:

- canonical `tools` support
- tool schema subset validation
- Anthropic tool translation
- Anthropic `tool_use` stream decoding
- OpenAI-compatible `tool_calls` streaming relay

But the public online API still rejects tools at the outer public text gate. This slice exposes a managed tools contract on the public `chat.completions` path without widening the surface all the way to `responses`, vision, or structured outputs.

## 2. Goal

Allow `POST /v1/chat/completions` to accept managed function tools and emit OpenAI-compatible tool-call streaming responses across both OpenAI and Anthropic upstream routes while preserving the existing shared execution core and strong consistency contract.

## 3. Non-Goals

This slice does not:

- expose tools on `POST /v1/responses`
- add Router-executed tool dispatch or tool-result callbacks
- add vision or structured-output public support
- add public parallel-tool-call tuning
- add raw passthrough tool semantics
- redesign the canonical execution pipeline into a separate tools kernel

## 4. Fixed Scope Decisions

The following decisions are locked for this slice:

- Public tools support lands on `POST /v1/chat/completions` only
- `POST /v1/responses` continues to reject tool-related request structures with `unsupported_capability`
- Only function tools are publicly accepted
- The supported `tool_choice` subset is:
  - implicit default / omitted
  - `"auto"`
  - a named function choice of the form `{"type":"function","function":{"name":"..."}}`
- The following are rejected in this slice:
  - `tool_choice: "required"`
  - `tool_choice: "none"`
  - `allowed_tools`-style constrained sets
  - non-function tools
- `stream=true` is the primary public tool-call contract in this slice
- `stream=false` may return an aggregated assistant message and normalized finish reason, but this slice does not claim a full tool-result lifecycle on the public non-stream surface

## 5. Why This Shape

Three constraints drive this narrower contract:

1. The user-selected product contract is strong consistency across OpenAI and Anthropic managed routes.
2. Anthropic and OpenAI tool interfaces are similar enough for function tools and simple `tool_choice`, but `responses` and richer tool-choice controls widen the semantic gap materially.
3. The existing codebase already proves the hardest translation leg for this slice: Anthropic `tool_use` can already become OpenAI-compatible `tool_calls` stream output.

This makes public `chat.completions` tools the highest-value next step with the smallest contract risk.

## 6. Public API Contract

### 6.1 `POST /v1/chat/completions`

New publicly accepted fields in this slice:

- `tools`
  - only entries with `type: "function"`
  - only the already-supported JSON Schema subset
- `tool_choice`
  - omitted
  - `"auto"`
  - `{"type":"function","function":{"name":"..."}}`

Previously accepted text-path fields remain accepted:

- `model`
- `messages`
- `stream`
- `temperature`
- `top_p`
- `max_tokens`
- `max_completion_tokens`
- `stop`

Still rejected on this endpoint:

- image content
- structured output contracts
- audio and non-text modalities
- unsupported `tool_choice` forms

### 6.2 `POST /v1/responses`

This slice deliberately keeps the existing rejection behavior for:

- `tools`
- tool-related control fields

Rejected requests remain:

- HTTP `400`
- OpenAI-style error object
- `error.type = "unsupported_capability"`

## 7. Managed Tools Contract

### 7.1 Tool Definition Subset

Public tools continue to use the v1 managed schema subset already enforced by `ValidateSchemaSubset(...)`.

This slice does not expand that subset. It only exposes it on the public chat path.

### 7.2 Supported `tool_choice`

The supported subset is intentionally narrower than raw provider-native capability:

- omitted / default
- `"auto"`
- specific named function

This is chosen because it maps cleanly to both providers:

- OpenAI chat completions accepts `auto` and a named function tool choice
- Anthropic Messages supports `tool_choice` modes including `auto` and a named tool selection

Rejected forms are rejected locally before execution rather than best-effort translated.

### 7.3 Tool Results Are Out Of Scope

This slice supports model-issued tool calls, not Router-mediated tool execution.

That means:

- the model may emit tool-call deltas
- NexusRouter relays those deltas in OpenAI-compatible form
- NexusRouter does not execute the tool
- NexusRouter does not yet expose a fully managed tool-result continuation contract on the public online path

## 8. Canonical and Validation Changes

### 8.1 Public Gate Rule

The existing public text gate must become endpoint-aware:

- `chat.completions`
  - tools allowed for this slice
  - vision and structured outputs still rejected
- `responses`
  - tools still rejected
  - vision and structured outputs still rejected

### 8.2 Capability Validation

After the public gate:

- policy `AllowTools` still applies
- model-family `SupportsTools` still applies
- tool schemas still pass the managed subset validator

This preserves one shared capability pipeline.

### 8.3 Decode Rules

`DecodeChatCompletionRequest(...)` must:

- preserve function tools as canonical tools
- decode the supported `tool_choice` subset into canonical `ToolChoice`
- reject unsupported `tool_choice` shapes as `invalid_request`

`DecodeResponsesRequest(...)` remains unchanged for tools in this slice except that public handlers still reject them later.

## 9. Provider Translation

### 9.1 OpenAI Upstream

OpenAI upstream chat requests should encode:

- `tools`
- supported `tool_choice`
- existing streaming and usage settings

This route is mostly conservative passthrough after local compatibility validation.

### 9.2 Anthropic Upstream

Anthropic upstream Messages requests should encode:

- `tools` as current `input_schema` definitions
- supported `tool_choice` mapped into Anthropic Messages semantics

This route remains a strict translation path from the canonical request.

## 10. Response Contract

### 10.1 Streaming

For `stream=true`, the public response contract is:

- OpenAI-compatible chat-completion SSE
- tool call deltas emitted through `tool_calls`
- finish reasons normalized into OpenAI-compatible values

This is the primary public tools contract in this slice.

### 10.2 Non-Streaming

For `stream=false`, NexusRouter continues to aggregate canonical events into a final chat response.

This slice only guarantees:

- aggregated assistant text if present
- normalized finish reason if tool calling ends the turn
- no broken or contradictory final shape when tool-call events occur

This slice does **not** claim a full public non-stream tool-result lifecycle beyond those guarantees.

## 11. Error Model

Public handler normalization remains:

- malformed tool payload or unsupported `tool_choice` shape -> `invalid_request`
- tools not allowed by this endpoint phase contract -> `unsupported_capability`
- tools blocked by client policy or model-family support -> existing validation error mapping
- route exhaustion / upstream failures -> `upstream_error`

Provider-native error bodies remain hidden from the public contract.

## 12. Testing Strategy

This slice must add:

- service tests proving:
  - public chat tools are allowed through the public gate
  - public responses tools are still rejected
- e2e tests proving:
  - chat tools no longer fail with `unsupported_capability`
  - Anthropic chat tools stream as OpenAI-compatible `tool_calls`
  - responses tools remain rejected
- integration tests proving:
  - OpenAI chat request encoding includes `tools`
  - OpenAI chat request encoding includes supported `tool_choice`
  - unsupported `tool_choice` shapes are rejected locally

## 13. Acceptance Criteria

- `POST /v1/chat/completions` accepts managed function tools on both OpenAI and Anthropic routes
- `POST /v1/chat/completions` supports the chosen `tool_choice` subset
- Anthropic tool-use streaming is publicly available through OpenAI-compatible `tool_calls`
- `POST /v1/responses` still rejects tools with `unsupported_capability`
- no second execution core is introduced for tools
- all new behavior is covered by integration and e2e tests

## 14. Research Inputs

This slice is aligned with the provider docs as checked on 2026-03-22:

- OpenAI Chat Completions tool-choice support: `auto`, `none`, `required`, and named function tool choice
  - https://platform.openai.com/docs/api-reference/chat/create-chat-completion
  - https://platform.openai.com/docs/api-reference/chat/create-completion
- Anthropic tool-use and `tool_choice` support including `auto`, `any`, `tool`, and `none`
  - https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/implement-tool-use
  - https://docs.anthropic.com/en/release-notes/api
