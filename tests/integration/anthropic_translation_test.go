package integration

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/providers/anthropic"
	"github.com/nikkofu/nexus-router/internal/streaming"
)

func TestAnthropicRouteTranslatesChatRequest(t *testing.T) {
	server, capture := newAnthropicStubServer(t, "messages_stream")
	adapter := anthropic.NewAdapter(server.Client())

	_, err := adapter.Execute(context.Background(), config.ProviderConfig{
		BaseURL: server.URL,
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "anthropic/claude-sonnet-4-5",
		Conversation: []canonical.Turn{
			{
				Role: canonical.RoleSystem,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentTypeText,
					Text: "be concise",
				}},
			},
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentTypeText,
					Text: "hello anthropic",
				}},
			},
		},
		Stream: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(capture.Body(), `"system":"be concise"`) {
		t.Fatalf("captured body missing system prompt: %s", capture.Body())
	}
	if !strings.Contains(capture.Body(), `"text":"hello anthropic"`) {
		t.Fatalf("captured body missing user text: %s", capture.Body())
	}
}

func TestAnthropicStreamBecomesOpenAIChunks(t *testing.T) {
	server, _ := newAnthropicStubServer(t, "messages_stream")
	adapter := anthropic.NewAdapter(server.Client())

	result, err := adapter.Execute(context.Background(), config.ProviderConfig{
		BaseURL: server.URL,
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "anthropic/claude-sonnet-4-5",
		Conversation: []canonical.Turn{
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentTypeText,
					Text: "hello anthropic",
				}},
			},
		},
		Stream: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	rec := httptest.NewRecorder()
	if err := streaming.WriteChatCompletionSSE(rec, result.Events); err != nil {
		t.Fatalf("WriteChatCompletionSSE() error = %v", err)
	}

	if !strings.Contains(rec.Body.String(), "chat.completion.chunk") {
		t.Fatalf("expected chat chunk in body, got %q", rec.Body.String())
	}
}

func TestAnthropicResponsesRouteNormalizesOutput(t *testing.T) {
	server, _ := newAnthropicStubServer(t, "messages_stream")
	adapter := anthropic.NewAdapter(server.Client())

	result, err := adapter.Execute(context.Background(), config.ProviderConfig{
		BaseURL: server.URL,
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindResponses,
		PublicModel:  "anthropic/claude-sonnet-4-5",
		Conversation: []canonical.Turn{
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentTypeText,
					Text: "hello anthropic",
				}},
			},
		},
		Stream: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	rec := httptest.NewRecorder()
	if err := streaming.WriteResponsesSSE(rec, result.Events); err != nil {
		t.Fatalf("WriteResponsesSSE() error = %v", err)
	}

	if !strings.Contains(rec.Body.String(), "response.output_text.delta") {
		t.Fatalf("expected response event in body, got %q", rec.Body.String())
	}
}

func TestAnthropicRouteEncodesNamedToolChoice(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-test-key")

	server, capture := newAnthropicStubServer(t, "messages_stream")
	adapter := anthropic.NewAdapter(server.Client())

	_, err := adapter.Execute(context.Background(), config.ProviderConfig{
		Provider:  "anthropic",
		BaseURL:   server.URL,
		APIKeyEnv: "ANTHROPIC_API_KEY",
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "anthropic/claude-sonnet-4-5",
		Tools: []canonical.Tool{
			{
				Name: "lookup_weather",
				Schema: map[string]any{
					"type": "object",
				},
			},
		},
		ToolChoice: canonical.ToolChoice{Name: "lookup_weather"},
		Stream:     true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(capture.Body()), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	rawChoice, ok := payload["tool_choice"].(map[string]any)
	if !ok {
		t.Fatalf("tool_choice = %#v, want object", payload["tool_choice"])
	}
	if rawChoice["type"] != "tool" {
		t.Fatalf("tool_choice.type = %#v, want %q", rawChoice["type"], "tool")
	}
	if rawChoice["name"] != "lookup_weather" {
		t.Fatalf("tool_choice.name = %#v, want %q", rawChoice["name"], "lookup_weather")
	}
}

func TestAnthropicAdapterSendsProviderAuthHeaders(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-test-key")

	server, capture := newAnthropicStubServer(t, "messages_stream")
	adapter := anthropic.NewAdapter(server.Client())

	_, err := adapter.Execute(context.Background(), config.ProviderConfig{
		Provider:  "anthropic",
		BaseURL:   server.URL,
		APIKeyEnv: "ANTHROPIC_API_KEY",
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "anthropic/claude-sonnet-4-5",
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := capture.Header("x-api-key"); got != "anthropic-test-key" {
		t.Fatalf("x-api-key = %q, want %q", got, "anthropic-test-key")
	}
	if got := capture.Header("anthropic-version"); got != "2023-06-01" {
		t.Fatalf("anthropic-version = %q, want %q", got, "2023-06-01")
	}
}

func TestAnthropicAdapterFailsFastWithoutConfiguredEnvValue(t *testing.T) {
	server, capture := newAnthropicStubServer(t, "messages_stream")
	adapter := anthropic.NewAdapter(server.Client())

	_, err := adapter.Execute(context.Background(), config.ProviderConfig{
		Provider:  "anthropic",
		BaseURL:   server.URL,
		APIKeyEnv: "ANTHROPIC_API_KEY",
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "anthropic/claude-sonnet-4-5",
		Stream:       true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := capture.Hits(); got != 0 {
		t.Fatalf("upstream hits = %d, want 0", got)
	}
}
