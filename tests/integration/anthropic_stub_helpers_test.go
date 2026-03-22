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

type anthropicCapture struct {
	mu      sync.RWMutex
	hits    int
	body    string
	headers http.Header
}

func (c *anthropicCapture) record(r *http.Request, body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits++
	c.body = body
	c.headers = r.Header.Clone()
}

func (c *anthropicCapture) Body() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.body
}

func (c *anthropicCapture) Hits() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits
}

func (c *anthropicCapture) Header(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.headers == nil {
		return ""
	}
	return c.headers.Get(key)
}

func newAnthropicStubServer(t *testing.T, scenario string) (*httptest.Server, *anthropicCapture) {
	t.Helper()

	capture := &anthropicCapture{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		capture.record(r, string(payload))

		switch scenario {
		case "messages_stream":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"delta\":{\"text\":\"hel\"}}\n\n")
			_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"delta\":{\"text\":\"lo\"}}\n\n")
			_, _ = fmt.Fprint(w, "event: message_stop\ndata: {}\n\n")
		case "tool_use_stream":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "event: content_block_start\ndata: {\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"lookup_weather\"}}\n\n")
			_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"delta\":{\"partial_json\":\"{\\\"city\\\":\\\"Shanghai\\\"}\"}}\n\n")
			_, _ = fmt.Fprint(w, "event: message_stop\ndata: {}\n\n")
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
