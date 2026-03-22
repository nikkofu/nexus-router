package openai

import "github.com/nikkofu/nexus-router/internal/canonical"

func EncodeRequest(req canonical.Request) ([]byte, error) {
	switch req.EndpointKind {
	case canonical.EndpointKindResponses:
		payload := map[string]any{
			"model":  req.PublicModel,
			"stream": true,
			"input":  encodeResponsesInput(req.Conversation),
		}
		if req.Generation.Temperature != nil {
			payload["temperature"] = *req.Generation.Temperature
		}
		if req.Generation.TopP != nil {
			payload["top_p"] = *req.Generation.TopP
		}
		if req.Generation.MaxOutputTokens != nil {
			payload["max_output_tokens"] = *req.Generation.MaxOutputTokens
		}
		if len(req.Metadata) > 0 {
			payload["metadata"] = req.Metadata
		}
		return marshal(payload)
	default:
		payload := map[string]any{
			"model":    req.PublicModel,
			"stream":   true,
			"messages": encodeChatMessages(req.Conversation),
			"stream_options": map[string]any{
				"include_usage": true,
			},
		}
		if req.Generation.Temperature != nil {
			payload["temperature"] = *req.Generation.Temperature
		}
		if req.Generation.TopP != nil {
			payload["top_p"] = *req.Generation.TopP
		}
		if req.Generation.MaxOutputTokens != nil {
			payload["max_completion_tokens"] = *req.Generation.MaxOutputTokens
		}
		if len(req.Generation.Stop) > 0 {
			payload["stop"] = req.Generation.Stop
		}
		return marshal(payload)
	}
}

func encodeChatMessages(conversation []canonical.Turn) []any {
	messages := make([]any, 0, len(conversation))
	for _, turn := range conversation {
		messages = append(messages, map[string]any{
			"role":    string(turn.Role),
			"content": encodeChatContent(turn.Content),
		})
	}

	return messages
}

func encodeChatContent(blocks []canonical.ContentBlock) any {
	if len(blocks) == 1 && blocks[0].Type == canonical.ContentTypeText {
		return blocks[0].Text
	}

	content := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case canonical.ContentTypeText:
			content = append(content, map[string]any{
				"type": "text",
				"text": block.Text,
			})
		case canonical.ContentTypeImage:
			content = append(content, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": block.Image.URL,
				},
			})
		}
	}

	return content
}

func encodeResponsesInput(conversation []canonical.Turn) []any {
	input := make([]any, 0, len(conversation))
	for _, turn := range conversation {
		content := make([]map[string]any, 0, len(turn.Content))
		for _, block := range turn.Content {
			switch block.Type {
			case canonical.ContentTypeText:
				content = append(content, map[string]any{
					"type": "input_text",
					"text": block.Text,
				})
			case canonical.ContentTypeImage:
				content = append(content, map[string]any{
					"type":      "input_image",
					"image_url": block.Image.URL,
				})
			}
		}
		input = append(input, map[string]any{
			"role":    string(turn.Role),
			"content": content,
		})
	}

	return input
}
