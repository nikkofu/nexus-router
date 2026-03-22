# NexusRouter Upstream Auth Online Path Design

**Date:** 2026-03-22
**Status:** Draft for implementation
**Scope:** Incremental slice after public text online path and runtime health

## 1. Summary

NexusRouter already validates client API keys locally and exposes provider `api_key_env` in configuration, but the live online provider adapters do not yet attach provider authentication headers to outbound requests.

This slice makes the real online path capable of authenticating to configured OpenAI and Anthropic upstreams with the same default header rules already used by runtime health probes.

## 2. Goal

Ensure real upstream execution requests from NexusRouter carry the correct provider authentication headers and fail locally before dialing when required provider credentials are not configured.

## 3. Non-Goals

This slice does not:

- add new provider families
- expose client-supplied upstream credentials
- add passthrough mode
- add per-request header override APIs
- redesign provider adapters around a new transport abstraction

## 4. Locked Decisions

- OpenAI upstream requests use `Authorization: Bearer <env value>`
- Anthropic upstream requests use:
  - `x-api-key: <env value>`
  - `anthropic-version: 2023-06-01`
- Missing `api_key_env` or missing/blank environment variable is a local configuration/runtime error
- The same provider-default header rules must be shared by runtime health probes and online inference adapters

## 5. Design

### 5.1 Shared Header Builder

Introduce one shared helper under `internal/providers` that:

- accepts `config.ProviderConfig`
- resolves `api_key_env`
- returns provider-default outbound headers
- returns a stable fail-fast error when credentials are missing

This helper becomes the single source of truth for provider-default auth headers.

### 5.2 Probe Reuse

`internal/health/prober.go` should stop owning its own provider auth merge rules. It should reuse the shared helper, then layer configured probe header overrides on top where appropriate.

### 5.3 Online Adapter Use

`internal/providers/openai/adapter.go` and `internal/providers/anthropic/adapter.go` should call the shared helper before sending an upstream request:

- on success, copy the returned headers onto the outbound request
- on failure, return a classified/local execution error without dialing

### 5.4 Error Behavior

If provider auth cannot be resolved locally:

- no upstream HTTP request is sent
- the adapter returns an internal execution error
- failover remains governed by the existing orchestrator/runtime error semantics

## 6. Testing

The slice must add:

- integration tests that capture outbound online adapter headers for OpenAI and Anthropic
- integration tests proving missing `api_key_env` or missing env var fails without dialing
- an e2e test proving the public online path sends provider auth headers through the configured upstream

## 7. Acceptance Criteria

- real OpenAI online adapter requests carry bearer auth from `api_key_env`
- real Anthropic online adapter requests carry `x-api-key` and `anthropic-version`
- missing upstream credentials fail locally before network dial
- runtime health probes and online adapters use one shared provider auth rule set
