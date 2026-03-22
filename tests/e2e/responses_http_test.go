package e2e

import "testing"

func TestResponsesHTTPStreamingOpenAI(t *testing.T) {
	env := startHTTPTestEnv(t, "openai_responses")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/responses", env.Token, responsesTextRequest("openai/gpt-4.1", true))
	assertStatus(t, resp, 200)
	assertHeaderContains(t, resp, "Content-Type", "text/event-stream")

	body := readBody(t, resp)
	assertBodyContains(t, body, "response.output_text.delta", "response.completed")
}

func TestResponsesHTTPNonStreamingOpenAI(t *testing.T) {
	env := startHTTPTestEnv(t, "openai_responses")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/responses", env.Token, responsesTextRequest("openai/gpt-4.1", false))
	assertStatus(t, resp, 200)
	assertHeaderContains(t, resp, "Content-Type", "application/json")

	body := readBody(t, resp)
	assertBodyContains(t, body, "\"object\":\"response\"", "\"text\":\"hello\"")
}

func TestResponsesHTTPNonStreamingOpenAIForcesEventModeAndIncludesUsage(t *testing.T) {
	env := startHTTPTestEnv(t, "openai_responses_usage")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/responses", env.Token, responsesTextRequest("openai/gpt-4.1", false))
	assertStatus(t, resp, 200)
	assertHeaderContains(t, resp, "Content-Type", "application/json")

	body := readBody(t, resp)
	assertBodyContains(t, body, "\"text\":\"hello\"", "\"input_tokens\":11", "\"output_tokens\":7", "\"total_tokens\":18")
	assertBodyContains(t, env.Primary.Body(), "\"stream\":true")
}

func TestResponsesHTTPStreamingAnthropic(t *testing.T) {
	env := startHTTPTestEnv(t, "anthropic_text")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/responses", env.Token, responsesTextRequest("anthropic/claude-sonnet-4-5", true))
	assertStatus(t, resp, 200)
	assertHeaderContains(t, resp, "Content-Type", "text/event-stream")

	body := readBody(t, resp)
	assertBodyContains(t, body, "response.output_text.delta", "response.completed")
}

func TestResponsesHTTPNonStreamingAnthropic(t *testing.T) {
	env := startHTTPTestEnv(t, "anthropic_text")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/responses", env.Token, responsesTextRequest("anthropic/claude-sonnet-4-5", false))
	assertStatus(t, resp, 200)
	assertHeaderContains(t, resp, "Content-Type", "application/json")

	body := readBody(t, resp)
	assertBodyContains(t, body, "\"object\":\"response\"", "\"text\":\"hello\"")
}

func TestResponsesHTTPNonStreamingAnthropicForcesEventModeAndIncludesUsage(t *testing.T) {
	env := startHTTPTestEnv(t, "anthropic_text_usage")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/responses", env.Token, responsesTextRequest("anthropic/claude-sonnet-4-5", false))
	assertStatus(t, resp, 200)
	assertHeaderContains(t, resp, "Content-Type", "application/json")

	body := readBody(t, resp)
	assertBodyContains(t, body, "\"text\":\"hello\"", "\"input_tokens\":11", "\"output_tokens\":7", "\"total_tokens\":18")
	assertBodyContains(t, env.Primary.Body(), "\"stream\":true")
}

func TestResponsesHTTPRejectsToolsOnPublicSurface(t *testing.T) {
	env := startHTTPTestEnv(t, "openai_responses")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/responses", env.Token, map[string]any{
		"model":  "openai/gpt-4.1",
		"stream": true,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": "weather?",
					},
				},
			},
		},
		"tools": []map[string]any{
			{
				"type": "function",
				"name": "lookup_weather",
				"parameters": map[string]any{
					"type": "object",
				},
			},
		},
	})
	assertStatus(t, resp, 400)

	body := readBody(t, resp)
	assertJSONErrorType(t, body, "unsupported_capability")

	if env.Primary.Hits() != 0 {
		t.Fatalf("primary hits = %d, want 0", env.Primary.Hits())
	}
}
