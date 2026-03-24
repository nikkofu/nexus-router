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
		return canonical.Request{}, invalidRequestError("malformed request body: %v", err)
	}

	conversation := make([]canonical.Turn, 0, len(req.Messages))
	for _, msg := range req.Messages {
		role := normalizeRole(msg.Role)
		blocks, err := normalizeChatContent(msg.Content, msg.Role, role)
		if err != nil {
			return canonical.Request{}, err
		}

		conversation = append(conversation, canonical.Turn{
			Role:    role,
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

	toolChoice, err := decodeChatToolChoice(req.ToolChoice)
	if err != nil {
		return canonical.Request{}, err
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
		ToolChoice:       toolChoice,
		ResponseContract: decodeChatResponseContract(req.ResponseFormat),
		Stream:           req.Stream,
	}, nil
}

func normalizeChatContent(raw json.RawMessage, rawRole string, role canonical.Role) ([]canonical.ContentBlock, error) {
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
		return nil, invalidRequestError("messages.content must be a string or array of content objects")
	}

	blocks := make([]canonical.ContentBlock, 0, len(items))
	for _, item := range items {
		itemType, _ := item["type"].(string)
		switch itemType {
		case "text":
			if isImageLikeContentItem(item) {
				return nil, invalidRequestError("messages.content text items must not include image fields")
			}
			blocks = append(blocks, canonical.ContentBlock{
				Type: canonical.ContentTypeText,
				Text: fmt.Sprint(item["text"]),
			})
		case "image_url":
			if err := validatePublicImageRole(rawRole, role); err != nil {
				return nil, err
			}

			imageURL, err := normalizeChatImageURL(item)
			if err != nil {
				return nil, err
			}
			if err := validatePublicImageURL(imageURL); err != nil {
				return nil, err
			}

			blocks = append(blocks, canonical.ContentBlock{
				Type: canonical.ContentTypeImage,
				Image: &canonical.ImageInput{
					URL: imageURL,
				},
			})
		default:
			return nil, invalidRequestError("messages.content item type %q is not supported", itemType)
		}
	}

	return blocks, nil
}

func normalizeChatImageURL(item map[string]any) (string, error) {
	if _, hasFileID := item["file_id"]; hasFileID {
		return "", unsupportedCapabilityError("image file_id form is not supported")
	}
	if err := validateAllowedObjectKeys(item, "type", "image_url"); err != nil {
		return "", err
	}

	imageValue, ok := item["image_url"]
	if !ok {
		return "", invalidRequestError("image_url.url is required")
	}

	switch value := imageValue.(type) {
	case map[string]any:
		if _, hasFileID := value["file_id"]; hasFileID {
			return "", unsupportedCapabilityError("image file_id form is not supported")
		}
		if err := validateAllowedObjectKeys(value, "url"); err != nil {
			return "", err
		}

		rawURL, ok := value["url"]
		if !ok {
			return "", invalidRequestError("image_url.url is required")
		}

		imageURL, ok := rawURL.(string)
		if !ok {
			return "", invalidRequestError("image_url.url must be a string")
		}

		return imageURL, nil
	default:
		return "", invalidRequestError("image_url must be an object with a url field")
	}
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

func decodeChatToolChoice(raw json.RawMessage) (canonical.ToolChoice, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return canonical.ToolChoice{}, nil
	}

	var mode string
	if err := json.Unmarshal(raw, &mode); err == nil {
		if mode == "" || mode == "auto" {
			return canonical.ToolChoice{}, nil
		}
		return canonical.ToolChoice{}, fmt.Errorf("invalid tool_choice: unsupported value %q", mode)
	}

	var choice struct {
		Type     string `json:"type"`
		Function *struct {
			Name string `json:"name"`
		} `json:"function,omitempty"`
	}
	if err := json.Unmarshal(raw, &choice); err != nil {
		return canonical.ToolChoice{}, fmt.Errorf("invalid tool_choice: %w", err)
	}
	if choice.Type != "function" || choice.Function == nil || choice.Function.Name == "" {
		return canonical.ToolChoice{}, fmt.Errorf("invalid tool_choice: expected named function tool choice")
	}

	return canonical.ToolChoice{Name: choice.Function.Name}, nil
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
