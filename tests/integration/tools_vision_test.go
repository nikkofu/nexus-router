package integration

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nikkofu/nexus-router/internal/auth"
	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/capabilities"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/providers/anthropic"
	"github.com/nikkofu/nexus-router/internal/streaming"
)

func TestRejectsVisionWhenPolicyDisallowsIt(t *testing.T) {
	req := canonical.Request{
		PublicModel: "openai/gpt-4.1",
		Conversation: []canonical.Turn{
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{
					{
						Type: canonical.ContentTypeImage,
						Image: &canonical.ImageInput{
							URL:      "https://example.com/cat.png",
							MIMEType: "image/png",
						},
					},
				},
			},
		},
	}

	policy := auth.ClientPolicy{
		ID:             "no-vision",
		AllowVision:    false,
		AllowTools:     true,
		AllowStreaming: true,
	}

	err := capabilities.ValidateRequest(capabilities.DefaultRegistry(), policy, req)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestAcceptsManagedModelFamily(t *testing.T) {
	req := canonical.Request{
		PublicModel: "anthropic/claude-sonnet-4-5",
	}

	policy := auth.ClientPolicy{
		ID:              "default",
		AllowVision:     true,
		AllowTools:      true,
		AllowStructured: true,
		AllowStreaming:  true,
	}

	if err := capabilities.ValidateRequest(capabilities.DefaultRegistry(), policy, req); err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
}

func TestRejectsUnsupportedToolSchema(t *testing.T) {
	req := canonical.Request{
		PublicModel: "anthropic/claude-sonnet-4-5",
		Tools: []canonical.Tool{
			{
				Name: "lookup_weather",
				Schema: map[string]any{
					"type": "object",
					"oneOf": []any{
						map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	policy := auth.ClientPolicy{
		ID:             "no-tools",
		AllowVision:    true,
		AllowTools:     true,
		AllowStreaming: true,
	}

	err := capabilities.ValidateRequest(capabilities.DefaultRegistry(), policy, req)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestVisionRequestMapsToAnthropicBlocks(t *testing.T) {
	server, capture := newAnthropicStubServer(t, "messages_stream")
	adapter := anthropic.NewAdapter(server.Client())

	_, err := adapter.Execute(context.Background(), config.ProviderConfig{
		BaseURL: server.URL,
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "anthropic/claude-sonnet-4-5",
		Conversation: []canonical.Turn{
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{
					{
						Type: canonical.ContentTypeText,
						Text: "describe this image",
					},
					{
						Type: canonical.ContentTypeImage,
						Image: &canonical.ImageInput{
							URL:      "https://example.com/cat.png",
							MIMEType: "image/png",
						},
					},
				},
			},
		},
		Stream: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	body := capture.Body()
	if !strings.Contains(body, `"type":"image"`) {
		t.Fatalf("captured body missing image block: %s", body)
	}
	if !strings.Contains(body, `"url":"https://example.com/cat.png"`) {
		t.Fatalf("captured body missing image URL: %s", body)
	}
}

func TestToolCallStreamsInOpenAICompatibleShape(t *testing.T) {
	server, capture := newAnthropicStubServer(t, "tool_use_stream")
	adapter := anthropic.NewAdapter(server.Client())

	result, err := adapter.Execute(context.Background(), config.ProviderConfig{
		BaseURL: server.URL,
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "anthropic/claude-sonnet-4-5",
		Tools: []canonical.Tool{
			{
				Name: "lookup_weather",
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
					},
					"required": []any{"city"},
				},
			},
		},
		Conversation: []canonical.Turn{
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentTypeText,
					Text: "weather in shanghai",
				}},
			},
		},
		Stream: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(capture.Body(), `"tools":[`) {
		t.Fatalf("captured body missing tools array: %s", capture.Body())
	}
	if !strings.Contains(capture.Body(), `"name":"lookup_weather"`) {
		t.Fatalf("captured body missing tool name: %s", capture.Body())
	}

	rec := httptest.NewRecorder()
	if err := streaming.WriteChatCompletionSSE(rec, result.Events); err != nil {
		t.Fatalf("WriteChatCompletionSSE() error = %v", err)
	}

	if !strings.Contains(rec.Body.String(), `"tool_calls"`) {
		t.Fatalf("expected OpenAI tool_calls in body, got %q", rec.Body.String())
	}
}
