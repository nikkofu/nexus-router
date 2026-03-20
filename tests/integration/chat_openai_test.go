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
