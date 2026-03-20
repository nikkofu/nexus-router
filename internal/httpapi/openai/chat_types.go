package openai

import "encoding/json"

type ChatCompletionRequest struct {
	Model               string              `json:"model"`
	Messages            []ChatMessage       `json:"messages"`
	Stream              bool                `json:"stream"`
	Temperature         *float64            `json:"temperature,omitempty"`
	TopP                *float64            `json:"top_p,omitempty"`
	MaxTokens           *int                `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int                `json:"max_completion_tokens,omitempty"`
	Stop                json.RawMessage     `json:"stop,omitempty"`
	Tools               []ChatTool          `json:"tools,omitempty"`
	ToolChoice          json.RawMessage     `json:"tool_choice,omitempty"`
	ResponseFormat      *ChatResponseFormat `json:"response_format,omitempty"`
}

type ChatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type ChatTool struct {
	Type     string            `json:"type"`
	Function *ChatToolFunction `json:"function,omitempty"`
}

type ChatToolFunction struct {
	Name       string         `json:"name"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type ChatResponseFormat struct {
	Type       string                `json:"type"`
	JSONSchema *ChatJSONSchemaFormat `json:"json_schema,omitempty"`
}

type ChatJSONSchemaFormat struct {
	Name   string         `json:"name,omitempty"`
	Schema map[string]any `json:"schema,omitempty"`
}
