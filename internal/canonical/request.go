package canonical

type Request struct {
	EndpointKind     EndpointKind
	PublicModel      string
	Conversation     []Turn
	Generation       Generation
	Tools            []Tool
	ToolChoice       ToolChoice
	ResponseContract ResponseContract
	Stream           bool
	Metadata         map[string]string
}

type EndpointKind string

const (
	EndpointKindChatCompletions EndpointKind = "chat_completions"
	EndpointKindResponses       EndpointKind = "responses"
)

type Generation struct {
	Temperature     *float64
	TopP            *float64
	MaxOutputTokens *int
	Stop            []string
}

type Tool struct {
	Name   string
	Schema map[string]any
}

type ToolChoice struct {
	Name string
}

type ResponseContract struct {
	Kind   ResponseContractKind
	Schema map[string]any
}

type ResponseContractKind string

const (
	ResponseContractNone       ResponseContractKind = ""
	ResponseContractJSONObject ResponseContractKind = "json_object"
	ResponseContractJSONSchema ResponseContractKind = "json_schema"
)
