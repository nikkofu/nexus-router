package openai

import (
	"strings"

	"github.com/nikkofu/nexus-router/internal/canonical"
)

type FinalResponse struct {
	ID     string                `json:"id"`
	Object string                `json:"object"`
	Model  string                `json:"model"`
	Status string                `json:"status"`
	Output []FinalResponseOutput `json:"output"`
}

type FinalResponseOutput struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func FinalizeResponse(events []canonical.Event, model string) FinalResponse {
	text := aggregateResponseText(events)

	return FinalResponse{
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
