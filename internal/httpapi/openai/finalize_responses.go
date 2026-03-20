package openai

import "github.com/nikkofu/nexus-router/internal/canonical"

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
	text, _ := aggregateFinalText(events)

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
