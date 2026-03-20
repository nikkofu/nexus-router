package integration

import (
	"testing"
	"time"

	"github.com/nikkofu/nexus-router/internal/circuitbreaker"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/health"
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
