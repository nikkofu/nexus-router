package e2e

import (
	"strings"
	"testing"

	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/httpapi/openai"
)

func TestFinalizeChatCompletionAggregatesTextAndNormalizesFinishReason(t *testing.T) {
	events := []canonical.Event{
		{Type: canonical.EventContentDelta, Data: map[string]any{"text": "hel"}},
		{Type: canonical.EventContentDelta, Data: map[string]any{"text": "lo"}},
		{Type: canonical.EventMessageStop, Data: map[string]any{"stop_reason": "end_turn"}},
	}

	got := openai.FinalizeChatCompletion(events, "openai/gpt-4.1")

	if !strings.HasPrefix(got.ID, "chatcmpl-") {
		t.Fatalf("id = %q, want prefix %q", got.ID, "chatcmpl-")
	}
	if got.Object != "chat.completion" {
		t.Fatalf("object = %q, want %q", got.Object, "chat.completion")
	}
	if got.Model != "openai/gpt-4.1" {
		t.Fatalf("model = %q, want %q", got.Model, "openai/gpt-4.1")
	}
	if len(got.Choices) != 1 {
		t.Fatalf("choices len = %d, want 1", len(got.Choices))
	}

	choice := got.Choices[0]
	if choice.Message.Role != "assistant" {
		t.Fatalf("choice.message.role = %q, want %q", choice.Message.Role, "assistant")
	}
	if choice.Message.Content != "hello" {
		t.Fatalf("choice.message.content = %q, want %q", choice.Message.Content, "hello")
	}
	if choice.FinishReason != "stop" {
		t.Fatalf("choice.finish_reason = %q, want %q", choice.FinishReason, "stop")
	}
}

func TestFinalizeResponseAggregatesTextAndMarksCompleted(t *testing.T) {
	events := []canonical.Event{
		{Type: canonical.EventContentDelta, Data: map[string]any{"text": "foo"}},
		{Type: canonical.EventContentDelta, Data: map[string]any{"text": "bar"}},
		{Type: canonical.EventMessageStop, Data: map[string]any{"finish_reason": "max_tokens"}},
	}

	got := openai.FinalizeResponse(events, "openai/gpt-4.1")

	if !strings.HasPrefix(got.ID, "resp_") {
		t.Fatalf("id = %q, want prefix %q", got.ID, "resp_")
	}
	if got.Object != "response" {
		t.Fatalf("object = %q, want %q", got.Object, "response")
	}
	if got.Model != "openai/gpt-4.1" {
		t.Fatalf("model = %q, want %q", got.Model, "openai/gpt-4.1")
	}
	if got.Status != "completed" {
		t.Fatalf("status = %q, want %q", got.Status, "completed")
	}
	if len(got.Output) != 1 {
		t.Fatalf("output len = %d, want 1", len(got.Output))
	}
	if got.Output[0].Type != "output_text" {
		t.Fatalf("output[0].type = %q, want %q", got.Output[0].Type, "output_text")
	}
	if got.Output[0].Text != "foobar" {
		t.Fatalf("output[0].text = %q, want %q", got.Output[0].Text, "foobar")
	}
}
