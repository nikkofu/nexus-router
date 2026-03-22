package e2e

import (
	"testing"

	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/health"
)

func TestPreOutputRetryableFailureEjectsPrimaryAndEnablesFallback(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-test-key")

	env := startHTTPTestEnvWithConfigMutator(t, "primary_429_backup_success", func(cfg *config.Config) {
		cfg.Breaker.FailureThreshold = 1
		cfg.Health.ProbeInterval = "1h"
		for i := range cfg.Providers {
			cfg.Providers[i].APIKeyEnv = "OPENAI_API_KEY"
		}
	})
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/chat/completions", env.Token, chatTextRequest("openai/gpt-4.1", false))
	assertStatus(t, resp, 200)

	body := readBody(t, resp)
	assertBodyContains(t, body, "\"object\":\"chat.completion\"", "\"content\":\"hello\"")

	snapshot := waitForRuntimeSnapshot(t, env.Client, env.BaseURL, func(snapshot health.RuntimeSnapshot) bool {
		primary, ok := runtimeUpstreamByName(snapshot, "openai-main")
		if !ok || primary.State != health.StateOpen {
			return false
		}

		backup, ok := runtimeUpstreamByName(snapshot, "openai-backup")
		return ok && backup.Eligible
	})

	primary, ok := runtimeUpstreamByName(snapshot, "openai-main")
	if !ok {
		t.Fatal("missing runtime snapshot for openai-main")
	}
	if primary.State != health.StateOpen {
		t.Fatalf("primary state = %q, want %q", primary.State, health.StateOpen)
	}
	if primary.Eligible {
		t.Fatal("primary eligible = true, want false")
	}
	if primary.BreakerState != health.BreakerStateOpen {
		t.Fatalf("primary breaker_state = %q, want %q", primary.BreakerState, health.BreakerStateOpen)
	}
	if primary.Source != health.SourceRequest {
		t.Fatalf("primary source = %q, want %q", primary.Source, health.SourceRequest)
	}

	backup, ok := runtimeUpstreamByName(snapshot, "openai-backup")
	if !ok {
		t.Fatal("missing runtime snapshot for openai-backup")
	}
	if backup.State != health.StateHealthy {
		t.Fatalf("backup state = %q, want %q", backup.State, health.StateHealthy)
	}
	if !backup.Eligible {
		t.Fatal("backup eligible = false, want true")
	}
	if backup.Source != health.SourceRequest {
		t.Fatalf("backup source = %q, want %q", backup.Source, health.SourceRequest)
	}

	secondResp := postJSON(t, env.Client, env.BaseURL+"/v1/chat/completions", env.Token, chatTextRequest("openai/gpt-4.1", false))
	assertStatus(t, secondResp, 200)

	if env.Primary.Hits() != 1 {
		t.Fatalf("primary hits = %d, want 1", env.Primary.Hits())
	}
	if env.Backup == nil || env.Backup.Hits() != 2 {
		t.Fatalf("backup hits = %d, want 2", env.Backup.Hits())
	}
}

func TestPostOutputFailureDoesNotEjectPrimary(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-test-key")

	env := startHTTPTestEnvWithConfigMutator(t, "partial_stream_interrupt_after_output", func(cfg *config.Config) {
		cfg.Breaker.FailureThreshold = 1
		cfg.Health.ProbeInterval = "1h"
		for i := range cfg.Providers {
			cfg.Providers[i].APIKeyEnv = "OPENAI_API_KEY"
		}
	})
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/chat/completions", env.Token, chatTextRequest("openai/gpt-4.1", true))
	assertStatus(t, resp, 502)

	body := readBody(t, resp)
	assertJSONErrorType(t, body, "upstream_error")

	snapshot := waitForRuntimeSnapshot(t, env.Client, env.BaseURL, func(snapshot health.RuntimeSnapshot) bool {
		primary, ok := runtimeUpstreamByName(snapshot, "openai-main")
		return ok && primary.LastError != ""
	})

	primary, ok := runtimeUpstreamByName(snapshot, "openai-main")
	if !ok {
		t.Fatal("missing runtime snapshot for openai-main")
	}
	if primary.State != health.StateHealthy {
		t.Fatalf("primary state = %q, want %q", primary.State, health.StateHealthy)
	}
	if !primary.Eligible {
		t.Fatal("primary eligible = false, want true")
	}
	if primary.BreakerState != health.BreakerStateClosed {
		t.Fatalf("primary breaker_state = %q, want %q", primary.BreakerState, health.BreakerStateClosed)
	}
	if primary.LastError == "" {
		t.Fatal("primary last_error = empty, want non-empty")
	}
	if primary.Source != health.SourceRequest {
		t.Fatalf("primary source = %q, want %q", primary.Source, health.SourceRequest)
	}

	if env.Primary.Hits() != 1 {
		t.Fatalf("primary hits = %d, want 1", env.Primary.Hits())
	}
	if env.Backup == nil || env.Backup.Hits() != 0 {
		t.Fatalf("backup hits = %d, want 0", env.Backup.Hits())
	}
}

func TestHalfOpenProbeRecoveryReturnsPrimaryToHealthy(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-test-key")

	primary := newMutableHTTPProviderStub(t, "rate_limit", 200)
	defer primary.Close()
	backup := newMutableHTTPProviderStub(t, "openai_text", 200)
	defer backup.Close()

	env := startCustomHTTPTestEnv(t, runtimeRecoveryConfig(primary.URL(), backup.URL()))
	defer env.Close()

	resp := postJSON(t, env.Client, env.BaseURL+"/v1/chat/completions", env.Token, chatTextRequest("openai/gpt-4.1", false))
	assertStatus(t, resp, 200)

	waitForRuntimeSnapshot(t, env.Client, env.BaseURL, func(snapshot health.RuntimeSnapshot) bool {
		primaryUpstream, ok := runtimeUpstreamByName(snapshot, "openai-main")
		return ok && primaryUpstream.State == health.StateOpen
	})

	primary.SetScenario("openai_text")

	snapshot := waitForRuntimeSnapshot(t, env.Client, env.BaseURL, func(snapshot health.RuntimeSnapshot) bool {
		primaryUpstream, ok := runtimeUpstreamByName(snapshot, "openai-main")
		return ok && primaryUpstream.State == health.StateHealthy && primaryUpstream.Source == health.SourceProbe
	})

	primaryUpstream, ok := runtimeUpstreamByName(snapshot, "openai-main")
	if !ok {
		t.Fatal("missing runtime snapshot for openai-main")
	}
	if !primaryUpstream.Eligible {
		t.Fatal("primary eligible = false, want true")
	}
	if primaryUpstream.BreakerState != health.BreakerStateClosed {
		t.Fatalf("primary breaker_state = %q, want %q", primaryUpstream.BreakerState, health.BreakerStateClosed)
	}

	secondResp := postJSON(t, env.Client, env.BaseURL+"/v1/chat/completions", env.Token, chatTextRequest("openai/gpt-4.1", false))
	assertStatus(t, secondResp, 200)

	if primary.Hits() != 2 {
		t.Fatalf("primary hits = %d, want 2", primary.Hits())
	}
	if backup.Hits() != 1 {
		t.Fatalf("backup hits = %d, want 1", backup.Hits())
	}
}

func runtimeUpstreamByName(snapshot health.RuntimeSnapshot, name string) (health.UpstreamStatus, bool) {
	for _, upstream := range snapshot.Upstreams {
		if upstream.Name == name {
			return upstream, true
		}
	}

	return health.UpstreamStatus{}, false
}

func runtimeRecoveryConfig(primaryURL, backupURL string) config.Config {
	return config.Config{
		Server: config.ServerConfig{
			ListenAddr: "127.0.0.1:0",
		},
		Auth: config.AuthConfig{
			ClientKeys: []config.ClientKeyConfig{
				{
					ID:                   "local-dev",
					Secret:               "sk-nx-local-dev",
					Active:               true,
					AllowedModelPatterns: []string{"openai/gpt-*"},
					AllowStreaming:       true,
					AllowTools:           true,
					AllowVision:          true,
					AllowStructured:      true,
				},
			},
		},
		Models: []config.ModelConfig{
			{
				Pattern:    "openai/gpt-*",
				RouteGroup: "openai-family",
			},
		},
		Providers: []config.ProviderConfig{
			{
				Name:      "openai-main",
				Provider:  "openai",
				BaseURL:   primaryURL,
				APIKeyEnv: "OPENAI_API_KEY",
			},
			{
				Name:      "openai-backup",
				Provider:  "openai",
				BaseURL:   backupURL,
				APIKeyEnv: "OPENAI_API_KEY",
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
		Health: config.HealthConfig{
			ProbeInterval: "25ms",
			ProbeTimeout:  "1s",
		},
		Breaker: config.BreakerConfig{
			FailureThreshold:         1,
			RecoverySuccessThreshold: 1,
			OpenInterval:             "50ms",
		},
	}
}
