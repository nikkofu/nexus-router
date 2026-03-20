package openai

type ResponsesRequest struct {
	Model  string              `json:"model"`
	Input  []ResponsesInput    `json:"input"`
	Stream bool                `json:"stream"`
	Text   *ResponsesTextBlock `json:"text,omitempty"`
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
