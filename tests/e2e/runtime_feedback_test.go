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

	if env.Primary.Hits() != 1 {
		t.Fatalf("primary hits = %d, want 1", env.Primary.Hits())
	}
	if env.Backup == nil || env.Backup.Hits() != 1 {
		t.Fatalf("backup hits = %d, want 1", env.Backup.Hits())
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

func runtimeUpstreamByName(snapshot health.RuntimeSnapshot, name string) (health.UpstreamStatus, bool) {
	for _, upstream := range snapshot.Upstreams {
		if upstream.Name == name {
			return upstream, true
		}
	}

	return health.UpstreamStatus{}, false
}
