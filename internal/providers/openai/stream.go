package openai

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/nikkofu/nexus-router/internal/canonical"
)

type StreamDecodeError struct {
	Err             error
	OutputCommitted bool
}

func (e *StreamDecodeError) Error() string {
	if e.Err == nil {
		return "openai stream decode error"
	}

	return e.Err.Error()
}

func (e *StreamDecodeError) Unwrap() error {
	return e.Err
}

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
				return nil, &StreamDecodeError{Err: err, OutputCommitted: len(events) > 0}
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
			return nil, &StreamDecodeError{Err: err, OutputCommitted: len(events) > 0}
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
		return nil, &StreamDecodeError{Err: err, OutputCommitted: len(events) > 0}
	}

	return events, nil
}
