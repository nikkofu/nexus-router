package integration

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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

func TestMainBinaryLoadsConfigFileAndServesConfiguredAddress(t *testing.T) {
	port := reserveTCPPort(t)
	cfgPath := writeMainConfigFixture(t, port, reserveTCPPort(t))
	binPath := filepath.Join(t.TempDir(), "nexus-router")

	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/nexus-router")
	buildCmd.Dir = "../.."
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build error = %v\n%s", err, buildOutput)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "-config", cfgPath)
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "OPENAI_API_KEY=openai-test-key")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("StderrPipe() error = %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	waitForHTTPStatus(t, "http://127.0.0.1:"+port+"/livez", http.StatusOK, stderr)
}

func TestMainBinaryServesAdminEndpointsOnAdminListener(t *testing.T) {
	publicPort := reserveTCPPort(t)
	adminPort := reserveTCPPort(t)
	cfgPath := writeMainConfigFixture(t, publicPort, adminPort)
	binPath := filepath.Join(t.TempDir(), "nexus-router")

	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/nexus-router")
	buildCmd.Dir = "../.."
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build error = %v\n%s", err, buildOutput)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "-config", cfgPath)
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "OPENAI_API_KEY=openai-test-key")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("StderrPipe() error = %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	waitForHTTPStatus(t, "http://127.0.0.1:"+publicPort+"/livez", http.StatusOK, stderr)
	waitForHTTPStatus(t, "http://127.0.0.1:"+adminPort+"/admin/config", http.StatusOK, stderr)
	assertHTTPStatus(t, "http://127.0.0.1:"+publicPort+"/admin/config", http.StatusNotFound)
}

func TestMainBinaryGracefullyShutsDownOnInterrupt(t *testing.T) {
	publicPort := reserveTCPPort(t)
	adminPort := reserveTCPPort(t)
	cfgPath := writeMainConfigFixture(t, publicPort, adminPort)
	binPath := filepath.Join(t.TempDir(), "nexus-router")

	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/nexus-router")
	buildCmd.Dir = "../.."
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build error = %v\n%s", err, buildOutput)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "-config", cfgPath)
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "OPENAI_API_KEY=openai-test-key")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("StderrPipe() error = %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	waitForHTTPStatus(t, "http://127.0.0.1:"+publicPort+"/livez", http.StatusOK, stderr)

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("Signal() error = %v", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	select {
	case err := <-waitCh:
		if err != nil {
			t.Fatalf("Wait() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("process did not exit after interrupt")
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

func reserveTCPPort(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort() error = %v", err)
	}

	return port
}

func writeMainConfigFixture(t *testing.T, publicPort, adminPort string) string {
	t.Helper()

	cfgPath := filepath.Join(t.TempDir(), "nexus-router.yaml")
	cfg := fmt.Sprintf(`
server:
  listen_addr: 127.0.0.1:%s
  admin_listen_addr: 127.0.0.1:%s
  tls:
    mode: disabled
auth:
  client_keys:
    - id: local-dev
      secret: sk-nx-local-dev
      active: true
      allowed_model_patterns:
        - openai/gpt-*
      allow_streaming: true
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
health:
  probe_interval: 1h
  probe_timeout: 1s
  require_initial_probe: false
breaker:
  failure_threshold: 3
  recovery_success_threshold: 1
  open_interval: 30s
limits:
  max_request_bytes: 1048576
`, publicPort, adminPort)

	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return cfgPath
}

func waitForHTTPStatus(t *testing.T, url string, want int, stderr io.Reader) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	client := &http.Client{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				t.Fatalf("ReadAll() error = %v", readErr)
			}
			if resp.StatusCode == want {
				return
			}
			_ = body
		}
		time.Sleep(25 * time.Millisecond)
	}

	data, _ := io.ReadAll(stderr)
	t.Fatalf("timed out waiting for %s = %d; stderr=%s", url, want, data)
}

func assertHTTPStatus(t *testing.T, url string, want int) {
	t.Helper()

	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("Get(%q) error = %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status for %s = %d, want %d, body = %q", url, resp.StatusCode, want, body)
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
		name    string
		probe   string
		health  string
		breaker string
		wantErr string
	}{
		{
			name: "failure threshold zero",
			breaker: `
breaker:
  failure_threshold: 0
`,
			wantErr: "breaker.failure_threshold",
		},
		{
			name: "recovery success threshold zero",
			breaker: `
breaker:
  recovery_success_threshold: 0
`,
			wantErr: "breaker.recovery_success_threshold",
		},
		{
			name: "health probe interval empty",
			health: `
health:
  probe_interval: ""
`,
			wantErr: "health.probe_interval",
		},
		{
			name: "breaker open interval empty",
			breaker: `
breaker:
  open_interval: ""
`,
			wantErr: "breaker.open_interval",
		},
		{
			name: "health probe timeout empty",
			health: `
health:
  probe_timeout: ""
`,
			wantErr: "health.probe_timeout",
		},
		{
			name: "provider probe interval empty",
			probe: `    probe:
      interval: ""
`,
			wantErr: "providers[0].probe.interval",
		},
		{
			name: "provider probe timeout empty",
			probe: `    probe:
      timeout: ""
`,
			wantErr: "providers[0].probe.timeout",
		},
		{
			name: "health probe interval bare key",
			health: `
health:
  probe_interval:
`,
			wantErr: "health.probe_interval",
		},
		{
			name: "provider probe timeout null",
			probe: `    probe:
      timeout: null
`,
			wantErr: "providers[0].probe.timeout",
		},
		{
			name: "health section bare key",
			health: `
health:
`,
			wantErr: "health",
		},
		{
			name: "health section null",
			health: `
health: null
`,
			wantErr: "health",
		},
		{
			name: "breaker section bare key",
			breaker: `
breaker:
`,
			wantErr: "breaker",
		},
		{
			name: "breaker section null",
			breaker: `
breaker: null
`,
			wantErr: "breaker",
		},
		{
			name: "health require initial probe null",
			health: `
health:
  require_initial_probe: null
`,
			wantErr: "health.require_initial_probe",
		},
		{
			name: "health require initial probe bare key",
			health: `
health:
  require_initial_probe:
`,
			wantErr: "health.require_initial_probe",
		},
		{
			name: "breaker failure threshold null",
			breaker: `
breaker:
  failure_threshold: null
`,
			wantErr: "breaker.failure_threshold",
		},
		{
			name: "breaker failure threshold bare key",
			breaker: `
breaker:
  failure_threshold:
`,
			wantErr: "breaker.failure_threshold",
		},
		{
			name: "breaker recovery success threshold null",
			breaker: `
breaker:
  recovery_success_threshold: null
`,
			wantErr: "breaker.recovery_success_threshold",
		},
		{
			name: "breaker recovery success threshold bare key",
			breaker: `
breaker:
  recovery_success_threshold:
`,
			wantErr: "breaker.recovery_success_threshold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := fmt.Sprintf(base, tt.probe, tt.health, tt.breaker)
			_, err := config.Load(strings.NewReader(cfg))
			if err == nil {
				t.Fatal("expected Load() error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
