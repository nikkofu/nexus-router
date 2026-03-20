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

	tools := make([]canonical.Tool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		if tool.Type != "function" || tool.Function == nil {
			continue
		}
		tools = append(tools, canonical.Tool{
			Name:   tool.Function.Name,
			Schema: tool.Function.Parameters,
		})
	}

	stop, err := decodeStop(req.Stop)
	if err != nil {
		return canonical.Request{}, err
	}

	maxOutputTokens := req.MaxCompletionTokens
	if maxOutputTokens == nil {
		maxOutputTokens = req.MaxTokens
	}

	return canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  req.Model,
		Conversation: conversation,
		Generation: canonical.Generation{
			Temperature:     req.Temperature,
			TopP:            req.TopP,
			MaxOutputTokens: maxOutputTokens,
			Stop:            stop,
		},
		Tools:            tools,
		ResponseContract: decodeChatResponseContract(req.ResponseFormat),
		Stream:           req.Stream,
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
			imageURL := fmt.Sprint(item["image_url"])
			if imageValue, ok := item["image_url"].(map[string]any); ok {
				imageURL = fmt.Sprint(imageValue["url"])
			}
			blocks = append(blocks, canonical.ContentBlock{
				Type: canonical.ContentTypeImage,
				Image: &canonical.ImageInput{
					URL: imageURL,
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

func decodeStop(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if single == "" {
			return nil, nil
		}
		return []string{single}, nil
	}

	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, err
	}

	return many, nil
}

func decodeChatResponseContract(format *ChatResponseFormat) canonical.ResponseContract {
	if format == nil {
		return canonical.ResponseContract{}
	}

	switch format.Type {
	case "json_object":
		return canonical.ResponseContract{
			Kind: canonical.ResponseContractJSONObject,
		}
	case "json_schema":
		contract := canonical.ResponseContract{
			Kind: canonical.ResponseContractJSONSchema,
		}
		if format.JSONSchema != nil {
			contract.Schema = format.JSONSchema.Schema
		}
		return contract
	default:
		return canonical.ResponseContract{}
	}
}
