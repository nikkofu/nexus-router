package integration

import (
	"context"
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
