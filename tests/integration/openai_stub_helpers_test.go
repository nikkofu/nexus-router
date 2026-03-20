package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newOpenAIStubServer(t *testing.T, scenario string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch scenario {
		case "chat_stream":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n")
			_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		case "responses_stream":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hel\"}\n\n")
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"lo\"}\n\n")
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\"}\n\n")
		case "rate_limit":
			http.Error(w, `{"error":{"message":"rate limit"}}`, http.StatusTooManyRequests)
		default:
			http.Error(w, "unknown scenario", http.StatusInternalServerError)
		}
	}))
}
