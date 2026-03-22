# NexusRouter Public Vision Online Path Design

**Date:** 2026-03-23
**Status:** Draft for review
**Scope:** Incremental public-surface expansion after public text and public chat tools
**Depends on:** `docs/superpowers/specs/2026-03-20-nexusrouter-v1-data-plane-design.md`, `docs/superpowers/specs/2026-03-21-nexusrouter-public-text-api-design.md`, `docs/superpowers/specs/2026-03-22-nexusrouter-public-chat-tools-design.md`

## 1. Summary

NexusRouter already has the internal building blocks for image inputs:

- canonical image content blocks
- OpenAI-compatible request decoders for chat and responses image inputs
- OpenAI upstream request encoders for chat and responses image inputs
- Anthropic request translation for image content
- capability validation for policy-level and model-family-level vision support

But the current public online surface still rejects image content during the public phase gate. This slice exposes a managed public vision contract on both `POST /v1/chat/completions` and `POST /v1/responses` without widening into provider-private image features, file handles, image output, or structured-output combinations.

## 2. Goal

Allow both public OpenAI-style endpoints to accept managed image inputs and route them across OpenAI and Anthropic upstreams through the existing shared execution core while preserving the strong consistency contract already chosen for NexusRouter.

## 3. Non-Goals

This slice does not:

- add image generation or image output APIs
- add public support for base64 image payloads
- add public support for `file_id` or provider file handles
- add provider-private image options such as detail/quality knobs
- add structured outputs combined with vision
- add `responses` tools
- redesign the canonical execution pipeline
- add new provider families

## 4. Fixed Scope Decisions

The following decisions are locked for this slice:

- Public vision support lands on both:
  - `POST /v1/chat/completions`
  - `POST /v1/responses`
- The publicly supported image-input subset is:
  - remote `http://` or `https://` image URLs only
  - image inputs attached to `user` content only
- Mixed text and image input remains supported within the same user turn
- `chat.completions` keeps the current managed tools contract
- `responses` continues to reject tools with `unsupported_capability`
- Structured outputs remain rejected on both endpoints
- Non-stream requests continue to use the existing canonical-event aggregation path rather than a second provider-native shortcut path

## 5. Why This Shape

Three constraints drive this narrower public contract:

1. The user-selected product promise is strong consistency across managed OpenAI and Anthropic routes.
2. Both providers support image inputs, but the broader image-input surface is not naturally isomorphic:
   - OpenAI accepts several shapes depending on endpoint and product family
   - Anthropic supports URL, base64, and file-oriented image sources with provider-specific details
3. Remote image URLs are the clearest stable intersection that can be validated locally and translated consistently through the current architecture.

This makes "URL-only public vision on both endpoints" the highest-value next slice with the lowest semantic risk.

## 6. Public API Contract

### 6.1 `POST /v1/chat/completions`

Publicly accepted content forms in this slice:

- existing text-only messages
- mixed user content arrays containing:
  - `{"type":"text","text":"..."}`
  - `{"type":"image_url","image_url":{"url":"https://example.com/cat.png"}}`

Publicly accepted endpoint capabilities after this slice:

- text
- vision
- the already-shipped managed function tools subset

Still rejected on this endpoint:

- image inputs on non-`user` roles
- `data:` URLs
- file handles / `file_id`
- provider-private image options
- structured output contracts
- audio and other non-text modalities

### 6.2 `POST /v1/responses`

Publicly accepted content forms in this slice:

- existing text-only input items
- mixed user input content arrays containing:
  - `{"type":"input_text","text":"..."}`
  - `{"type":"input_image","image_url":"https://example.com/cat.png"}`

Publicly accepted endpoint capabilities after this slice:

- text
- vision

Still rejected on this endpoint:

- image inputs on non-`user` roles
- `data:` URLs
- file handles / `file_id`
- tools
- structured output contracts
- other non-text modalities beyond this managed image subset

## 7. Managed Vision Contract

### 7.1 Supported Image Source Subset

The public contract accepts only remote image URLs with:

- scheme `http`
- scheme `https`

The following are intentionally not public in this slice:

- `data:` URLs
- raw base64 payloads
- `file_id`
- vendor-specific image references

### 7.2 Role Restrictions

To keep cross-provider behavior consistent, public image inputs are accepted only on `user` turns / items.

This slice does not expose:

- system images
- assistant images
- tool-role images

### 7.3 Mixed Content Rules

Both public endpoints must continue to accept mixed text and image content within the same user input.

Examples that remain valid after this slice:

- text followed by one image
- text followed by multiple images
- multiple text blocks surrounding an image

The router does not impose provider-private image-count tuning in this slice beyond any future general runtime protection limits.

## 8. Public Validation and Error Model

### 8.1 Public Phase Gate

The current public text-only gate must be generalized into a public managed-surface gate:

- `chat.completions`
  - tools allowed
  - vision allowed
  - structured outputs still rejected
- `responses`
  - vision allowed
  - tools still rejected
  - structured outputs still rejected

This preserves the existing layering:

1. public surface gate
2. capability validation
3. route planning
4. provider execution
5. stream or aggregate response encoding

### 8.2 Local Request Validation

The public decoders and/or validation helpers must reject malformed image inputs locally when:

- image URL field is missing
- image URL is empty
- image URL scheme is not `http` or `https`
- image content is attached to a non-`user` role
- image item shape does not match the public endpoint contract

These errors should be treated as malformed public requests rather than upstream failures.

### 8.3 Error Mapping

Error mapping remains normalized into the public OpenAI-style contract:

- malformed image payloads or invalid remote URLs -> `invalid_request`
- image-input shapes intentionally not exposed in this phase, such as `data:` URLs or `file_id` -> `unsupported_capability`
- client policy blocks vision -> existing validation error mapping
- model family does not support vision -> existing validation error mapping
- route exhaustion / upstream failures -> `upstream_error`

Provider-native error bodies remain hidden from the public contract.

## 9. Decode Rules

### 9.1 Chat Decode

`DecodeChatCompletionRequest(...)` already recognizes OpenAI-style image items. This slice tightens and publicly codifies the accepted subset:

- string content remains text-only
- array content may include:
  - `text`
  - `image_url`
- image URL must be extracted from `image_url.url`
- invalid public image forms should fail locally

### 9.2 Responses Decode

`DecodeResponsesRequest(...)` already recognizes `input_image` items. This slice tightens and publicly codifies the accepted subset:

- image URL must be supplied as a remote URL string
- invalid public image forms should fail locally
- image items on non-`user` roles should fail locally

## 10. Provider Translation

### 10.1 OpenAI Upstream

OpenAI upstream requests should continue to encode image inputs in native OpenAI-compatible form:

- chat -> `image_url.url`
- responses -> `input_image.image_url`

No provider-private image tuning is added in this slice.

### 10.2 Anthropic Upstream

Anthropic Messages requests should translate image inputs into Anthropic content blocks using URL-based image sources only.

This slice expects Anthropic translation to normalize public image inputs as:

- `{"type":"image","source":{"type":"url","url":"https://..."}}`

This slice does not expose Anthropic base64 or file source types publicly.

## 11. Response Contract

### 11.1 Streaming

For `stream=true`, the response contract remains unchanged from the client perspective:

- `chat.completions` returns OpenAI-compatible chat-completion SSE
- `responses` returns OpenAI-compatible responses SSE

Vision affects input semantics only in this slice. No new public stream event types are introduced.

### 11.2 Non-Streaming

For `stream=false`, the existing canonical event aggregation path remains in force.

This slice guarantees:

- no second non-stream execution path is introduced
- final responses remain OpenAI-compatible
- finish reasons and usage behavior continue to follow the current aggregation rules

## 12. Testing Strategy

This slice must add:

- service tests proving:
  - public chat vision is allowed through the public gate
  - public responses vision is allowed through the public gate
  - responses tools are still rejected
  - structured outputs are still rejected
- integration tests proving:
  - chat decode accepts the public image subset
  - responses decode accepts the public image subset
  - malformed / non-public image forms are rejected locally
  - OpenAI upstream requests include image content on both endpoints
  - Anthropic translation emits URL-based image source blocks
- e2e tests proving:
  - `chat.completions` vision succeeds against OpenAI and Anthropic routes
  - `responses` vision succeeds against OpenAI and Anthropic routes
  - `data:` URL or other non-public image forms are rejected as expected

## 13. Acceptance Criteria

- `POST /v1/chat/completions` accepts managed image inputs on both OpenAI and Anthropic routes
- `POST /v1/responses` accepts managed image inputs on both OpenAI and Anthropic routes
- the public image-input subset is limited to remote `http/https` URLs on `user` inputs
- `chat.completions` keeps the current tools support
- `responses` still rejects tools with `unsupported_capability`
- structured outputs still reject publicly on both endpoints
- non-stream requests still aggregate canonical events instead of taking a provider-native shortcut
- all new behavior is covered by integration and e2e tests

## 14. Research Inputs

This slice is aligned with the provider docs as checked on 2026-03-23:

- OpenAI vision guide
  - <https://developers.openai.com/api/docs/guides/images-vision>
- OpenAI Chat Completions API reference
  - <https://platform.openai.com/docs/api-reference/chat/create-chat-completion>
- OpenAI Responses input items reference
  - <https://platform.openai.com/docs/api-reference/responses/input-items?lang=curl>
- Anthropic vision guide
  - <https://docs.anthropic.com/en/docs/build-with-claude/vision>
- Anthropic Messages examples
  - <https://docs.anthropic.com/en/api/messages-examples>
