package conformance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nikkofu/nexus-router/internal/auth"
	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/capabilities"
	"github.com/nikkofu/nexus-router/internal/circuitbreaker"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/health"
	"github.com/nikkofu/nexus-router/internal/orchestrator"
	"github.com/nikkofu/nexus-router/internal/providers"
	anthropicprovider "github.com/nikkofu/nexus-router/internal/providers/anthropic"
	openaiprovider "github.com/nikkofu/nexus-router/internal/providers/openai"
	"github.com/nikkofu/nexus-router/internal/router"
	"github.com/nikkofu/nexus-router/internal/streaming"
)

func TestConformanceMatrix(t *testing.T) {
	t.Run("chat_text_openai", func(t *testing.T) {
		server := newOpenAIStubServer(t, "chat_stream")
		adapter := openaiprovider.NewAdapter(server.Client())

		result, err := adapter.Execute(context.Background(), config.ProviderConfig{
			BaseURL: server.URL,
		}, canonical.Request{
			EndpointKind: canonical.EndpointKindChatCompletions,
			PublicModel:  "openai/gpt-4.1",
			Stream:       true,
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		rec := httptest.NewRecorder()
		if err := streaming.WriteChatCompletionSSE(rec, result.Events); err != nil {
			t.Fatalf("WriteChatCompletionSSE() error = %v", err)
		}

		assertContainsAll(t, rec.Body.String(), "chat.completion.chunk", "[DONE]")
	})

	t.Run("chat_text_anthropic", func(t *testing.T) {
		server, _ := newAnthropicStubServer(t, "messages_stream")
		adapter := anthropicprovider.NewAdapter(server.Client())

		result, err := adapter.Execute(context.Background(), config.ProviderConfig{
			BaseURL: server.URL,
		}, canonical.Request{
			EndpointKind: canonical.EndpointKindChatCompletions,
			PublicModel:  "anthropic/claude-sonnet-4-5",
			Stream:       true,
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		rec := httptest.NewRecorder()
		if err := streaming.WriteChatCompletionSSE(rec, result.Events); err != nil {
			t.Fatalf("WriteChatCompletionSSE() error = %v", err)
		}

		assertContainsAll(t, rec.Body.String(), "chat.completion.chunk", "[DONE]")
	})

	t.Run("responses_text_openai", func(t *testing.T) {
		server := newOpenAIStubServer(t, "responses_stream")
		adapter := openaiprovider.NewAdapter(server.Client())

		result, err := adapter.Execute(context.Background(), config.ProviderConfig{
			BaseURL: server.URL,
		}, canonical.Request{
			EndpointKind: canonical.EndpointKindResponses,
			PublicModel:  "openai/gpt-4.1",
			Stream:       true,
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		rec := httptest.NewRecorder()
		if err := streaming.WriteResponsesSSE(rec, result.Events); err != nil {
			t.Fatalf("WriteResponsesSSE() error = %v", err)
		}

		assertContainsAll(t, rec.Body.String(), "response.output_text.delta", "response.completed")
	})

	t.Run("responses_text_anthropic", func(t *testing.T) {
		server, _ := newAnthropicStubServer(t, "messages_stream")
		adapter := anthropicprovider.NewAdapter(server.Client())

		result, err := adapter.Execute(context.Background(), config.ProviderConfig{
			BaseURL: server.URL,
		}, canonical.Request{
			EndpointKind: canonical.EndpointKindResponses,
			PublicModel:  "anthropic/claude-sonnet-4-5",
			Stream:       true,
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		rec := httptest.NewRecorder()
		if err := streaming.WriteResponsesSSE(rec, result.Events); err != nil {
			t.Fatalf("WriteResponsesSSE() error = %v", err)
		}

		assertContainsAll(t, rec.Body.String(), "response.output_text.delta", "response.completed")
	})

	t.Run("tool_call_stream", func(t *testing.T) {
		server, capture := newAnthropicStubServer(t, "tool_use_stream")
		adapter := anthropicprovider.NewAdapter(server.Client())

		result, err := adapter.Execute(context.Background(), config.ProviderConfig{
			BaseURL: server.URL,
		}, canonical.Request{
			EndpointKind: canonical.EndpointKindChatCompletions,
			PublicModel:  "anthropic/claude-sonnet-4-5",
			Tools: []canonical.Tool{
				{
					Name: "lookup_weather",
					Schema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"city": map[string]any{"type": "string"},
						},
						"required": []any{"city"},
					},
				},
			},
			Stream: true,
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		rec := httptest.NewRecorder()
		if err := streaming.WriteChatCompletionSSE(rec, result.Events); err != nil {
			t.Fatalf("WriteChatCompletionSSE() error = %v", err)
		}

		assertContainsAll(t, capture.Body(), `"tools":[`, `"name":"lookup_weather"`)
		assertContainsAll(t, rec.Body.String(), `"tool_calls"`, `"function"`)
	})

	t.Run("vision_request", func(t *testing.T) {
		server, capture := newAnthropicStubServer(t, "messages_stream")
		adapter := anthropicprovider.NewAdapter(server.Client())

		_, err := adapter.Execute(context.Background(), config.ProviderConfig{
			BaseURL: server.URL,
		}, canonical.Request{
			EndpointKind: canonical.EndpointKindChatCompletions,
			PublicModel:  "anthropic/claude-sonnet-4-5",
			Conversation: []canonical.Turn{
				{
					Role: canonical.RoleUser,
					Content: []canonical.ContentBlock{
						{Type: canonical.ContentTypeText, Text: "describe"},
						{Type: canonical.ContentTypeImage, Image: &canonical.ImageInput{
							URL:      "https://example.com/cat.png",
							MIMEType: "image/png",
						}},
					},
				},
			},
			Stream: true,
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		assertContainsAll(t, capture.Body(), `"type":"image"`, `"url":"https://example.com/cat.png"`)
	})

	t.Run("supported_structured_output", func(t *testing.T) {
		req := canonical.Request{
			PublicModel: "anthropic/claude-sonnet-4-5",
			ResponseContract: canonical.ResponseContract{
				Kind: canonical.ResponseContractJSONSchema,
				Schema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"answer": map[string]any{"type": "string"},
					},
					"required":             []any{"answer"},
					"additionalProperties": false,
				},
			},
		}

		policy := auth.ClientPolicy{
			ID:              "structured",
			AllowStructured: true,
			AllowVision:     true,
			AllowTools:      true,
			AllowStreaming:  true,
		}

		if err := capabilities.ValidateRequest(capabilities.DefaultRegistry(), policy, req); err != nil {
			t.Fatalf("ValidateRequest() error = %v", err)
		}
	})

	t.Run("rejected_unsupported_schema", func(t *testing.T) {
		req := canonical.Request{
			PublicModel: "anthropic/claude-sonnet-4-5",
			ResponseContract: canonical.ResponseContract{
				Kind: canonical.ResponseContractJSONSchema,
				Schema: map[string]any{
					"oneOf": []any{},
				},
			},
		}

		policy := auth.ClientPolicy{
			ID:              "structured",
			AllowStructured: true,
			AllowVision:     true,
			AllowTools:      true,
			AllowStreaming:  true,
		}

		if err := capabilities.ValidateRequest(capabilities.DefaultRegistry(), policy, req); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("pre_output_failover", func(t *testing.T) {
		cfg := failoverConfig()
		manager := health.NewManager()
		planner := router.NewPlanner(cfg, manager)
		executor := &fakeExecutor{
			results: map[string]fakeResult{
				"openai-main": {
					err: &providers.ExecutionError{
						Err:             errors.New("rate limited"),
						Retryable:       true,
						OutputCommitted: false,
					},
				},
				"openai-backup": {
					result: providers.Result{
						Events: []canonical.Event{
							{Type: canonical.EventContentDelta, Data: map[string]any{"text": "hello"}},
						},
					},
				},
			},
		}

		runner := orchestrator.New(planner, executor)
		outcome, err := runner.Run(context.Background(), canonical.Request{PublicModel: "openai/gpt-4.1"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if len(outcome.Attempts) != 2 {
			t.Fatalf("attempts = %d, want 2", len(outcome.Attempts))
		}
	})

	t.Run("no_failover_after_partial_output", func(t *testing.T) {
		cfg := failoverConfig()
		manager := health.NewManager()
		planner := router.NewPlanner(cfg, manager)
		executor := &fakeExecutor{
			results: map[string]fakeResult{
				"openai-main": {
					err: &providers.ExecutionError{
						Err:             errors.New("partial stream"),
						Retryable:       true,
						OutputCommitted: true,
					},
				},
			},
		}

		runner := orchestrator.New(planner, executor)
		outcome, err := runner.Run(context.Background(), canonical.Request{PublicModel: "openai/gpt-4.1"})
		if err == nil {
			t.Fatal("expected error")
		}
		if len(outcome.Attempts) != 1 {
			t.Fatalf("attempts = %d, want 1", len(outcome.Attempts))
		}
	})
}

func assertContainsAll(t *testing.T, value string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(value, part) {
			t.Fatalf("expected %q to contain %q", value, part)
		}
	}
}

func newOpenAIStubServer(t *testing.T, scenario string) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	return server
}

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
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		capture.setBody(string(payload))

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

func failoverConfig() config.Config {
	return config.Config{
		Models: []config.ModelConfig{
			{
				Pattern:    "openai/gpt-*",
				RouteGroup: "openai-family",
			},
		},
		Routing: config.RoutingConfig{
			RouteGroups: []config.RouteGroupConfig{
				{
					Name:      "openai-family",
					Primary:   "openai-main",
					Fallbacks: []string{"openai-backup"},
				},
			},
		},
		Providers: []config.ProviderConfig{
			{Name: "openai-main", Provider: "openai", BaseURL: "https://api.openai.com"},
			{Name: "openai-backup", Provider: "openai", BaseURL: "https://backup.openai.example"},
		},
	}
}

type fakeExecutor struct {
	results map[string]fakeResult
}

type fakeResult struct {
	result providers.Result
	err    error
}

func (f *fakeExecutor) Execute(_ context.Context, upstream string, _ canonical.Request) (providers.Result, error) {
	result, ok := f.results[upstream]
	if !ok {
		return providers.Result{}, errors.New("unexpected upstream")
	}
	return result.result, result.err
}

func TestBreakerTransitionsRemainUsable(t *testing.T) {
	b := circuitbreaker.New(3, time.Minute, 1)
	for i := 0; i < 3; i++ {
		b.RecordFailure()
	}
	if got := b.State(); got != circuitbreaker.StateOpen {
		t.Fatalf("state = %q, want %q", got, circuitbreaker.StateOpen)
	}
}
