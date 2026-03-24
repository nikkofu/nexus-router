package integration

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nikkofu/nexus-router/internal/auth"
	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/capabilities"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/providers/anthropic"
	"github.com/nikkofu/nexus-router/internal/providers/openai"
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
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	messages, ok := payload["messages"].([]any)
	if !ok {
		t.Fatalf("messages = %#v, want array", payload["messages"])
	}

	var imageBlock map[string]any
	for _, rawMessage := range messages {
		message, ok := rawMessage.(map[string]any)
		if !ok {
			continue
		}
		content, ok := message["content"].([]any)
		if !ok {
			continue
		}
		for _, rawBlock := range content {
			block, ok := rawBlock.(map[string]any)
			if !ok {
				continue
			}
			if block["type"] == "image" {
				imageBlock = block
				break
			}
		}
		if imageBlock != nil {
			break
		}
	}

	if imageBlock == nil {
		t.Fatalf("captured body missing image block: %s", body)
	}

	source, ok := imageBlock["source"].(map[string]any)
	if !ok {
		t.Fatalf("image.source = %#v, want object", imageBlock["source"])
	}
	if source["type"] != "url" {
		t.Fatalf("image.source.type = %#v, want %q", source["type"], "url")
	}
	if source["url"] != "https://example.com/cat.png" {
		t.Fatalf("image.source.url = %#v, want %q", source["url"], "https://example.com/cat.png")
	}
	if _, ok := source["media_type"]; ok {
		t.Fatalf("image.source.media_type should be omitted, got %#v", source["media_type"])
	}
	if _, ok := source["data"]; ok {
		t.Fatalf("image.source.data should be omitted, got %#v", source["data"])
	}
	if _, ok := source["file"]; ok {
		t.Fatalf("image.source.file should be omitted, got %#v", source["file"])
	}
	if _, ok := source["file_id"]; ok {
		t.Fatalf("image.source.file_id should be omitted, got %#v", source["file_id"])
	}
	if len(source) != 2 {
		t.Fatalf("image.source = %#v, want only type and url", source)
	}
}

func TestOpenAIAdapterEncodesChatVision(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-test-key")

	server, capture := newOpenAICaptureStubServer(t, "chat_stream")
	adapter := openai.NewAdapter(server.Client())

	_, err := adapter.Execute(context.Background(), config.ProviderConfig{
		Provider:  "openai",
		BaseURL:   server.URL,
		APIKeyEnv: "OPENAI_API_KEY",
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "openai/gpt-4.1",
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

	var payload map[string]any
	if err := json.Unmarshal([]byte(capture.Body()), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	messages, ok := payload["messages"].([]any)
	if !ok {
		t.Fatalf("messages = %#v, want array", payload["messages"])
	}

	var imageBlock map[string]any
	for _, rawMessage := range messages {
		message, ok := rawMessage.(map[string]any)
		if !ok {
			continue
		}
		content, ok := message["content"].([]any)
		if !ok {
			continue
		}
		for _, rawBlock := range content {
			block, ok := rawBlock.(map[string]any)
			if !ok {
				continue
			}
			if block["type"] == "image_url" {
				imageBlock = block
				break
			}
		}
		if imageBlock != nil {
			break
		}
	}

	if imageBlock == nil {
		t.Fatalf("captured body missing image_url block: %s", capture.Body())
	}
	imageURL, ok := imageBlock["image_url"].(map[string]any)
	if !ok {
		t.Fatalf("image_url = %#v, want object", imageBlock["image_url"])
	}
	if imageURL["url"] != "https://example.com/cat.png" {
		t.Fatalf("image_url.url = %#v, want %q", imageURL["url"], "https://example.com/cat.png")
	}
}

func TestOpenAIAdapterEncodesResponsesVision(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-test-key")

	server, capture := newOpenAICaptureStubServer(t, "responses_stream")
	adapter := openai.NewAdapter(server.Client())

	_, err := adapter.Execute(context.Background(), config.ProviderConfig{
		Provider:  "openai",
		BaseURL:   server.URL,
		APIKeyEnv: "OPENAI_API_KEY",
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindResponses,
		PublicModel:  "openai/gpt-4.1",
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

	var payload map[string]any
	if err := json.Unmarshal([]byte(capture.Body()), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	input, ok := payload["input"].([]any)
	if !ok {
		t.Fatalf("input = %#v, want array", payload["input"])
	}

	var imageBlock map[string]any
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		content, ok := item["content"].([]any)
		if !ok {
			continue
		}
		for _, rawBlock := range content {
			block, ok := rawBlock.(map[string]any)
			if !ok {
				continue
			}
			if block["type"] == "input_image" {
				imageBlock = block
				break
			}
		}
		if imageBlock != nil {
			break
		}
	}

	if imageBlock == nil {
		t.Fatalf("captured body missing input_image block: %s", capture.Body())
	}
	imageURL, ok := imageBlock["image_url"].(string)
	if !ok {
		t.Fatalf("input_image.image_url = %#v, want string", imageBlock["image_url"])
	}
	if imageURL != "https://example.com/cat.png" {
		t.Fatalf("input_image.image_url = %#v, want %q", imageURL, "https://example.com/cat.png")
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
