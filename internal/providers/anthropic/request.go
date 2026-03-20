package anthropic

import (
	"encoding/json"

	"github.com/nikkofu/nexus-router/internal/canonical"
)

func EncodeRequest(req canonical.Request) ([]byte, error) {
	system := ""
	messages := make([]map[string]any, 0, len(req.Conversation))
	tools := make([]map[string]any, 0, len(req.Tools))

	for _, turn := range req.Conversation {
		if turn.Role == canonical.RoleSystem {
			for _, block := range turn.Content {
				if block.Type == canonical.ContentTypeText {
					system = block.Text
					break
				}
			}
			continue
		}

		content := make([]map[string]any, 0, len(turn.Content))
		for _, block := range turn.Content {
			switch block.Type {
			case canonical.ContentTypeText:
				content = append(content, map[string]any{
					"type": "text",
					"text": block.Text,
				})
			case canonical.ContentTypeImage:
				content = append(content, map[string]any{
					"type": "image",
					"source": map[string]any{
						"type":       "url",
						"url":        block.Image.URL,
						"media_type": block.Image.MIMEType,
					},
				})
			}
		}

		messages = append(messages, map[string]any{
			"role":    string(turn.Role),
			"content": content,
		})
	}

	for _, tool := range req.Tools {
		tools = append(tools, map[string]any{
			"name":         tool.Name,
			"input_schema": tool.Schema,
		})
	}

	payload := map[string]any{
		"model":    req.PublicModel,
		"system":   system,
		"stream":   req.Stream,
		"messages": messages,
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}

	return json.Marshal(payload)
}
