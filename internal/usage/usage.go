package usage

import "github.com/nikkofu/nexus-router/internal/canonical"

type Summary struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

type partialSummary struct {
	inputTokens  int
	hasInput     bool
	outputTokens int
	hasOutput    bool
	totalTokens  int
	hasTotal     bool
}

func FromEvents(events []canonical.Event) (Summary, bool) {
	var (
		summary Summary
		found   bool
	)

	for _, event := range events {
		if event.Type != canonical.EventUsage {
			continue
		}

		partial, ok := partialFromData(event.Data)
		if !ok {
			continue
		}

		if partial.hasInput {
			summary.InputTokens = partial.inputTokens
		}
		if partial.hasOutput {
			summary.OutputTokens = partial.outputTokens
		}
		if partial.hasTotal {
			summary.TotalTokens = partial.totalTokens
		}
		found = true
	}

	if !found {
		return Summary{}, false
	}
	if summary.TotalTokens == 0 {
		summary.TotalTokens = summary.InputTokens + summary.OutputTokens
	}

	return summary, true
}

func FromData(data map[string]any) (Summary, bool) {
	partial, ok := partialFromData(data)
	if !ok {
		return Summary{}, false
	}

	summary := Summary{}
	if partial.hasInput {
		summary.InputTokens = partial.inputTokens
	}
	if partial.hasOutput {
		summary.OutputTokens = partial.outputTokens
	}
	if partial.hasTotal {
		summary.TotalTokens = partial.totalTokens
	}
	if summary.TotalTokens == 0 {
		summary.TotalTokens = summary.InputTokens + summary.OutputTokens
	}

	return summary, true
}

func partialFromData(data map[string]any) (partialSummary, bool) {
	if len(data) == 0 {
		return partialSummary{}, false
	}

	summary := partialSummary{}
	summary.inputTokens, summary.hasInput = intFromMap(data, "input_tokens")
	if !summary.hasInput {
		summary.inputTokens, summary.hasInput = intFromMap(data, "prompt_tokens")
	}

	summary.outputTokens, summary.hasOutput = intFromMap(data, "output_tokens")
	if !summary.hasOutput {
		summary.outputTokens, summary.hasOutput = intFromMap(data, "completion_tokens")
	}

	summary.totalTokens, summary.hasTotal = intFromMap(data, "total_tokens")

	if !summary.hasInput && !summary.hasOutput && !summary.hasTotal {
		return partialSummary{}, false
	}

	return summary, true
}

func intFromMap(data map[string]any, key string) (int, bool) {
	value, ok := data[key]
	if !ok {
		return 0, false
	}

	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float32:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}
