package anthropic

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/nikkofu/nexus-router/internal/canonical"
)

func DecodeStream(kind canonical.EndpointKind, r io.Reader) ([]canonical.Event, error) {
	scanner := bufio.NewScanner(r)
	events := make([]canonical.Event, 0, 8)
	currentEvent := ""
	currentToolName := ""
	currentToolID := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "event: "):
			currentEvent = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			payload := strings.TrimPrefix(line, "data: ")
			switch currentEvent {
			case "content_block_start":
				var start struct {
					ContentBlock struct {
						Type string `json:"type"`
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"content_block"`
				}
				if err := json.Unmarshal([]byte(payload), &start); err != nil {
					return nil, err
				}
				if start.ContentBlock.Type == "tool_use" {
					currentToolName = start.ContentBlock.Name
					currentToolID = start.ContentBlock.ID
					events = append(events, canonical.Event{
						Type: canonical.EventToolCallStart,
						Data: map[string]any{
							"id":   currentToolID,
							"name": currentToolName,
						},
					})
				}
			case "content_block_delta":
				var delta struct {
					Delta struct {
						Text        string `json:"text"`
						PartialJSON string `json:"partial_json"`
					} `json:"delta"`
				}
				if err := json.Unmarshal([]byte(payload), &delta); err != nil {
					return nil, err
				}

				if delta.Delta.PartialJSON != "" {
					events = append(events, canonical.Event{
						Type: canonical.EventToolCallDelta,
						Data: map[string]any{
							"id":        currentToolID,
							"name":      currentToolName,
							"arguments": delta.Delta.PartialJSON,
						},
					})
					continue
				}

				data := map[string]any{
					"object": "chat.completion.chunk",
					"text":   delta.Delta.Text,
				}
				if kind == canonical.EndpointKindResponses {
					data = map[string]any{
						"type":  "response.output_text.delta",
						"delta": delta.Delta.Text,
					}
				}

				events = append(events, canonical.Event{
					Type: canonical.EventContentDelta,
					Data: data,
				})
			case "message_stop":
				events = append(events, canonical.Event{Type: canonical.EventMessageStop})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return events, nil
}
