package openai

import (
	"encoding/json"
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

	return canonical.Request{
		EndpointKind: canonical.EndpointKindResponses,
		PublicModel:  req.Model,
		Conversation: conversation,
		Stream:       req.Stream,
	}, nil
}
