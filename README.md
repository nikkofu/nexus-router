# NexusRouter

NexusRouter is a single-binary, configuration-driven AI gateway that exposes an OpenAI-compatible public API while routing requests to managed OpenAI and Anthropic upstreams.

The current project is intentionally focused on the v1 data plane. It does not try to be a generic raw reverse proxy, and it does not include a control plane, billing, tenant dashboards, or external infrastructure dependencies. Instead, it enforces a narrow managed compatibility contract: if NexusRouter accepts a request, that request must fit a tested feature slice that behaves consistently across the supported provider families.

## Core Guarantees

NexusRouter is opinionated about the boundary between accepted and rejected traffic:

- OpenAI-style public ingress, even when the selected upstream is Anthropic
- Managed family aliases instead of unrestricted raw model passthrough
- Local static API key authentication owned by NexusRouter
- Provider credentials kept server-side and never supplied by clients
- Consistent rejection for unsupported public shapes instead of vague best-effort forwarding
- Single-process failover based on local health, breaker state, and ordered fallback routing

## Current Status

The repository currently implements the following mainline slices on `main`:

- OpenAI-compatible public ingress for:
  - `POST /v1/chat/completions`
  - `POST /v1/responses`
- NexusRouter-owned static API key authentication from local config
- Managed model families:
  - `openai/gpt-*`
  - `anthropic/claude-*`
- Shared execution core for both endpoints
- Streaming and non-streaming response handling
- Active health probes, circuit breaking, temporary upstream ejection, and ordered failover
- Managed chat tools on `chat.completions`
- Managed public vision on both endpoints
- OpenAI passthrough-style upstream request encoding after validation
- Anthropic request translation through a canonical internal request model
- Read-only admin and health endpoints
- Conformance, integration, and end-to-end test coverage

## What NexusRouter Supports Today

### Public Endpoints

- `POST /v1/chat/completions`
- `POST /v1/responses`
- `GET /livez`
- `GET /readyz`
- `GET /admin/config`
- `GET /admin/routes`
- `GET /admin/upstreams`

If `server.admin_listen_addr` is configured, admin endpoints are served on the admin listener; otherwise they are mounted on the main listener.

### Auth Model

Clients authenticate with NexusRouter API keys from local YAML config.

- Clients send `Authorization: Bearer <sk-nx-...>`
- Each key is mapped to a local client policy
- Policies can allow or block:
  - model patterns
  - streaming
  - tools
  - vision
  - structured outputs

The `allow_structured` policy bit already exists in config and admin output, but the current public surface still rejects structured output contracts on both endpoints.

Upstream provider credentials are not supplied by clients. They come from server-side environment variables referenced by config.

### Routing Model

Routing is config-driven:

- public model pattern -> route group
- route group -> primary upstream + ordered fallbacks
- runtime health + breaker state influence execution selection

The service is designed as a managed compatibility layer, not unrestricted provider passthrough.

### Health, Breaker, and Failover

Execution selection is influenced by both active probes and request-time feedback:

- Providers are probed on a configurable interval
- Default probe target is `/v1/models` for both OpenAI and Anthropic providers
- Request failures can open the local breaker and temporarily eject an upstream
- Recovery is driven by probe success thresholds
- `GET /readyz` stays unready until required route groups have at least one eligible upstream

### Managed Capability Matrix

Current public contract:

| Capability | `chat.completions` | `responses` |
|---|---|---|
| Text | Yes | Yes |
| Streaming SSE | Yes | Yes |
| Non-stream aggregation | Yes | Yes |
| Function tools | Yes | No |
| Vision input | Yes | Yes |
| Structured outputs | No | No |

### Vision Contract

Public vision support is intentionally narrow:

- Only remote `http://` and `https://` image URLs are accepted
- Images are accepted only on `user` content
- Mixed text + image input is supported
- `chat.completions` accepted image form:
  - `{"type":"image_url","image_url":{"url":"https://..."}}`
- `responses` accepted image form:
  - `{"type":"input_image","image_url":"https://..."}`

Rejected image forms include:

- `data:` URLs
- `file_id`
- provider-private image options
- image payloads on non-`user` roles
- malformed or mismatched image item shapes

Error typing is normalized as:

- malformed or empty remote image payloads -> `invalid_request`
- intentionally unsupported public forms such as `data:` and `file_id` -> `unsupported_capability`

### Tools Contract

Managed tools are exposed only on `POST /v1/chat/completions`.

- Only function tools are accepted
- `responses` still rejects tools with `unsupported_capability`
- Tool-call output is normalized into OpenAI-compatible chat streaming semantics

### Upstream Translation Contract

OpenAI upstream encoding is pinned to native-compatible request shapes:

- chat vision -> `image_url.url`
- responses vision -> `input_image.image_url`

Anthropic upstream vision translation is pinned to a URL-only source shape:

```json
{
  "type": "image",
  "source": {
    "type": "url",
    "url": "https://example.com/cat.png"
  }
}
```

No base64 or file-backed image translation is exposed in the current public contract.

## What NexusRouter Does Not Support Yet

The current repository does not implement:

- control plane services
- user accounts, organizations, or dashboard UX
- billing or quota products
- Redis, PostgreSQL, Kafka, ClickHouse, or any required external dependency
- semantic caching
- auto-routing by latency or cost preference
- public structured outputs
- public `responses` tools
- public base64/file-backed vision
- additional provider families beyond OpenAI and Anthropic

## Architecture

NexusRouter is organized around a small set of explicit layers:

- `internal/httpapi`
  - OpenAI-compatible request decode, public handlers, response finalization, middleware
- `internal/canonical`
  - provider-neutral request and event model
- `internal/capabilities`
  - model-family matching and managed capability validation
- `internal/router`
  - route planning from config plus fallback ordering
- `internal/health`
  - runtime upstream health state and probing
- `internal/circuitbreaker`
  - local breaker behavior and temporary ejection
- `internal/runtime`
  - upstream registry and executor dispatch
- `internal/providers/openai`
  - OpenAI-native request/stream handling
- `internal/providers/anthropic`
  - Anthropic request translation and stream normalization
- `internal/service`
  - shared execution service used by both public endpoints
- `internal/app`
  - process assembly and server lifecycle

The central rule is that HTTP decode, capability validation, route planning, provider translation, and streaming normalization remain separate concerns.

## Getting Started

### Prerequisites

- Go 1.24+
- an OpenAI API key if you want to route to OpenAI
- an Anthropic API key if you want to route to Anthropic

### 1. Build

```bash
go build ./cmd/nexus-router
```

### 2. Configure provider credentials

```bash
export OPENAI_API_KEY=your-openai-key
export ANTHROPIC_API_KEY=your-anthropic-key
```

### 3. Start from the example config

Example config:

- `configs/nexus-router.example.yaml`

Run:

```bash
./nexus-router -config configs/nexus-router.example.yaml
```

Default example listeners:

- public API: `127.0.0.1:8080`
- admin API: `127.0.0.1:9090`

## Configuration Overview

The top-level config sections are:

- `server`
  - public/admin listen addresses and optional TLS file mode
- `auth`
  - static client keys and per-key policy
- `models`
  - public model pattern to route-group mapping
- `providers`
  - upstream definitions, base URLs, auth env var names, optional probes
- `routing`
  - primary and fallback upstream ordering
- `health`
  - probe interval, timeout, and initial readiness behavior
- `breaker`
  - open interval and failure/recovery thresholds
- `limits`
  - request size guardrails

Minimal example:

```yaml
auth:
  client_keys:
    - id: local-dev
      secret: sk-nx-local-dev
      active: true
      allowed_model_patterns:
        - openai/gpt-*
        - anthropic/claude-*
      allow_streaming: true
      allow_tools: true
      allow_vision: true
      allow_structured: true

models:
  - pattern: openai/gpt-*
    route_group: openai-family
  - pattern: anthropic/claude-*
    route_group: anthropic-family

routing:
  route_groups:
    - name: openai-family
      primary: openai-main
    - name: anthropic-family
      primary: anthropic-main
```

For full details, see:

- `configs/nexus-router.example.yaml`

## Example Requests

### Chat text

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H 'Authorization: Bearer sk-nx-local-dev' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "openai/gpt-4.1",
    "stream": true,
    "messages": [
      {"role": "user", "content": "hello"}
    ]
  }'
```

### Chat vision

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H 'Authorization: Bearer sk-nx-local-dev' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "anthropic/claude-sonnet-4-5",
    "stream": true,
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "describe this image"},
          {"type": "image_url", "image_url": {"url": "https://example.com/cat.png"}}
        ]
      }
    ]
  }'
```

### Responses vision

```bash
curl http://127.0.0.1:8080/v1/responses \
  -H 'Authorization: Bearer sk-nx-local-dev' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "openai/gpt-4.1",
    "stream": true,
    "input": [
      {
        "role": "user",
        "content": [
          {"type": "input_text", "text": "describe this image"},
          {"type": "input_image", "image_url": "https://example.com/cat.png"}
        ]
      }
    ]
  }'
```

## Testing

The repository has three main test layers:

- `tests/conformance`
  - provider and stream normalization behavior
- `tests/integration`
  - decode, capability, translation, and runtime component behavior
- `tests/e2e`
  - public HTTP contract, failover, health, middleware, and finalization

Useful commands:

```bash
go test ./tests/integration -count=1
go test ./tests/e2e -count=1
go test ./... -count=1
```

## Design Documents

The implementation in this repository follows the staged design docs under `docs/superpowers/specs/`:

- `2026-03-20-nexusrouter-v1-data-plane-design.md`
- `2026-03-21-nexusrouter-public-text-api-design.md`
- `2026-03-22-nexusrouter-public-chat-tools-design.md`
- `2026-03-23-nexusrouter-public-vision-design.md`

The original product brief is in:

- `docs/RFP.md`

## Repository Layout

```text
cmd/nexus-router/                 CLI entrypoint
configs/                          example configuration
internal/app/                     process wiring and server lifecycle
internal/auth/                    client key resolution
internal/canonical/               canonical request and event types
internal/capabilities/            managed capability validation
internal/circuitbreaker/          breaker state
internal/health/                  probes and runtime health
internal/httpapi/                 public and admin HTTP surface
internal/providers/openai/        OpenAI upstream adapter
internal/providers/anthropic/     Anthropic upstream adapter
internal/router/                  route planning
internal/runtime/                 upstream executor and registry
internal/service/                 shared execution service
internal/streaming/               SSE encoding and relay
tests/                            conformance, integration, and e2e tests
```

## Project Direction

The current direction remains:

- single process
- single binary
- no mandatory external dependencies
- data plane first
- strong managed compatibility contract
- provider health and failover before broader surface area

That means NexusRouter prefers explicit local rejection over vague best-effort passthrough whenever a request would otherwise escape the tested compatibility matrix.
