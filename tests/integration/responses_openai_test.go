package integration

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/config"
	openaiapi "github.com/nikkofu/nexus-router/internal/httpapi/openai"
	"github.com/nikkofu/nexus-router/internal/providers/openai"
	"github.com/nikkofu/nexus-router/internal/streaming"
)

func TestResponsesHandlerNormalizesInputIntoCanonicalRequest(t *testing.T) {
	reqBody := `{
		"model": "openai/gpt-4.1",
		"stream": true,
		"input": [
			{
				"role": "user",
				"content": [
					{"type": "input_text", "text": "hello from responses"}
				]
			}
		]
	}`

	got, err := openaiapi.DecodeResponsesRequest(strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("DecodeResponsesRequest() error = %v", err)
	}

	if got.PublicModel != "openai/gpt-4.1" {
		t.Fatalf("PublicModel = %q, want %q", got.PublicModel, "openai/gpt-4.1")
	}
	if got.EndpointKind != canonical.EndpointKindResponses {
		t.Fatalf("EndpointKind = %q, want %q", got.EndpointKind, canonical.EndpointKindResponses)
	}
	if len(got.Conversation) != 1 {
		t.Fatalf("conversation len = %d, want 1", len(got.Conversation))
	}
	if got.Conversation[0].Content[0].Text != "hello from responses" {
		t.Fatalf("content = %q, want %q", got.Conversation[0].Content[0].Text, "hello from responses")
	}
}

func TestResponsesStreamingRelaysOpenAIEvents(t *testing.T) {
	server := newOpenAIStubServer(t, "responses_stream")
	adapter := openai.NewAdapter(server.Client())

	result, err := adapter.Execute(context.Background(), config.ProviderConfig{
		BaseURL: server.URL,
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindResponses,
		PublicModel:  "openai/gpt-4.1",
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	rec := httptest.NewRecorder()
	if err := streaming.WriteResponsesSSE(rec, result.Events); err != nil {
		t.Fatalf("WriteResponsesSSE() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "response.output_text.delta") {
		t.Fatalf("expected responses stream event in body, got %q", body)
	}
}

func TestOpenAIAdapterClassifiesRetryable429(t *testing.T) {
	server := newOpenAIStubServer(t, "rate_limit")
	adapter := openai.NewAdapter(server.Client())

	_, err := adapter.Execute(context.Background(), config.ProviderConfig{
		BaseURL: server.URL,
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "openai/gpt-4.1",
		Stream:       true,
	})
	if err == nil {
		t.Fatal("expected error")
	}

	classified, ok := err.(*openai.ClassifiedError)
	if !ok {
		t.Fatalf("error type = %T, want *openai.ClassifiedError", err)
	}
	if !classified.Retryable {
		t.Fatal("Retryable = false, want true")
	}
	if classified.StatusCode != 429 {
		t.Fatalf("StatusCode = %d, want 429", classified.StatusCode)
	}
}
