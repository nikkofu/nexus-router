package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/runtime"
)

func TestRuntimeRegistryResolvesProviderByUpstreamName(t *testing.T) {
	registry := runtime.NewRegistry([]config.ProviderConfig{
		{
			Name:     "openai-main",
			Provider: "openai",
			BaseURL:  "https://api.openai.com",
		},
	})

	got, err := registry.Resolve("openai-main")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Name != "openai-main" {
		t.Fatalf("Name = %q, want %q", got.Name, "openai-main")
	}
	if got.Provider != "openai" {
		t.Fatalf("Provider = %q, want %q", got.Provider, "openai")
	}
}

func TestRuntimeExecutorDispatchesOpenAIAdapterForOpenAIProvider(t *testing.T) {
	server, capture := newProviderDispatchStubServer(t)
	defer server.Close()

	executor := runtime.NewExecutor(
		runtime.NewRegistry([]config.ProviderConfig{
			{
				Name:     "openai-main",
				Provider: "openai",
				BaseURL:  server.URL,
			},
		}),
		server.Client(),
		nil,
	)

	result, err := executor.Execute(context.Background(), "openai-main", canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "openai/gpt-4.1",
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if capture.Path() != "/v1/chat/completions" {
		t.Fatalf("path = %q, want %q", capture.Path(), "/v1/chat/completions")
	}
	if len(result.Events) == 0 {
		t.Fatal("expected at least one event")
	}
}

func TestRuntimeExecutorDispatchesAnthropicAdapterForAnthropicProvider(t *testing.T) {
	server, capture := newProviderDispatchStubServer(t)
	defer server.Close()

	executor := runtime.NewExecutor(
		runtime.NewRegistry([]config.ProviderConfig{
			{
				Name:     "anthropic-main",
				Provider: "anthropic",
				BaseURL:  server.URL,
			},
		}),
		server.Client(),
		nil,
	)

	result, err := executor.Execute(context.Background(), "anthropic-main", canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "anthropic/claude-sonnet-4-5",
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if capture.Path() != "/v1/messages" {
		t.Fatalf("path = %q, want %q", capture.Path(), "/v1/messages")
	}
	if len(result.Events) == 0 {
		t.Fatal("expected at least one event")
	}
}

func TestRuntimeExecutorWrapsUnknownUpstreamError(t *testing.T) {
	executor := runtime.NewExecutor(runtime.NewRegistry(nil), nil, nil)

	_, err := executor.Execute(context.Background(), "missing-upstream", canonical.Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing-upstream") {
		t.Fatalf("error = %q, want upstream name included", err.Error())
	}
}

func TestRuntimeExecutorWrapsUnknownProviderError(t *testing.T) {
	executor := runtime.NewExecutor(
		runtime.NewRegistry([]config.ProviderConfig{
			{
				Name:     "mystery-main",
				Provider: "mystery",
				BaseURL:  "https://example.com",
			},
		}),
		nil,
		nil,
	)

	_, err := executor.Execute(context.Background(), "mystery-main", canonical.Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "mystery-main") || !strings.Contains(err.Error(), "mystery") {
		t.Fatalf("error = %q, want upstream and provider names included", err.Error())
	}
}
