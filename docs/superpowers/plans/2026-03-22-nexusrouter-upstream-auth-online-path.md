# NexusRouter Upstream Auth Online Path Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make real OpenAI and Anthropic upstream execution requests carry provider auth headers from `api_key_env`, and fail locally before dialing when credentials are missing.

**Architecture:** Add a shared provider auth header builder in `internal/providers`, reuse it from both runtime health probing and live online adapters, and verify behavior through integration and e2e tests. Keep the public API and routing flow unchanged.

**Tech Stack:** Go, `net/http`, existing `config`, `providers`, `health`, `tests/integration`, `tests/e2e`

---

## Planned File Structure

- Create: `internal/providers/auth_headers.go`
- Modify: `internal/providers/openai/adapter.go`
- Modify: `internal/providers/anthropic/adapter.go`
- Modify: `internal/health/prober.go`
- Modify: `tests/integration/openai_stub_helpers_test.go`
- Modify: `tests/integration/anthropic_stub_helpers_test.go`
- Modify: `tests/integration/chat_openai_test.go`
- Modify: `tests/integration/anthropic_translation_test.go`
- Modify: `tests/e2e/http_test_helpers.go`
- Modify: `tests/e2e/chat_http_test.go`

### Task 1: Add Failing Coverage for Online Upstream Auth

**Files:**
- Modify: `tests/integration/openai_stub_helpers_test.go`
- Modify: `tests/integration/anthropic_stub_helpers_test.go`
- Modify: `tests/integration/chat_openai_test.go`
- Modify: `tests/integration/anthropic_translation_test.go`
- Modify: `tests/e2e/http_test_helpers.go`
- Modify: `tests/e2e/chat_http_test.go`

- [ ] **Step 1: Write failing integration tests for OpenAI outbound auth headers**
- [ ] **Step 2: Run the targeted integration tests and verify they fail for missing headers**
- [ ] **Step 3: Write failing integration tests for Anthropic outbound auth headers and missing-key fail-fast**
- [ ] **Step 4: Write a failing e2e public-path test that captures upstream auth headers**
- [ ] **Step 5: Run the targeted e2e test and verify it fails**

### Task 2: Implement Shared Provider Auth Header Resolution

**Files:**
- Create: `internal/providers/auth_headers.go`
- Modify: `internal/health/prober.go`
- Modify: `internal/providers/openai/adapter.go`
- Modify: `internal/providers/anthropic/adapter.go`

- [ ] **Step 1: Add a shared helper that resolves provider auth headers from `config.ProviderConfig`**
- [ ] **Step 2: Update runtime health probing to reuse the shared helper**
- [ ] **Step 3: Update OpenAI and Anthropic adapters to attach resolved headers before dialing**
- [ ] **Step 4: Return local execution/classified errors when headers cannot be resolved**
- [ ] **Step 5: Run targeted integration and e2e suites and verify green**

### Task 3: Run Full Verification

**Files:**
- Verify only

- [ ] **Step 1: Run `go test ./tests/integration -count=1`**
- [ ] **Step 2: Run `go test ./tests/e2e -count=1`**
- [ ] **Step 3: Run `go test ./... -count=1`**
- [ ] **Step 4: Run `git diff --check`**
- [ ] **Step 5: Commit**
