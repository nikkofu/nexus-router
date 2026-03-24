package openai

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/nikkofu/nexus-router/internal/canonical"
)

func DecodeResponsesRequest(r io.Reader) (canonical.Request, error) {
	var req struct {
		Model           string              `json:"model"`
		Input           []responsesInput    `json:"input"`
		Stream          bool                `json:"stream"`
		Text            *ResponsesTextBlock `json:"text,omitempty"`
		Tools           []ResponsesTool     `json:"tools,omitempty"`
		Temperature     *float64            `json:"temperature,omitempty"`
		TopP            *float64            `json:"top_p,omitempty"`
		MaxOutputTokens *int                `json:"max_output_tokens,omitempty"`
		Metadata        map[string]string   `json:"metadata,omitempty"`
	}
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return canonical.Request{}, invalidRequestError("malformed request body: %v", err)
	}

	conversation := make([]canonical.Turn, 0, len(req.Input))
	for _, item := range req.Input {
		role := normalizeRole(item.Role)
		blocks := make([]canonical.ContentBlock, 0, len(item.Content))
		for _, content := range item.Content {
			switch content.Type {
			case "input_text":
				blocks = append(blocks, canonical.ContentBlock{
					Type: canonical.ContentTypeText,
					Text: content.Text,
				})
			case "input_image":
				if err := validatePublicImageRole(item.Role, role); err != nil {
					return canonical.Request{}, err
				}

				imageURL, err := normalizeResponsesImageURL(content.Raw)
				if err != nil {
					return canonical.Request{}, err
				}
				if err := validatePublicImageURL(imageURL); err != nil {
					return canonical.Request{}, err
				}

				blocks = append(blocks, canonical.ContentBlock{
					Type: canonical.ContentTypeImage,
					Image: &canonical.ImageInput{
						URL: imageURL,
					},
				})
			default:
				return canonical.Request{}, invalidRequestError("input content item type %q is not supported", content.Type)
			}
		}

		conversation = append(conversation, canonical.Turn{
			Role:    role,
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

type responsesInput struct {
	Role    string                  `json:"role"`
	Content []responsesContentInput `json:"content"`
}

type responsesContentInput struct {
	Type string         `json:"type"`
	Text string         `json:"text,omitempty"`
	Raw  map[string]any `json:"-"`
}

func (r *responsesContentInput) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.Raw = raw
	r.Type, _ = raw["type"].(string)
	r.Text, _ = raw["text"].(string)

	return nil
}

func normalizeResponsesImageURL(item map[string]any) (string, error) {
	if _, hasFileID := item["file_id"]; hasFileID {
		return "", unsupportedCapabilityError("input_image file_id form is not supported")
	}
	if err := validateAllowedObjectKeys(item, "type", "image_url"); err != nil {
		return "", err
	}

	imageValue, ok := item["image_url"]
	if !ok {
		return "", invalidRequestError("input_image.image_url is required")
	}

	switch value := imageValue.(type) {
	case string:
		return value, nil
	case map[string]any:
		if _, hasFileID := value["file_id"]; hasFileID {
			return "", unsupportedCapabilityError("input_image file_id form is not supported")
		}
		return "", invalidRequestError("input_image.image_url must be a string")
	default:
		return "", invalidRequestError("input_image.image_url must be a string")
	}
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
