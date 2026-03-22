package anthropic

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/usage"
)

type StreamDecodeError struct {
	Err             error
	OutputCommitted bool
}

func (e *StreamDecodeError) Error() string {
	if e.Err == nil {
		return "anthropic stream decode error"
	}

	return e.Err.Error()
}

func (e *StreamDecodeError) Unwrap() error {
	return e.Err
}

func DecodeStream(kind canonical.EndpointKind, r io.Reader) ([]canonical.Event, error) {
	scanner := bufio.NewScanner(r)
	events := make([]canonical.Event, 0, 8)
	currentEvent := ""
	currentToolName := ""
	currentToolID := ""
	currentStopReason := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "event: "):
			currentEvent = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			payload := strings.TrimPrefix(line, "data: ")
			switch currentEvent {
			case "message_start":
				var start struct {
					Message struct {
						Usage map[string]any `json:"usage"`
					} `json:"message"`
				}
				if err := json.Unmarshal([]byte(payload), &start); err != nil {
					return nil, &StreamDecodeError{Err: err, OutputCommitted: len(events) > 0}
				}
				if _, ok := usage.FromData(start.Message.Usage); ok {
					events = append(events, canonical.Event{
						Type: canonical.EventUsage,
						Data: start.Message.Usage,
					})
				}
			case "content_block_start":
				var start struct {
					ContentBlock struct {
						Type string `json:"type"`
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"content_block"`
				}
				if err := json.Unmarshal([]byte(payload), &start); err != nil {
					return nil, &StreamDecodeError{Err: err, OutputCommitted: len(events) > 0}
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
					return nil, &StreamDecodeError{Err: err, OutputCommitted: len(events) > 0}
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
			case "message_delta":
				var delta struct {
					Delta struct {
						StopReason string `json:"stop_reason"`
					} `json:"delta"`
					Usage map[string]any `json:"usage"`
				}
				if err := json.Unmarshal([]byte(payload), &delta); err != nil {
					return nil, &StreamDecodeError{Err: err, OutputCommitted: len(events) > 0}
				}
				if delta.Delta.StopReason != "" {
					currentStopReason = delta.Delta.StopReason
				}
				if _, ok := usage.FromData(delta.Usage); ok {
					events = append(events, canonical.Event{
						Type: canonical.EventUsage,
						Data: delta.Usage,
					})
				}
			case "message_stop":
				var data map[string]any
				if currentStopReason != "" {
					data = map[string]any{"stop_reason": currentStopReason}
				}
				events = append(events, canonical.Event{Type: canonical.EventMessageStop, Data: data})
				currentStopReason = ""
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, &StreamDecodeError{Err: err, OutputCommitted: len(events) > 0}
	}

	return events, nil
}
