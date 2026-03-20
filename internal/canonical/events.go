package canonical

type EventType string

const (
	EventMessageStart  EventType = "message_start"
	EventContentStart  EventType = "content_block_start"
	EventContentDelta  EventType = "content_delta"
	EventToolCallStart EventType = "tool_call_start"
	EventToolCallDelta EventType = "tool_call_delta"
	EventContentStop   EventType = "content_block_stop"
	EventMessageStop   EventType = "message_stop"
	EventUsage         EventType = "usage"
	EventError         EventType = "error"
)

type Event struct {
	Type EventType
	Data map[string]any
}
