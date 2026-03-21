package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/nikkofu/nexus-router/internal/app"
	"github.com/nikkofu/nexus-router/internal/config"
)

func TestNewServerExposesLiveness(t *testing.T) {
	cfg := config.Config{
		Server: config.ServerConfig{ListenAddr: "127.0.0.1:0"},
	}

	srv, err := app.New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMainBinaryBuilds(t *testing.T) {
	out := t.TempDir() + "/nexus-router"
	cmd := exec.Command("go", "build", "-o", out, "./cmd/nexus-router")
	cmd.Dir = "../.."

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build error = %v\n%s", err, output)
	}
}

func TestServiceShutdownWithoutStartReturnsNil(t *testing.T) {
	cfg := config.Config{
		Server: config.ServerConfig{ListenAddr: "127.0.0.1:0"},
	}

	srv, err := app.New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestLoadConfigRejectsUnknownRouteGroup(t *testing.T) {
	_, err := config.Load(strings.NewReader(`
server:
  listen_addr: 127.0.0.1:8080
models:
  - pattern: openai/gpt-*
    route_group: missing
`))
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadConfigAcceptsExample(t *testing.T) {
	data, err := os.ReadFile("../../configs/nexus-router.example.yaml")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if _, err := config.Load(strings.NewReader(string(data))); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadConfigAppliesRuntimeHealthDefaultsWhenOmitted(t *testing.T) {
	cfg, err := config.Load(strings.NewReader(`
server:
  listen_addr: 127.0.0.1:8080
  tls:
    mode: disabled
auth:
  client_keys:
    - id: local-openai
      secret: sk-nx-openai
      active: true
      allowed_model_patterns:
        - openai/gpt-*
models:
  - pattern: openai/gpt-*
    route_group: openai-family
providers:
  - name: openai-main
    provider: openai
    base_url: https://api.openai.com
    api_key_env: OPENAI_API_KEY
routing:
  route_groups:
    - name: openai-family
      primary: openai-main
      fallbacks: []
`))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Health.ProbeInterval != "15s" {
		t.Fatalf("health.probe_interval = %q, want %q", cfg.Health.ProbeInterval, "15s")
	}
	if cfg.Health.ProbeTimeout != "2s" {
		t.Fatalf("health.probe_timeout = %q, want %q", cfg.Health.ProbeTimeout, "2s")
	}
	if !cfg.Health.RequireInitialProbe {
		t.Fatalf("health.require_initial_probe = %v, want %v", cfg.Health.RequireInitialProbe, true)
	}
	if cfg.Breaker.FailureThreshold != 3 {
		t.Fatalf("breaker.failure_threshold = %d, want %d", cfg.Breaker.FailureThreshold, 3)
	}
	if cfg.Breaker.OpenInterval != "30s" {
		t.Fatalf("breaker.open_interval = %q, want %q", cfg.Breaker.OpenInterval, "30s")
	}
	if cfg.Breaker.RecoverySuccessThreshold != 1 {
		t.Fatalf("breaker.recovery_success_threshold = %d, want %d", cfg.Breaker.RecoverySuccessThreshold, 1)
	}
}

func TestLoadConfigPreservesExplicitRuntimeHealthValues(t *testing.T) {
	cfg, err := config.Load(strings.NewReader(`
server:
  listen_addr: 127.0.0.1:8080
  tls:
    mode: disabled
auth:
  client_keys:
    - id: local-openai
      secret: sk-nx-openai
      active: true
      allowed_model_patterns:
        - openai/gpt-*
models:
  - pattern: openai/gpt-*
    route_group: openai-family
providers:
  - name: openai-main
    provider: openai
    base_url: https://api.openai.com
    api_key_env: OPENAI_API_KEY
routing:
  route_groups:
    - name: openai-family
      primary: openai-main
      fallbacks: []
health:
  probe_interval: 7s
  probe_timeout: 3s
  require_initial_probe: false
breaker:
  failure_threshold: 5
  open_interval: 45s
  recovery_success_threshold: 2
`))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Health.ProbeInterval != "7s" {
		t.Fatalf("health.probe_interval = %q, want %q", cfg.Health.ProbeInterval, "7s")
	}
	if cfg.Health.ProbeTimeout != "3s" {
		t.Fatalf("health.probe_timeout = %q, want %q", cfg.Health.ProbeTimeout, "3s")
	}
	if cfg.Health.RequireInitialProbe {
		t.Fatalf("health.require_initial_probe = %v, want %v", cfg.Health.RequireInitialProbe, false)
	}
	if cfg.Breaker.FailureThreshold != 5 {
		t.Fatalf("breaker.failure_threshold = %d, want %d", cfg.Breaker.FailureThreshold, 5)
	}
	if cfg.Breaker.OpenInterval != "45s" {
		t.Fatalf("breaker.open_interval = %q, want %q", cfg.Breaker.OpenInterval, "45s")
	}
	if cfg.Breaker.RecoverySuccessThreshold != 2 {
		t.Fatalf("breaker.recovery_success_threshold = %d, want %d", cfg.Breaker.RecoverySuccessThreshold, 2)
	}
}

func TestLoadConfigRejectsInvalidRuntimeHealthExplicitOverrides(t *testing.T) {
	base := `
server:
  listen_addr: 127.0.0.1:8080
  tls:
    mode: disabled
auth:
  client_keys:
    - id: local-openai
      secret: sk-nx-openai
      active: true
      allowed_model_patterns:
        - openai/gpt-*
models:
  - pattern: openai/gpt-*
    route_group: openai-family
providers:
  - name: openai-main
    provider: openai
    base_url: https://api.openai.com
    api_key_env: OPENAI_API_KEY
%s
routing:
  route_groups:
    - name: openai-family
      primary: openai-main
      fallbacks: []
%s
%s
`

	tests := []struct {
		name string
		probe string
		health string
		breaker string
	}{
		{
			name: "failure threshold zero",
			breaker: `
breaker:
  failure_threshold: 0
`,
		},
		{
			name: "recovery success threshold zero",
			breaker: `
breaker:
  recovery_success_threshold: 0
`,
		},
		{
			name: "health probe interval empty",
			health: `
health:
  probe_interval: ""
`,
		},
		{
			name: "breaker open interval empty",
			breaker: `
breaker:
  open_interval: ""
`,
		},
		{
			name: "health probe timeout empty",
			health: `
health:
  probe_timeout: ""
`,
		},
		{
			name: "provider probe interval empty",
			probe: `    probe:
      interval: ""
`,
		},
		{
			name: "provider probe timeout empty",
			probe: `    probe:
      timeout: ""
`,
		},
		{
			name: "health probe interval bare key",
			health: `
health:
  probe_interval:
`,
		},
		{
			name: "provider probe timeout null",
			probe: `    probe:
      timeout: null
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := fmt.Sprintf(base, tt.probe, tt.health, tt.breaker)
			_, err := config.Load(strings.NewReader(cfg))
			if err == nil {
				t.Fatal("expected Load() error")
			}
		})
	}
}
