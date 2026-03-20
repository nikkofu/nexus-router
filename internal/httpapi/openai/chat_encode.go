package openai

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/nikkofu/nexus-router/internal/canonical"
)

func DecodeChatCompletionRequest(r io.Reader) (canonical.Request, error) {
	var req ChatCompletionRequest
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return canonical.Request{}, err
	}

	conversation := make([]canonical.Turn, 0, len(req.Messages))
	for _, msg := range req.Messages {
		blocks, err := normalizeChatContent(msg.Content)
		if err != nil {
			return canonical.Request{}, err
		}

		conversation = append(conversation, canonical.Turn{
			Role:    normalizeRole(msg.Role),
			Content: blocks,
		})
	}

	return canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  req.Model,
		Conversation: conversation,
		Stream:       req.Stream,
	}, nil
}

func normalizeChatContent(raw json.RawMessage) ([]canonical.ContentBlock, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return []canonical.ContentBlock{{
			Type: canonical.ContentTypeText,
			Text: text,
		}}, nil
	}

	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}

	blocks := make([]canonical.ContentBlock, 0, len(items))
	for _, item := range items {
		switch item["type"] {
		case "text":
			blocks = append(blocks, canonical.ContentBlock{
				Type: canonical.ContentTypeText,
				Text: fmt.Sprint(item["text"]),
			})
		case "image_url":
			blocks = append(blocks, canonical.ContentBlock{
				Type: canonical.ContentTypeImage,
				Image: &canonical.ImageInput{
					URL: fmt.Sprint(item["image_url"]),
				},
			})
		}
	}

	return blocks, nil
}

func normalizeRole(role string) canonical.Role {
	switch role {
	case "system":
		return canonical.RoleSystem
	case "assistant":
		return canonical.RoleAssistant
	case "tool":
		return canonical.RoleTool
	default:
		return canonical.RoleUser
	}
}
