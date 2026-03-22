package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/config"
	openaiapi "github.com/nikkofu/nexus-router/internal/httpapi/openai"
	"github.com/nikkofu/nexus-router/internal/providers/openai"
	"github.com/nikkofu/nexus-router/internal/streaming"
)

func TestChatHandlerNormalizesMessagesIntoCanonicalRequest(t *testing.T) {
	reqBody := `{
		"model": "openai/gpt-4.1",
		"stream": true,
		"messages": [
			{"role": "system", "content": "be concise"},
			{"role": "user", "content": "hello"}
		]
	}`

	got, err := openaiapi.DecodeChatCompletionRequest(strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("DecodeChatCompletionRequest() error = %v", err)
	}

	if got.PublicModel != "openai/gpt-4.1" {
		t.Fatalf("PublicModel = %q, want %q", got.PublicModel, "openai/gpt-4.1")
	}
	if !got.Stream {
		t.Fatal("Stream = false, want true")
	}
	if len(got.Conversation) != 2 {
		t.Fatalf("conversation len = %d, want 2", len(got.Conversation))
	}
	if got.Conversation[0].Role != canonical.RoleSystem {
		t.Fatalf("first role = %q, want %q", got.Conversation[0].Role, canonical.RoleSystem)
	}
	if got.Conversation[1].Content[0].Text != "hello" {
		t.Fatalf("user content = %q, want %q", got.Conversation[1].Content[0].Text, "hello")
	}
}

func TestChatHandlerDecodesVisionContentBlocks(t *testing.T) {
	reqBody := `{
		"model":"openai/gpt-4.1",
		"stream":true,
		"messages":[{
			"role":"user",
			"content":[
				{"type":"text","text":"describe"},
				{"type":"image_url","image_url":{"url":"https://example.com/cat.png"}}
			]
		}]
	}`

	got, err := openaiapi.DecodeChatCompletionRequest(strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("DecodeChatCompletionRequest() error = %v", err)
	}

	if len(got.Conversation) != 1 {
		t.Fatalf("conversation len = %d, want 1", len(got.Conversation))
	}
	if len(got.Conversation[0].Content) != 2 {
		t.Fatalf("content len = %d, want 2", len(got.Conversation[0].Content))
	}
	if got.Conversation[0].Content[0].Type != canonical.ContentTypeText {
		t.Fatalf("first block type = %q, want text", got.Conversation[0].Content[0].Type)
	}
	if got.Conversation[0].Content[1].Type != canonical.ContentTypeImage {
		t.Fatalf("second block type = %q, want image", got.Conversation[0].Content[1].Type)
	}
	if got.Conversation[0].Content[1].Image == nil {
		t.Fatal("second block image = nil, want non-nil image payload")
	}
	if got.Conversation[0].Content[1].Image.URL != "https://example.com/cat.png" {
		t.Fatalf("image url = %q, want %q", got.Conversation[0].Content[1].Image.URL, "https://example.com/cat.png")
	}
}

func TestChatHandlerRejectsUnsupportedPublicImageForms(t *testing.T) {
	cases := []struct {
		name          string
		payload       string
		wantSemantics string
	}{
		{
			name: "bare string image url",
			payload: `{
				"model":"openai/gpt-4.1",
				"messages":[{"role":"user","content":[{"type":"image_url","image_url":"https://example.com/cat.png"}]}]
			}`,
			wantSemantics: "invalid_request",
		},
		{
			name: "malformed image item shape",
			payload: `{
				"model":"openai/gpt-4.1",
				"messages":[{"role":"user","content":[{"type":"image_url","image_url":["https://example.com/cat.png"]}]}]
			}`,
			wantSemantics: "invalid_request",
		},
		{
			name: "missing image url",
			payload: `{
				"model":"openai/gpt-4.1",
				"messages":[{"role":"user","content":[{"type":"image_url","image_url":{}}]}]
			}`,
			wantSemantics: "invalid_request",
		},
		{
			name: "empty image url",
			payload: `{
				"model":"openai/gpt-4.1",
				"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":""}}]}]
			}`,
			wantSemantics: "invalid_request",
		},
		{
			name: "non-user image role",
			payload: `{
				"model":"openai/gpt-4.1",
				"messages":[{"role":"assistant","content":[{"type":"image_url","image_url":{"url":"https://example.com/cat.png"}}]}]
			}`,
			wantSemantics: "invalid_request",
		},
		{
			name: "unknown image role",
			payload: `{
				"model":"openai/gpt-4.1",
				"messages":[{"role":"moderator","content":[{"type":"image_url","image_url":{"url":"https://example.com/cat.png"}}]}]
			}`,
			wantSemantics: "invalid_request",
		},
		{
			name: "non-http image url scheme",
			payload: `{
				"model":"openai/gpt-4.1",
				"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"ftp://example.com/cat.png"}}]}]
			}`,
			wantSemantics: "invalid_request",
		},
		{
			name: "data url image",
			payload: `{
				"model":"openai/gpt-4.1",
				"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}]
			}`,
			wantSemantics: "unsupported_capability",
		},
		{
			name: "file id image form",
			payload: `{
				"model":"openai/gpt-4.1",
				"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"file_id":"file_abc123"}}]}]
			}`,
			wantSemantics: "unsupported_capability",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := openaiapi.DecodeChatCompletionRequest(strings.NewReader(tc.payload))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantSemantics) {
				t.Fatalf("error = %q, want %q semantics", err.Error(), tc.wantSemantics)
			}
		})
	}
}

func TestChatHandlerDecodesManagedTools(t *testing.T) {
	reqBody := `{
		"model": "openai/gpt-4.1",
		"stream": true,
		"messages": [
			{"role": "user", "content": "weather?"}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "lookup_weather",
					"parameters": {
						"type": "object",
						"properties": {
							"city": {"type": "string"}
						}
					}
				}
			}
		]
	}`

	got, err := openaiapi.DecodeChatCompletionRequest(strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("DecodeChatCompletionRequest() error = %v", err)
	}

	if len(got.Tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(got.Tools))
	}
	if got.Tools[0].Name != "lookup_weather" {
		t.Fatalf("tool name = %q, want %q", got.Tools[0].Name, "lookup_weather")
	}
}

func TestChatHandlerAcceptsAutoToolChoice(t *testing.T) {
	reqBody := `{
		"model": "openai/gpt-4.1",
		"stream": true,
		"messages": [
			{"role": "user", "content": "weather?"}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "lookup_weather",
					"parameters": {"type": "object"}
				}
			}
		],
		"tool_choice": "auto"
	}`

	got, err := openaiapi.DecodeChatCompletionRequest(strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("DecodeChatCompletionRequest() error = %v", err)
	}

	if got.ToolChoice.Name != "" {
		t.Fatalf("tool choice name = %q, want empty", got.ToolChoice.Name)
	}
}

func TestChatHandlerDecodesNamedToolChoice(t *testing.T) {
	reqBody := `{
		"model": "openai/gpt-4.1",
		"stream": true,
		"messages": [
			{"role": "user", "content": "weather?"}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "lookup_weather",
					"parameters": {"type": "object"}
				}
			}
		],
		"tool_choice": {
			"type": "function",
			"function": {
				"name": "lookup_weather"
			}
		}
	}`

	got, err := openaiapi.DecodeChatCompletionRequest(strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("DecodeChatCompletionRequest() error = %v", err)
	}

	if got.ToolChoice.Name != "lookup_weather" {
		t.Fatalf("tool choice name = %q, want %q", got.ToolChoice.Name, "lookup_weather")
	}
}

func TestChatHandlerRejectsUnsupportedToolChoice(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{
			name: "required string",
			payload: `{
				"model": "openai/gpt-4.1",
				"stream": true,
				"messages": [{"role": "user", "content": "weather?"}],
				"tool_choice": "required"
			}`,
		},
		{
			name: "none string",
			payload: `{
				"model": "openai/gpt-4.1",
				"stream": true,
				"messages": [{"role": "user", "content": "weather?"}],
				"tool_choice": "none"
			}`,
		},
		{
			name: "malformed named object",
			payload: `{
				"model": "openai/gpt-4.1",
				"stream": true,
				"messages": [{"role": "user", "content": "weather?"}],
				"tool_choice": {"type": "function", "function": {}}
			}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := openaiapi.DecodeChatCompletionRequest(strings.NewReader(tc.payload))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "tool_choice") {
				t.Fatalf("error = %q, want tool_choice validation failure", err.Error())
			}
		})
	}
}

func TestUnsupportedCapabilityReturnsOpenAIError(t *testing.T) {
	rec := httptest.NewRecorder()
	openaiapi.WriteError(rec, http.StatusBadRequest, "unsupported_capability", "vision not allowed")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var payload struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Error.Type != "unsupported_capability" {
		t.Fatalf("error.type = %q, want %q", payload.Error.Type, "unsupported_capability")
	}
	if payload.Error.Message != "vision not allowed" {
		t.Fatalf("error.message = %q, want %q", payload.Error.Message, "vision not allowed")
	}
}

func TestChatStreamingRelaysOpenAIChunks(t *testing.T) {
	server := newOpenAIStubServer(t, "chat_stream")
	adapter := openai.NewAdapter(server.Client())

	result, err := adapter.Execute(context.Background(), config.ProviderConfig{
		BaseURL: server.URL,
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "openai/gpt-4.1",
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	rec := httptest.NewRecorder()
	if err := streaming.WriteChatCompletionSSE(rec, result.Events); err != nil {
		t.Fatalf("WriteChatCompletionSSE() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "chat.completion.chunk") {
		t.Fatalf("expected chat chunk in body, got %q", body)
	}
}

func TestOpenAIAdapterEncodesManagedTools(t *testing.T) {
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
		Tools: []canonical.Tool{
			{
				Name: "lookup_weather",
				Schema: map[string]any{
					"type": "object",
				},
			},
		},
		Stream: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	body := capture.Body()
	if !strings.Contains(body, `"tools":[`) {
		t.Fatalf("captured body missing tools array: %s", body)
	}
	if !strings.Contains(body, `"name":"lookup_weather"`) {
		t.Fatalf("captured body missing tool name: %s", body)
	}
}

func TestOpenAIAdapterEncodesNamedToolChoice(t *testing.T) {
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

	body := capture.Body()
	if !strings.Contains(body, `"tool_choice":`) {
		t.Fatalf("captured body missing tool_choice: %s", body)
	}
	if !strings.Contains(body, `"name":"lookup_weather"`) {
		t.Fatalf("captured body missing selected tool name: %s", body)
	}
}

func TestOpenAIAdapterSendsBearerAuthFromProviderAPIKeyEnv(t *testing.T) {
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
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := capture.Header("Authorization"); got != "Bearer openai-test-key" {
		t.Fatalf("authorization = %q, want %q", got, "Bearer openai-test-key")
	}
}

func TestOpenAIAdapterFailsFastWithoutAPIKeyEnv(t *testing.T) {
	server, capture := newOpenAICaptureStubServer(t, "chat_stream")
	adapter := openai.NewAdapter(server.Client())

	_, err := adapter.Execute(context.Background(), config.ProviderConfig{
		Provider: "openai",
		BaseURL:  server.URL,
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "openai/gpt-4.1",
		Stream:       true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := capture.Hits(); got != 0 {
		t.Fatalf("upstream hits = %d, want 0", got)
	}
}
