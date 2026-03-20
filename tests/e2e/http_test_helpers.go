package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/nikkofu/nexus-router/internal/auth"
	"github.com/nikkofu/nexus-router/internal/config"
)

type requestCapture struct {
	mu   sync.RWMutex
	path string
}

func (c *requestCapture) setPath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.path = path
}

func (c *requestCapture) Path() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.path
}

func newProviderDispatchStubServer(t *testing.T) (*httptest.Server, *requestCapture) {
	t.Helper()

	capture := &requestCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.setPath(r.URL.Path)

		switch r.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		case "/v1/messages":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"delta\":{\"text\":\"hello\"}}\n\n")
			_, _ = fmt.Fprint(w, "event: message_stop\ndata: {}\n\n")
		default:
			http.Error(w, "unknown path", http.StatusNotFound)
		}
	}))

	return server, capture
}

func newTestResolver(keys ...config.ClientKeyConfig) auth.Resolver {
	return auth.NewResolver(keys)
}
