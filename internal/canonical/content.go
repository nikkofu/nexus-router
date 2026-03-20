package canonical

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Turn struct {
	Role    Role
	Content []ContentBlock
}

type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeImage      ContentType = "image"
	ContentTypeToolCall   ContentType = "tool_call"
	ContentTypeToolResult ContentType = "tool_result"
)

type ContentBlock struct {
	Type  ContentType
	Text  string
	Image *ImageInput
}

type ImageInput struct {
	URL      string
	MIMEType string
}
