package e2e

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nikkofu/nexus-router/internal/auth"
	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/capabilities"
	"github.com/nikkofu/nexus-router/internal/providers"
	"github.com/nikkofu/nexus-router/internal/router"
	"github.com/nikkofu/nexus-router/internal/service"
)

func TestPublicTextServiceRejectsUnsupportedCapabilities(t *testing.T) {
	type testCase struct {
		name string
		req  canonical.Request
	}

	cases := []testCase{
		{
			name: "image content",
			req: canonical.Request{
				EndpointKind: canonical.EndpointKindChatCompletions,
				PublicModel: "openai/gpt-4.1",
				Conversation: []canonical.Turn{
					{
						Role: canonical.RoleUser,
						Content: []canonical.ContentBlock{
							{
								Type:  canonical.ContentTypeImage,
								Image: &canonical.ImageInput{URL: "https://example.com/image.png", MIMEType: "image/png"},
							},
						},
					},
				},
			},
		},
		{
			name: "structured output contract",
			req: canonical.Request{
				EndpointKind: canonical.EndpointKindChatCompletions,
				PublicModel: "openai/gpt-4.1",
				ResponseContract: canonical.ResponseContract{
					Kind: canonical.ResponseContractJSONSchema,
					Schema: map[string]any{
						"type": "object",
					},
				},
			},
		},
	}

	planner := &stubPlanner{
		plan: router.Plan{
			Attempts: []router.Attempt{{Upstream: "openai-main"}},
		},
	}
	executor := &stubExecutor{}
	svc := service.NewExecuteService(capabilities.DefaultRegistry(), planner, executor)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := svc.Execute(context.Background(), auth.ClientPolicy{
				AllowStreaming:  true,
				AllowVision:     true,
				AllowTools:      true,
				AllowStructured: true,
			}, tc.req)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, service.ErrUnsupportedCapability) {
				t.Fatalf("error = %v, want ErrUnsupportedCapability", err)
			}
		})
	}

	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestPublicTextServiceAllowsChatTools(t *testing.T) {
	planner := &stubPlanner{
		plan: router.Plan{
			Attempts: []router.Attempt{{Upstream: "anthropic-main"}},
		},
	}
	executor := &stubExecutor{
		result: providers.Result{
			Events: []canonical.Event{
				{Type: canonical.EventMessageStop, Data: map[string]any{"finish_reason": "tool_calls"}},
			},
		},
	}
	svc := service.NewExecuteService(capabilities.DefaultRegistry(), planner, executor)

	_, _, err := svc.Execute(context.Background(), auth.ClientPolicy{
		AllowStreaming:  true,
		AllowVision:     true,
		AllowTools:      true,
		AllowStructured: true,
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "anthropic/claude-sonnet-4-5",
		Tools: []canonical.Tool{
			{
				Name: "lookup_weather",
				Schema: map[string]any{
					"type": "object",
				},
			},
		},
		Stream: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if planner.calls != 1 {
		t.Fatalf("planner calls = %d, want 1", planner.calls)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
}

func TestPublicTextServiceRejectsResponsesTools(t *testing.T) {
	planner := &stubPlanner{
		plan: router.Plan{
			Attempts: []router.Attempt{{Upstream: "openai-main"}},
		},
	}
	executor := &stubExecutor{}
	svc := service.NewExecuteService(capabilities.DefaultRegistry(), planner, executor)

	_, _, err := svc.Execute(context.Background(), auth.ClientPolicy{
		AllowStreaming:  true,
		AllowVision:     true,
		AllowTools:      true,
		AllowStructured: true,
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindResponses,
		PublicModel:  "openai/gpt-4.1",
		Tools: []canonical.Tool{
			{
				Name: "lookup_weather",
				Schema: map[string]any{
					"type": "object",
				},
			},
		},
		Stream: true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, service.ErrUnsupportedCapability) {
		t.Fatalf("error = %v, want ErrUnsupportedCapability", err)
	}
	if planner.calls != 0 {
		t.Fatalf("planner calls = %d, want 0", planner.calls)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestPublicTextServiceExecutesViaOrchestratorAndReturnsAttempts(t *testing.T) {
	planner := &stubPlanner{
		plan: router.Plan{
			Attempts: []router.Attempt{{Upstream: "openai-main"}},
		},
	}
	executor := &stubExecutor{
		result: providers.Result{
			Events: []canonical.Event{
				{Type: canonical.EventContentDelta, Data: map[string]any{"text": "hello"}},
			},
		},
	}
	svc := service.NewExecuteService(capabilities.DefaultRegistry(), planner, executor)

	result, attempts, err := svc.Execute(context.Background(), auth.ClientPolicy{
		AllowStreaming:  true,
		AllowVision:     true,
		AllowTools:      true,
		AllowStructured: true,
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "openai/gpt-4.1",
		Conversation: []canonical.Turn{
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{
					{Type: canonical.ContentTypeText, Text: "hello"},
				},
			},
		},
		Stream: true,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(result.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(result.Events))
	}
	if len(attempts) != 1 || attempts[0] != "openai-main" {
		t.Fatalf("attempts = %#v, want [openai-main]", attempts)
	}
	if planner.calls != 1 {
		t.Fatalf("planner calls = %d, want 1", planner.calls)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
}

func TestPublicTextServiceRunsCapabilityValidationBeforeOrchestrator(t *testing.T) {
	planner := &stubPlanner{
		plan: router.Plan{
			Attempts: []router.Attempt{{Upstream: "openai-main"}},
		},
	}
	executor := &stubExecutor{}
	svc := service.NewExecuteService(capabilities.DefaultRegistry(), planner, executor)

	_, _, err := svc.Execute(context.Background(), auth.ClientPolicy{
		AllowStreaming:  true,
		AllowVision:     true,
		AllowTools:      true,
		AllowStructured: true,
	}, canonical.Request{
		EndpointKind: canonical.EndpointKindChatCompletions,
		PublicModel:  "unknown/model",
		Stream:       true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported managed model family") {
		t.Fatalf("error = %q, want capability validation failure", err.Error())
	}
	if planner.calls != 0 {
		t.Fatalf("planner calls = %d, want 0", planner.calls)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

type stubPlanner struct {
	plan  router.Plan
	err   error
	calls int
}

func (s *stubPlanner) Plan(publicModel string) (router.Plan, error) {
	s.calls++
	return s.plan, s.err
}

type stubExecutor struct {
	result providers.Result
	err    error
	calls  int
}

func (s *stubExecutor) Execute(_ context.Context, _ string, _ canonical.Request) (providers.Result, error) {
	s.calls++
	return s.result, s.err
}
