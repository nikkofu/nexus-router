package openai

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/nikkofu/nexus-router/internal/canonical"
)

func DecodeResponsesRequest(r io.Reader) (canonical.Request, error) {
	var req ResponsesRequest
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return canonical.Request{}, err
	}

	conversation := make([]canonical.Turn, 0, len(req.Input))
	for _, item := range req.Input {
		blocks := make([]canonical.ContentBlock, 0, len(item.Content))
		for _, content := range item.Content {
			switch content.Type {
			case "input_text":
				blocks = append(blocks, canonical.ContentBlock{
					Type: canonical.ContentTypeText,
					Text: content.Text,
				})
			case "input_image":
				blocks = append(blocks, canonical.ContentBlock{
					Type: canonical.ContentTypeImage,
					Image: &canonical.ImageInput{
						URL: content.ImageURL,
					},
				})
			}
		}

		conversation = append(conversation, canonical.Turn{
			Role:    normalizeRole(item.Role),
			Content: blocks,
		})
	}

	tools := make([]canonical.Tool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		if tool.Name == "" {
			continue
		}
		tools = append(tools, canonical.Tool{
			Name:   tool.Name,
			Schema: tool.Parameters,
		})
	}

	return canonical.Request{
		EndpointKind: canonical.EndpointKindResponses,
		PublicModel:  req.Model,
		Conversation: conversation,
		Generation: canonical.Generation{
			Temperature:     req.Temperature,
			TopP:            req.TopP,
			MaxOutputTokens: req.MaxOutputTokens,
		},
		Tools:            tools,
		ResponseContract: decodeResponsesResponseContract(req.Text),
		Stream:           req.Stream,
		Metadata:         req.Metadata,
	}, nil
}

func decodeResponsesResponseContract(text *ResponsesTextBlock) canonical.ResponseContract {
	if text == nil || len(text.Format) == 0 {
		return canonical.ResponseContract{}
	}

	switch fmt.Sprint(text.Format["type"]) {
	case "json_object":
		return canonical.ResponseContract{
			Kind: canonical.ResponseContractJSONObject,
		}
	case "json_schema":
		contract := canonical.ResponseContract{
			Kind: canonical.ResponseContractJSONSchema,
		}
		if schema, ok := text.Format["schema"].(map[string]any); ok {
			contract.Schema = schema
		}
		return contract
	default:
		return canonical.ResponseContract{}
	}
}
