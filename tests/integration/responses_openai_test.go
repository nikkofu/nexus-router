package integration

import (
	"strings"
	"testing"

	"github.com/nikkofu/nexus-router/internal/canonical"
	openaiapi "github.com/nikkofu/nexus-router/internal/httpapi/openai"
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
