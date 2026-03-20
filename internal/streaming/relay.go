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
		case canonical.EventMessageStop:
			if _, err := io.WriteString(w, "data: [DONE]\n\n"); err != nil {
				return err
			}
		}
	}

	return nil
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
