package openai

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/nikkofu/nexus-router/internal/canonical"
)

type FinalChatCompletion struct {
	ID      string                      `json:"id"`
	Object  string                      `json:"object"`
	Model   string                      `json:"model"`
	Choices []FinalChatCompletionChoice `json:"choices"`
}

type FinalChatCompletionChoice struct {
	Index        int                        `json:"index"`
	Message      FinalChatCompletionMessage `json:"message"`
	FinishReason string                     `json:"finish_reason"`
}

type FinalChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func FinalizeChatCompletion(events []canonical.Event, model string) FinalChatCompletion {
	text, finishReason := aggregateFinalText(events)

	return FinalChatCompletion{
		ID:     newGeneratedID("chatcmpl-"),
		Object: "chat.completion",
		Model:  model,
		Choices: []FinalChatCompletionChoice{
			{
				Index: 0,
				Message: FinalChatCompletionMessage{
					Role:    "assistant",
					Content: text,
				},
				FinishReason: finishReason,
			},
		},
	}
}

func aggregateFinalText(events []canonical.Event) (string, string) {
	var builder strings.Builder
	finishReason := "stop"

	for _, event := range events {
		switch event.Type {
		case canonical.EventContentDelta:
			text, ok := event.Data["text"].(string)
			if ok {
				builder.WriteString(text)
			}
		case canonical.EventMessageStop:
			if event.Data == nil {
				continue
			}
			if raw, ok := event.Data["finish_reason"]; ok {
				finishReason = normalizeFinishReason(raw)
				continue
			}
			if raw, ok := event.Data["stop_reason"]; ok {
				finishReason = normalizeFinishReason(raw)
			}
		}
	}

	return builder.String(), finishReason
}

func normalizeFinishReason(raw any) string {
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

func newGeneratedID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return prefix + hex.EncodeToString(b[:])
	}

	return fmt.Sprintf("%s%x", prefix, time.Now().UnixNano())
}
