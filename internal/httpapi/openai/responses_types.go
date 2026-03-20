package openai

type ResponsesRequest struct {
	Model           string              `json:"model"`
	Input           []ResponsesInput    `json:"input"`
	Stream          bool                `json:"stream"`
	Text            *ResponsesTextBlock `json:"text,omitempty"`
	Tools           []ResponsesTool     `json:"tools,omitempty"`
	Temperature     *float64            `json:"temperature,omitempty"`
	TopP            *float64            `json:"top_p,omitempty"`
	MaxOutputTokens *int                `json:"max_output_tokens,omitempty"`
	Metadata        map[string]string   `json:"metadata,omitempty"`
}

type ResponsesInput struct {
	Role    string                 `json:"role"`
	Content []ResponsesContentItem `json:"content"`
}

type ResponsesContentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type ResponsesTextBlock struct {
	Format map[string]any `json:"format,omitempty"`
}

type ResponsesTool struct {
	Type       string         `json:"type"`
	Name       string         `json:"name,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
}
