package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/nikkofu/nexus-router/internal/app"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/health"
)

func TestChatEndpointRejectsMissingBearerToken(t *testing.T) {
	resp := performRequest(t, testServiceConfig(), http.MethodPost, "/v1/chat/completions", []byte(`{}`), "")
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}

func TestAdminRoutesExposeRuntimeSummary(t *testing.T) {
	resp := performRequest(t, testServiceConfig(), http.MethodGet, "/admin/routes", nil, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
}

func TestReadyzReturns503WhenARequiredRouteGroupHasNoEligibleUpstream(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-test-key")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-test-key")

	openAIProbe := newIntegrationProbeServer(t, http.StatusOK)
	defer openAIProbe.Close()
	anthropicProbe := newIntegrationProbeServer(t, http.StatusInternalServerError)
	defer anthropicProbe.Close()

	cfg := testServiceConfig()
	cfg.Models = []config.ModelConfig{
		{
			Pattern:    "openai/gpt-*",
			RouteGroup: "openai-family",
		},
		{
			Pattern:    "anthropic/claude-*",
			RouteGroup: "anthropic-family",
		},
	}
	cfg.Providers = []config.ProviderConfig{
		{
			Name:      "openai-main",
			Provider:  "openai",
			BaseURL:   openAIProbe.URL,
			APIKeyEnv: "OPENAI_API_KEY",
		},
		{
			Name:      "anthropic-main",
			Provider:  "anthropic",
			BaseURL:   anthropicProbe.URL,
			APIKeyEnv: "ANTHROPIC_API_KEY",
		},
	}
	cfg.Routing.RouteGroups = []config.RouteGroupConfig{
		{
			Name:    "openai-family",
			Primary: "openai-main",
		},
		{
			Name:    "anthropic-family",
			Primary: "anthropic-main",
		},
	}
	cfg.Health.RequireInitialProbe = true

	srv := newTestService(t, cfg)
	defer shutdownTestService(t, srv)

	waitForIntegrationRuntimeSnapshot(t, srv, func(snapshot health.RuntimeSnapshot) bool {
		if !snapshot.Started || !snapshot.InitialProbeComplete {
			return false
		}

		openAI, ok := integrationUpstreamByName(snapshot, "openai-main")
		if !ok || !openAI.Eligible {
			return false
		}

		anthropic, ok := integrationUpstreamByName(snapshot, "anthropic-main")
		if !ok {
			return false
		}

		return !anthropic.Eligible
	})

	resp := performServiceRequest(t, srv, http.MethodGet, "/readyz", nil, "")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusServiceUnavailable)
	}
}

func TestAdminUpstreamsReturnsRuntimeSnapshotEnvelope(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-test-key")

	probe := newIntegrationProbeServer(t, http.StatusOK)
	defer probe.Close()

	cfg := testServiceConfig()
	cfg.Providers = []config.ProviderConfig{
		{
			Name:      "openai-main",
			Provider:  "openai",
			BaseURL:   probe.URL,
			APIKeyEnv: "OPENAI_API_KEY",
		},
	}
	cfg.Health.ProbeInterval = "50ms"
	cfg.Health.ProbeTimeout = "1s"
	cfg.Health.RequireInitialProbe = true

	srv := newTestService(t, cfg)
	defer shutdownTestService(t, srv)

	waitForIntegrationStatus(t, srv, "/readyz", http.StatusOK)

	resp := performServiceRequest(t, srv, http.MethodGet, "/admin/upstreams", nil, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}

	payload := decodeExactRuntimeSnapshot(t, resp.Body.Bytes())

	if !payload.Started {
		t.Fatal("started = false, want true")
	}
	if !payload.InitialProbeComplete {
		t.Fatal("initial_probe_complete = false, want true")
	}
	if !payload.HasEligibleUpstream {
		t.Fatal("has_eligible_upstream = false, want true")
	}
	if len(payload.Upstreams) != 1 {
		t.Fatalf("len(upstreams) = %d, want 1", len(payload.Upstreams))
	}

	upstream := payload.Upstreams[0]
	if upstream.Name != "openai-main" {
		t.Fatalf("name = %q, want %q", upstream.Name, "openai-main")
	}
	if upstream.Provider != "openai" {
		t.Fatalf("provider = %q, want %q", upstream.Provider, "openai")
	}
	if upstream.State != health.StateHealthy {
		t.Fatalf("state = %q, want %q", upstream.State, health.StateHealthy)
	}
	if !upstream.Eligible {
		t.Fatal("eligible = false, want true")
	}
	if upstream.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive_failures = %d, want 0", upstream.ConsecutiveFailures)
	}
	if !upstream.EjectedUntil.IsZero() {
		t.Fatalf("ejected_until = %v, want zero time", upstream.EjectedUntil)
	}
	if upstream.LastProbeAt.IsZero() {
		t.Fatal("last_probe_at = zero, want non-zero")
	}
	if !upstream.LastProbeOK {
		t.Fatal("last_probe_ok = false, want true")
	}
	if upstream.LastError != "" {
		t.Fatalf("last_error = %q, want empty string", upstream.LastError)
	}
	if upstream.BreakerState != health.BreakerStateClosed {
		t.Fatalf("breaker_state = %q, want %q", upstream.BreakerState, health.BreakerStateClosed)
	}
	if upstream.Source != health.SourceProbe {
		t.Fatalf("source = %q, want %q", upstream.Source, health.SourceProbe)
	}
}

func performRequest(t *testing.T, cfg config.Config, method, path string, body []byte, bearer string) *httptest.ResponseRecorder {
	t.Helper()

	srv := newTestService(t, cfg)
	defer shutdownTestService(t, srv)

	return performServiceRequest(t, srv, method, path, body, bearer)
}

func performServiceRequest(t *testing.T, srv *app.Service, method, path string, body []byte, bearer string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func newTestService(t *testing.T, cfg config.Config) *app.Service {
	t.Helper()

	srv, err := app.New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return srv
}

func shutdownTestService(t *testing.T, srv *app.Service) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func newIntegrationProbeServer(t *testing.T, status int) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
	}))
}

func waitForIntegrationStatus(t *testing.T, srv *app.Service, path string, want int) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp := performServiceRequest(t, srv, http.MethodGet, path, nil, "")
		if resp.Code == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	resp := performServiceRequest(t, srv, http.MethodGet, path, nil, "")
	t.Fatalf("status for %s = %d, want %d, body = %q", path, resp.Code, want, resp.Body.String())
}

func waitForIntegrationRuntimeSnapshot(t *testing.T, srv *app.Service, predicate func(health.RuntimeSnapshot) bool) health.RuntimeSnapshot {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := getIntegrationRuntimeSnapshot(t, srv)
		if predicate(snapshot) {
			return snapshot
		}
		time.Sleep(25 * time.Millisecond)
	}

	snapshot := getIntegrationRuntimeSnapshot(t, srv)
	t.Fatalf("runtime snapshot did not satisfy predicate: %+v", snapshot)
	return health.RuntimeSnapshot{}
}

func getIntegrationRuntimeSnapshot(t *testing.T, srv *app.Service) health.RuntimeSnapshot {
	t.Helper()

	resp := performServiceRequest(t, srv, http.MethodGet, "/admin/upstreams", nil, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %q", resp.Code, http.StatusOK, resp.Body.String())
	}

	return decodeExactRuntimeSnapshot(t, resp.Body.Bytes())
}

func decodeExactRuntimeSnapshot(t *testing.T, body []byte) health.RuntimeSnapshot {
	t.Helper()

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	assertExactJSONKeys(t, envelope, []string{
		"started",
		"initial_probe_complete",
		"has_eligible_upstream",
		"upstreams",
	})

	var upstreams []json.RawMessage
	if err := json.Unmarshal(envelope["upstreams"], &upstreams); err != nil {
		t.Fatalf("json.Unmarshal(upstreams) error = %v", err)
	}
	for _, upstream := range upstreams {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(upstream, &fields); err != nil {
			t.Fatalf("json.Unmarshal(upstream) error = %v", err)
		}
		assertExactJSONKeys(t, fields, []string{
			"name",
			"provider",
			"state",
			"eligible",
			"consecutive_failures",
			"ejected_until",
			"last_probe_at",
			"last_probe_ok",
			"last_error",
			"breaker_state",
			"source",
		})
	}

	var snapshot health.RuntimeSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatalf("json.Unmarshal(snapshot) error = %v", err)
	}

	return snapshot
}

func assertExactJSONKeys(t *testing.T, got map[string]json.RawMessage, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("json keys = %v, want %v", sortedJSONKeys(got), want)
	}

	for _, key := range want {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing json key %q in %v", key, sortedJSONKeys(got))
		}
	}

	wantSet := make(map[string]struct{}, len(want))
	for _, key := range want {
		wantSet[key] = struct{}{}
	}
	for key := range got {
		if _, ok := wantSet[key]; ok {
			continue
		}
		t.Fatalf("unexpected json key %q in %v", key, sortedJSONKeys(got))
	}
}

func sortedJSONKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func integrationUpstreamByName(snapshot health.RuntimeSnapshot, name string) (health.UpstreamStatus, bool) {
	for _, upstream := range snapshot.Upstreams {
		if upstream.Name == name {
			return upstream, true
		}
	}

	return health.UpstreamStatus{}, false
}

func testServiceConfig() config.Config {
	return config.Config{
		Server: config.ServerConfig{
			ListenAddr:      "127.0.0.1:0",
			AdminListenAddr: "127.0.0.1:9090",
		},
		Auth: config.AuthConfig{
			ClientKeys: []config.ClientKeyConfig{
				{
					ID:                   "local-dev",
					Secret:               "sk-nx-local-dev",
					Active:               true,
					AllowedModelPatterns: []string{"openai/gpt-*"},
					AllowStreaming:       true,
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
				BaseURL:   "https://api.openai.com",
				APIKeyEnv: "OPENAI_API_KEY",
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
	}
}
