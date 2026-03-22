package integration

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type openAICapture struct {
	mu      sync.RWMutex
	hits    int
	body    string
	headers http.Header
}

func (c *openAICapture) record(r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits++
	c.body = string(body)
	c.headers = r.Header.Clone()
}

func (c *openAICapture) Hits() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits
}

func (c *openAICapture) Body() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.body
}

func (c *openAICapture) Header(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.headers == nil {
		return ""
	}
	return c.headers.Get(key)
}

func newOpenAIStubServer(t *testing.T, scenario string) *httptest.Server {
	t.Helper()

	server, _ := newOpenAICaptureStubServer(t, scenario)
	return server
}

func newOpenAICaptureStubServer(t *testing.T, scenario string) (*httptest.Server, *openAICapture) {
	t.Helper()

	capture := &openAICapture{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.record(r)

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
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	server.Start()

	return server, capture
}
