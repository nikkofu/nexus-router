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

func TestResponsesHTTPStreamingOpenAIVision(t *testing.T) {
	env := startHTTPTestEnv(t, "openai_responses")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/responses", env.Token, responsesVisionRequest("openai/gpt-4.1", true))
	assertStatus(t, resp, 200)
	assertHeaderContains(t, resp, "Content-Type", "text/event-stream")

	body := readBody(t, resp)
	assertBodyContains(t, body, "response.output_text.delta", "response.completed")
	assertBodyContains(t, env.Primary.Body(), `"type":"input_image"`, `"image_url":"https://example.com/cat.png"`)
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

func TestResponsesHTTPStreamingAnthropicVision(t *testing.T) {
	env := startHTTPTestEnv(t, "anthropic_text")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/responses", env.Token, responsesVisionRequest("anthropic/claude-sonnet-4-5", true))
	assertStatus(t, resp, 200)
	assertHeaderContains(t, resp, "Content-Type", "text/event-stream")

	body := readBody(t, resp)
	assertBodyContains(t, body, "response.output_text.delta", "response.completed")
	assertBodyContains(t, env.Primary.Body(), `"type":"image"`, `"source":{"type":"url","url":"https://example.com/cat.png"}`)
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

func TestResponsesHTTPRejectsUnsupportedPublicImageForms(t *testing.T) {
	cases := []struct {
		name          string
		payload       map[string]any
		wantErrorType string
	}{
		{
			name: "malformed image item shape",
			payload: map[string]any{
				"model":  "openai/gpt-4.1",
				"stream": true,
				"input": []map[string]any{
					{
						"role": "user",
						"content": []map[string]any{
							{
								"type":      "input_image",
								"image_url": map[string]any{"url": "https://example.com/cat.png"},
							},
						},
					},
				},
			},
			wantErrorType: "invalid_request",
		},
		{
			name: "empty image url",
			payload: map[string]any{
				"model":  "openai/gpt-4.1",
				"stream": true,
				"input": []map[string]any{
					{
						"role": "user",
						"content": []map[string]any{
							{
								"type":      "input_image",
								"image_url": "",
							},
						},
					},
				},
			},
			wantErrorType: "invalid_request",
		},
		{
			name: "data url image",
			payload: map[string]any{
				"model":  "openai/gpt-4.1",
				"stream": true,
				"input": []map[string]any{
					{
						"role": "user",
						"content": []map[string]any{
							{
								"type":      "input_image",
								"image_url": "data:image/png;base64,AAAA",
							},
						},
					},
				},
			},
			wantErrorType: "unsupported_capability",
		},
		{
			name: "file id image form",
			payload: map[string]any{
				"model":  "openai/gpt-4.1",
				"stream": true,
				"input": []map[string]any{
					{
						"role": "user",
						"content": []map[string]any{
							{
								"type":    "input_image",
								"file_id": "file_abc123",
							},
						},
					},
				},
			},
			wantErrorType: "unsupported_capability",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := startHTTPTestEnv(t, "openai_responses")
			defer env.Close()

			resp := postJSON(t, env.Client, env.BaseURL+"/v1/responses", env.Token, tc.payload)
			assertStatus(t, resp, 400)

			body := readBody(t, resp)
			assertJSONErrorType(t, body, tc.wantErrorType)

			if env.Primary.Hits() != 0 {
				t.Fatalf("primary hits = %d, want 0", env.Primary.Hits())
			}
		})
	}
}
