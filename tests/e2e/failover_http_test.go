package e2e

import "testing"

func TestChatHTTPFailsOverBeforeOutputCommit(t *testing.T) {
	env := startHTTPTestEnv(t, "primary_429_backup_success")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/chat/completions", env.Token, chatTextRequest("openai/gpt-4.1", false))
	assertStatus(t, resp, 200)

	body := readBody(t, resp)
	assertBodyContains(t, body, "\"object\":\"chat.completion\"", "\"content\":\"hello\"")

	if env.Primary.Hits() != 1 {
		t.Fatalf("primary hits = %d, want 1", env.Primary.Hits())
	}
	if env.Backup == nil || env.Backup.Hits() != 1 {
		t.Fatalf("backup hits = %d, want 1", env.Backup.Hits())
	}
}

func TestChatHTTPDoesNotFailOverAfterOutputCommit(t *testing.T) {
	env := startHTTPTestEnv(t, "partial_stream_interrupt_after_output")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/chat/completions", env.Token, chatTextRequest("openai/gpt-4.1", true))
	assertStatus(t, resp, 502)

	body := readBody(t, resp)
	assertJSONErrorType(t, body, "upstream_error")

	if env.Primary.Hits() != 1 {
		t.Fatalf("primary hits = %d, want 1", env.Primary.Hits())
	}
	if env.Backup == nil || env.Backup.Hits() != 0 {
		t.Fatalf("backup hits = %d, want 0", env.Backup.Hits())
	}
}

func TestResponsesHTTPAllowsVisionOnPublicSurface(t *testing.T) {
	env := startHTTPTestEnv(t, "openai_responses")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/responses", env.Token, responsesVisionRequest("openai/gpt-4.1", false))
	assertStatus(t, resp, 200)

	body := readBody(t, resp)
	assertBodyContains(t, body, "\"object\":\"response\"", "\"text\":\"hello\"")

	if env.Primary.Hits() != 1 {
		t.Fatalf("primary hits = %d, want 1", env.Primary.Hits())
	}
}

func TestResponsesHTTPRejectsStructuredOutputOnPublicTextPath(t *testing.T) {
	env := startHTTPTestEnv(t, "openai_responses")
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/responses", env.Token, responsesStructuredOutputRequest())
	assertStatus(t, resp, 400)

	body := readBody(t, resp)
	assertJSONErrorType(t, body, "unsupported_capability")

	if env.Primary.Hits() != 0 {
		t.Fatalf("primary hits = %d, want 0", env.Primary.Hits())
	}
}
