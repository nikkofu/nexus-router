package streaming

import (
	"encoding/json"
	"io"

	"github.com/nikkofu/nexus-router/internal/canonical"
)

func WriteChatCompletionSSE(w io.Writer, events []canonical.Event) error {
	for _, event := range events {
		switch event.Type {
		case canonical.EventContentDelta:
			payload := map[string]any{
				"object": event.Data["object"],
				"choices": []map[string]any{
					{
						"delta": map[string]any{
							"content": event.Data["text"],
						},
					},
				},
			}
			if err := writeDataEvent(w, payload); err != nil {
				return err
			}
		case canonical.EventToolCallStart, canonical.EventToolCallDelta:
			payload := map[string]any{
				"object": "chat.completion.chunk",
				"choices": []map[string]any{
					{
						"delta": map[string]any{
							"tool_calls": []map[string]any{
								{
									"id":   event.Data["id"],
									"type": "function",
									"function": map[string]any{
										"name":      event.Data["name"],
										"arguments": event.Data["arguments"],
									},
								},
							},
						},
					},
				},
			}
			if err := writeDataEvent(w, payload); err != nil {
				return err
			}
		case canonical.EventMessageStop:
			if finishReason, ok := normalizedChatFinishReason(event.Data); ok {
				payload := map[string]any{
					"object": "chat.completion.chunk",
					"choices": []map[string]any{
						{
							"delta":         map[string]any{},
							"finish_reason": finishReason,
						},
					},
				}
				if err := writeDataEvent(w, payload); err != nil {
					return err
				}
				continue
			}
			if _, err := io.WriteString(w, "data: [DONE]\n\n"); err != nil {
				return err
			}
		}
	}

	return nil
}

func normalizedChatFinishReason(data map[string]any) (string, bool) {
	if len(data) == 0 {
		return "", false
	}

	if raw, ok := data["finish_reason"]; ok {
		return normalizeChatFinishReasonValue(raw), true
	}
	if raw, ok := data["stop_reason"]; ok {
		return normalizeChatFinishReasonValue(raw), true
	}

	return "", false
}

func normalizeChatFinishReasonValue(raw any) string {
	value, ok := raw.(string)
	if !ok || value == "" {
		return "stop"
	}

	switch value {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return value
	}
}

func WriteResponsesSSE(w io.Writer, events []canonical.Event) error {
	for _, event := range events {
		switch event.Type {
		case canonical.EventContentDelta:
			if err := writeDataEvent(w, event.Data); err != nil {
				return err
			}
		case canonical.EventMessageStop:
			if err := writeDataEvent(w, map[string]any{"type": "response.completed"}); err != nil {
				return err
			}
		}
	}

	return nil
}

func writeDataEvent(w io.Writer, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, "data: "); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "\n\n"); err != nil {
		return err
	}

	return nil
}
