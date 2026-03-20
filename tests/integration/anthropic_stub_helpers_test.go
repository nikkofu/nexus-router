package integration

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type anthropicCapture struct {
	mu   sync.RWMutex
	body string
}

func (c *anthropicCapture) setBody(body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.body = body
}

func (c *anthropicCapture) Body() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.body
}

func newAnthropicStubServer(t *testing.T, scenario string) (*httptest.Server, *anthropicCapture) {
	t.Helper()

	capture := &anthropicCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		capture.setBody(string(payload))

		switch scenario {
		case "messages_stream":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"delta\":{\"text\":\"hel\"}}\n\n")
			_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"delta\":{\"text\":\"lo\"}}\n\n")
			_, _ = fmt.Fprint(w, "event: message_stop\ndata: {}\n\n")
		default:
			http.Error(w, "unknown scenario", http.StatusInternalServerError)
		}
	}))

	return server, capture
}
