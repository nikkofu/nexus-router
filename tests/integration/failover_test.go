package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/circuitbreaker"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/health"
	"github.com/nikkofu/nexus-router/internal/orchestrator"
	"github.com/nikkofu/nexus-router/internal/providers"
	"github.com/nikkofu/nexus-router/internal/router"
)

func TestBreakerOpensAfterThreshold(t *testing.T) {
	b := circuitbreaker.New(3, time.Minute, 1)
	for i := 0; i < 3; i++ {
		b.RecordFailure()
	}

	if state := b.State(); state != circuitbreaker.StateOpen {
		t.Fatalf("state = %q, want %q", state, circuitbreaker.StateOpen)
	}
}

func TestPlannerSkipsEjectedUpstream(t *testing.T) {
	cfg := failoverConfig()
	manager := health.NewManager()
	manager.Eject("openai-main", time.Minute)

	planner := router.NewPlanner(cfg, manager)
	plan, err := planner.Plan("openai/gpt-4.1")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if len(plan.Attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(plan.Attempts))
	}
	if plan.Attempts[0].Upstream != "openai-backup" {
		t.Fatalf("upstream = %q, want %q", plan.Attempts[0].Upstream, "openai-backup")
	}
}

func TestPlannerReturnsOrderedFallbacks(t *testing.T) {
	cfg := failoverConfig()
	manager := health.NewManager()

	planner := router.NewPlanner(cfg, manager)
	plan, err := planner.Plan("openai/gpt-4.1")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if len(plan.Attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(plan.Attempts))
	}
	if plan.Attempts[0].Upstream != "openai-main" {
		t.Fatalf("first upstream = %q, want %q", plan.Attempts[0].Upstream, "openai-main")
	}
	if plan.Attempts[1].Upstream != "openai-backup" {
		t.Fatalf("second upstream = %q, want %q", plan.Attempts[1].Upstream, "openai-backup")
	}
}

func TestFailoverOccursBeforeAnyOutput(t *testing.T) {
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
						{Type: canonical.EventMessageStop},
					},
				},
			},
		},
	}

	runner := orchestrator.New(planner, executor)
	outcome, err := runner.Run(context.Background(), canonical.Request{
		PublicModel: "openai/gpt-4.1",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(outcome.Attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(outcome.Attempts))
	}
	if outcome.Attempts[0] != "openai-main" || outcome.Attempts[1] != "openai-backup" {
		t.Fatalf("unexpected attempts = %#v", outcome.Attempts)
	}
}

func TestNoFailoverAfterPartialStream(t *testing.T) {
	cfg := failoverConfig()
	manager := health.NewManager()
	planner := router.NewPlanner(cfg, manager)
	executor := &fakeExecutor{
		results: map[string]fakeResult{
			"openai-main": {
				err: &providers.ExecutionError{
					Err:             errors.New("stream interrupted"),
					Retryable:       true,
					OutputCommitted: true,
				},
			},
			"openai-backup": {
				result: providers.Result{
					Events: []canonical.Event{
						{Type: canonical.EventContentDelta, Data: map[string]any{"text": "backup"}},
						{Type: canonical.EventMessageStop},
					},
				},
			},
		},
	}

	runner := orchestrator.New(planner, executor)
	outcome, err := runner.Run(context.Background(), canonical.Request{
		PublicModel: "openai/gpt-4.1",
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if len(outcome.Attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(outcome.Attempts))
	}
	if outcome.Attempts[0] != "openai-main" {
		t.Fatalf("unexpected attempts = %#v", outcome.Attempts)
	}
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
			{
				Name:     "openai-main",
				Provider: "openai",
				BaseURL:  "https://api.openai.com",
			},
			{
				Name:     "openai-backup",
				Provider: "openai",
				BaseURL:  "https://backup.openai.example",
			},
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
