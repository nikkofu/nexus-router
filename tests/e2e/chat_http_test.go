package e2e

import "testing"

func TestChatCompletionsHTTPStreamingOpenAI(t *testing.T) {
	env := startHTTPTestEnv(t, "openai_text")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/chat/completions", env.Token, chatTextRequest("openai/gpt-4.1", true))
	assertStatus(t, resp, 200)
	assertHeaderContains(t, resp, "Content-Type", "text/event-stream")

	body := readBody(t, resp)
	assertBodyContains(t, body, "chat.completion.chunk", "[DONE]")

	if env.Primary.Hits() != 1 {
		t.Fatalf("primary hits = %d, want 1", env.Primary.Hits())
	}
}

func TestChatCompletionsHTTPNonStreamingOpenAI(t *testing.T) {
	env := startHTTPTestEnv(t, "openai_text")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/chat/completions", env.Token, chatTextRequest("openai/gpt-4.1", false))
	assertStatus(t, resp, 200)
	assertHeaderContains(t, resp, "Content-Type", "application/json")

	body := readBody(t, resp)
	assertBodyContains(t, body, "\"object\":\"chat.completion\"", "\"content\":\"hello\"")
}

func TestChatCompletionsHTTPStreamingAnthropic(t *testing.T) {
	env := startHTTPTestEnv(t, "anthropic_text")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/chat/completions", env.Token, chatTextRequest("anthropic/claude-sonnet-4-5", true))
	assertStatus(t, resp, 200)
	assertHeaderContains(t, resp, "Content-Type", "text/event-stream")

	body := readBody(t, resp)
	assertBodyContains(t, body, "chat.completion.chunk", "[DONE]")

	if env.Primary.Hits() != 1 {
		t.Fatalf("primary hits = %d, want 1", env.Primary.Hits())
	}
}

func TestChatCompletionsHTTPNonStreamingAnthropic(t *testing.T) {
	env := startHTTPTestEnv(t, "anthropic_text")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/chat/completions", env.Token, chatTextRequest("anthropic/claude-sonnet-4-5", false))
	assertStatus(t, resp, 200)
	assertHeaderContains(t, resp, "Content-Type", "application/json")

	body := readBody(t, resp)
	assertBodyContains(t, body, "\"object\":\"chat.completion\"", "\"content\":\"hello\"")
}
