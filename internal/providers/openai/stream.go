package openai

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

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			events = append(events, canonical.Event{Type: canonical.EventMessageStop})
			continue
		}

		if kind == canonical.EndpointKindResponses {
			var event map[string]any
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				return nil, err
			}
			events = append(events, canonical.Event{
				Type: canonical.EventContentDelta,
				Data: event,
			})
			continue
		}

		var chunk struct {
			Object  string `json:"object"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return nil, err
		}

		text := ""
		if len(chunk.Choices) > 0 {
			text = chunk.Choices[0].Delta.Content
		}

		events = append(events, canonical.Event{
			Type: canonical.EventContentDelta,
			Data: map[string]any{
				"object": chunk.Object,
				"text":   text,
			},
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return events, nil
}
