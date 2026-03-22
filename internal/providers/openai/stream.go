package openai

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
			responseEvents, err := decodeResponsesEvent(payload)
			if err != nil {
				return nil, &StreamDecodeError{Err: err, OutputCommitted: len(events) > 0}
			}
			events = append(events, responseEvents...)
			continue
		}

		chatEvents, err := decodeChatEvent(payload)
		if err != nil {
			return nil, &StreamDecodeError{Err: err, OutputCommitted: len(events) > 0}
		}
		events = append(events, chatEvents...)
	}

	if err := scanner.Err(); err != nil {
		return nil, &StreamDecodeError{Err: err, OutputCommitted: len(events) > 0}
	}

	return events, nil
}

func decodeResponsesEvent(payload string) ([]canonical.Event, error) {
	var event map[string]any
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return nil, err
	}

	events := make([]canonical.Event, 0, 2)
	switch eventType, _ := event["type"].(string); eventType {
	case "response.output_text.delta":
		delta, _ := event["delta"].(string)
		events = append(events, canonical.Event{
			Type: canonical.EventContentDelta,
			Data: map[string]any{
				"type":  "response.output_text.delta",
				"delta": delta,
			},
		})
	case "response.completed":
		appendUsageEvent(&events, nestedMap(event, "response", "usage"))
		appendUsageEvent(&events, mapValue(event["usage"]))
		events = append(events, canonical.Event{Type: canonical.EventMessageStop})
	default:
		appendUsageEvent(&events, mapValue(event["usage"]))
	}

	return events, nil
}

func decodeChatEvent(payload string) ([]canonical.Event, error) {
	var chunk map[string]any
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return nil, err
	}

	events := make([]canonical.Event, 0, 2)
	appendUsageEvent(&events, mapValue(chunk["usage"]))

	object, _ := chunk["object"].(string)
	text := ""
	finishReason := ""
	if choices, ok := chunk["choices"].([]any); ok && len(choices) > 0 {
		if choice := mapValue(choices[0]); len(choice) > 0 {
			if delta := mapValue(choice["delta"]); len(delta) > 0 {
				text, _ = delta["content"].(string)
			}
			finishReason, _ = choice["finish_reason"].(string)
		}
	}

	if text != "" {
		if object == "" {
			object = "chat.completion.chunk"
		}
		events = append(events, canonical.Event{
			Type: canonical.EventContentDelta,
			Data: map[string]any{
				"object": object,
				"text":   text,
			},
		})
	}
	if finishReason != "" {
		events = append(events, canonical.Event{
			Type: canonical.EventMessageStop,
			Data: map[string]any{"finish_reason": finishReason},
		})
	}

	return events, nil
}

func appendUsageEvent(events *[]canonical.Event, data map[string]any) {
	if _, ok := usage.FromData(data); ok {
		*events = append(*events, canonical.Event{
			Type: canonical.EventUsage,
			Data: data,
		})
	}
}

func nestedMap(data map[string]any, keys ...string) map[string]any {
	current := data
	for i, key := range keys {
		value, ok := current[key]
		if !ok {
			return nil
		}
		next := mapValue(value)
		if len(next) == 0 && i != len(keys)-1 {
			return nil
		}
		current = next
	}

	return current
}

func mapValue(value any) map[string]any {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil
	}

	return typed
}
