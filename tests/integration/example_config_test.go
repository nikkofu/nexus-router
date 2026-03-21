package integration

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/nikkofu/nexus-router/internal/app"
	"github.com/nikkofu/nexus-router/internal/config"
)

func TestExampleConfigBoots(t *testing.T) {
	data, err := os.ReadFile("../../configs/nexus-router.example.yaml")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	cfg, err := config.Load(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Health.ProbeInterval != "5s" {
		t.Fatalf("health.probe_interval = %q, want %q", cfg.Health.ProbeInterval, "5s")
	}
	if cfg.Health.ProbeTimeout != "1s" {
		t.Fatalf("health.probe_timeout = %q, want %q", cfg.Health.ProbeTimeout, "1s")
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
