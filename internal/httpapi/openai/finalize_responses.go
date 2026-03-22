package openai

import (
	"strings"

	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/usage"
)

type FinalResponse struct {
	ID     string                `json:"id"`
	Object string                `json:"object"`
	Model  string                `json:"model"`
	Status string                `json:"status"`
	Output []FinalResponseOutput `json:"output"`
	Usage  *FinalResponseUsage   `json:"usage,omitempty"`
}

type FinalResponseOutput struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type FinalResponseUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func FinalizeResponse(events []canonical.Event, model string) FinalResponse {
	text := aggregateResponseText(events)

	resp := FinalResponse{
		ID:     newGeneratedID("resp_"),
		Object: "response",
		Model:  model,
		Status: "completed",
		Output: []FinalResponseOutput{
			{
				Type: "output_text",
				Text: text,
			},
		},
	}

	if summary, ok := usage.FromEvents(events); ok {
		resp.Usage = &FinalResponseUsage{
			InputTokens:  summary.InputTokens,
			OutputTokens: summary.OutputTokens,
			TotalTokens:  summary.TotalTokens,
		}
	}

	return resp
}

func aggregateResponseText(events []canonical.Event) string {
	var builder strings.Builder

	for _, event := range events {
		if event.Type != canonical.EventContentDelta {
			continue
		}
		if text, ok := event.Data["text"].(string); ok {
			builder.WriteString(text)
			continue
		}
		if delta, ok := event.Data["delta"].(string); ok {
			builder.WriteString(delta)
		}
	}

	return builder.String()
}
