package integration

import (
	"context"
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
