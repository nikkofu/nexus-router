package e2e

import (
	"testing"

	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/health"
)

func TestReadyzReturns503WhileInitialProbePending(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-test-key")

	probe := newBlockingProbeStubServer(t)
	defer probe.Close()

	env := startCustomHTTPTestEnv(t, config.Config{
		Server: config.ServerConfig{
			ListenAddr: "127.0.0.1:0",
		},
		Providers: []config.ProviderConfig{
			{
				Name:      "openai-main",
				Provider:  "openai",
				BaseURL:   probe.URL(),
				APIKeyEnv: "OPENAI_API_KEY",
			},
		},
		Models: []config.ModelConfig{
			{
				Pattern:    "openai/gpt-*",
				RouteGroup: "openai-family",
			},
		},
		Routing: config.RoutingConfig{
			RouteGroups: []config.RouteGroupConfig{
				{
					Name:    "openai-family",
					Primary: "openai-main",
				},
			},
		},
		Health: config.HealthConfig{
			ProbeInterval:       "50ms",
			ProbeTimeout:        "1s",
			RequireInitialProbe: true,
		},
	})
	defer env.Close()

	waitForRuntimeSnapshot(t, env.Client, env.BaseURL, func(snapshot health.RuntimeSnapshot) bool {
		return snapshot.Started && !snapshot.InitialProbeComplete
	})

	resp := get(t, env.Client, env.BaseURL+"/readyz")
	assertStatus(t, resp, 503)
}

func TestReadyzReturns200WhenAllRequiredRouteGroupsHaveEligibleUpstreams(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-test-key")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-test-key")

	openAIProbe := newProbeStubServer(t, 200)
	defer openAIProbe.Close()
	anthropicProbe := newProbeStubServer(t, 200)
	defer anthropicProbe.Close()

	env := startCustomHTTPTestEnv(t, config.Config{
		Server: config.ServerConfig{
			ListenAddr: "127.0.0.1:0",
		},
		Providers: []config.ProviderConfig{
			{
				Name:      "openai-main",
				Provider:  "openai",
				BaseURL:   openAIProbe.URL(),
				APIKeyEnv: "OPENAI_API_KEY",
			},
			{
				Name:      "anthropic-main",
				Provider:  "anthropic",
				BaseURL:   anthropicProbe.URL(),
				APIKeyEnv: "ANTHROPIC_API_KEY",
			},
		},
		Models: []config.ModelConfig{
			{
				Pattern:    "openai/gpt-*",
				RouteGroup: "openai-family",
			},
			{
				Pattern:    "anthropic/claude-*",
				RouteGroup: "anthropic-family",
			},
		},
		Routing: config.RoutingConfig{
			RouteGroups: []config.RouteGroupConfig{
				{
					Name:    "openai-family",
					Primary: "openai-main",
				},
				{
					Name:    "anthropic-family",
					Primary: "anthropic-main",
				},
			},
		},
		Health: config.HealthConfig{
			ProbeInterval:       "50ms",
			ProbeTimeout:        "1s",
			RequireInitialProbe: true,
		},
	})
	defer env.Close()

	resp := waitForStatus(t, env.Client, env.BaseURL+"/readyz", 200)
	assertStatus(t, resp, 200)
}
