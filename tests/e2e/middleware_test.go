package e2e

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/httpapi"
)

func TestRequireBearerStoresResolvedClientPolicyInContext(t *testing.T) {
	resolver := newTestResolver(config.ClientKeyConfig{
		ID:                   "client-a",
		Secret:               "test-token",
		Active:               true,
		AllowedModelPatterns: []string{"openai/*"},
		AllowStreaming:       true,
		AllowTools:           true,
		AllowVision:          false,
		AllowStructured:      true,
	})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		policy, ok := httpapi.ClientPolicyFromContext(r.Context())
		if !ok {
			t.Fatal("ClientPolicyFromContext() ok = false, want true")
		}
		if policy.ID != "client-a" {
			t.Fatalf("policy.ID = %q, want %q", policy.ID, "client-a")
		}
		if !policy.AllowStreaming {
			t.Fatal("policy.AllowStreaming = false, want true")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	handler := httpapi.RequireBearer(resolver, next)
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRequireBearerRejectsMissingBearerToken(t *testing.T) {
	handler := httpapi.RequireBearer(newTestResolver(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec.Body.String() != "missing bearer token\n" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "missing bearer token\n")
	}
}

func TestRequireBearerRejectsInvalidBearerToken(t *testing.T) {
	resolver := newTestResolver(config.ClientKeyConfig{
		ID:     "client-a",
		Secret: "expected-token",
		Active: true,
	})
	handler := httpapi.RequireBearer(resolver, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec.Body.String() != "invalid bearer token\n" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "invalid bearer token\n")
	}
}
