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

func TestAdminConfigReturnsRedactedConfigSummary(t *testing.T) {
	cfg := testServiceConfig()
	cfg.Server.AdminListenAddr = "127.0.0.1:19090"
	cfg.Server.TLS.Mode = "disabled"
	cfg.Health.ProbeInterval = "7s"
	cfg.Health.ProbeTimeout = "3s"
	cfg.Breaker.OpenInterval = "45s"
	cfg.Limits.MaxRequestBytes = 2048
	cfg.Auth.ClientKeys[0].AllowTools = true
	cfg.Auth.ClientKeys[0].AllowVision = true
	cfg.Auth.ClientKeys[0].AllowStructured = true
	cfg.Providers = []config.ProviderConfig{
		{
			Name:      "openai-main",
			Provider:  "openai",
			BaseURL:   "https://api.openai.example",
			APIKeyEnv: "OPENAI_API_KEY",
			Probe: config.ProbeConfig{
				Method:           http.MethodHead,
				Path:             "/healthz",
				Headers:          map[string]string{"x-custom": "secretish"},
				ExpectedStatuses: []int{200, 204},
				Interval:         "11s",
				Timeout:          "4s",
			},
		},
	}

	resp := performRequest(t, cfg, http.MethodGet, "/admin/config", nil, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %q", resp.Code, http.StatusOK, resp.Body.String())
	}

	payload := decodeExactAdminConfigSummary(t, resp.Body.Bytes())

	if payload.Server.ListenAddr != "127.0.0.1:0" {
		t.Fatalf("server.listen_addr = %q, want %q", payload.Server.ListenAddr, "127.0.0.1:0")
	}
	if payload.Server.AdminListenAddr != "127.0.0.1:19090" {
		t.Fatalf("server.admin_listen_addr = %q, want %q", payload.Server.AdminListenAddr, "127.0.0.1:19090")
	}
	if payload.Server.TLS.Mode != "disabled" {
		t.Fatalf("server.tls.mode = %q, want %q", payload.Server.TLS.Mode, "disabled")
	}
	if len(payload.Auth.ClientKeys) != 1 {
		t.Fatalf("len(auth.client_keys) = %d, want 1", len(payload.Auth.ClientKeys))
	}
	if payload.Auth.ClientKeys[0].ID != "local-dev" {
		t.Fatalf("auth.client_keys[0].id = %q, want %q", payload.Auth.ClientKeys[0].ID, "local-dev")
	}
	if !payload.Auth.ClientKeys[0].AllowStructured {
		t.Fatal("auth.client_keys[0].allow_structured = false, want true")
	}
	if len(payload.Providers) != 1 {
		t.Fatalf("len(providers) = %d, want 1", len(payload.Providers))
	}

	provider := payload.Providers[0]
	if provider.Name != "openai-main" {
		t.Fatalf("providers[0].name = %q, want %q", provider.Name, "openai-main")
	}
	if !provider.HasAPIKeyEnv {
		t.Fatal("providers[0].has_api_key_env = false, want true")
	}
	if !provider.Probe.HasHeaders {
		t.Fatal("providers[0].probe.has_headers = false, want true")
	}
	if provider.Probe.Method != http.MethodHead {
		t.Fatalf("providers[0].probe.method = %q, want %q", provider.Probe.Method, http.MethodHead)
	}
	if provider.Probe.Path != "/healthz" {
		t.Fatalf("providers[0].probe.path = %q, want %q", provider.Probe.Path, "/healthz")
	}
	if provider.Probe.Interval != "11s" {
		t.Fatalf("providers[0].probe.interval = %q, want %q", provider.Probe.Interval, "11s")
	}
	if provider.Probe.Timeout != "4s" {
		t.Fatalf("providers[0].probe.timeout = %q, want %q", provider.Probe.Timeout, "4s")
	}
	if len(provider.Probe.ExpectedStatuses) != 2 || provider.Probe.ExpectedStatuses[0] != 200 || provider.Probe.ExpectedStatuses[1] != 204 {
		t.Fatalf("providers[0].probe.expected_statuses = %v, want [200 204]", provider.Probe.ExpectedStatuses)
	}
	if payload.Breaker.OpenInterval != "45s" {
		t.Fatalf("breaker.open_interval = %q, want %q", payload.Breaker.OpenInterval, "45s")
	}
	if payload.Limits.MaxRequestBytes != 2048 {
		t.Fatalf("limits.max_request_bytes = %d, want %d", payload.Limits.MaxRequestBytes, 2048)
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

type adminConfigSummaryPayload struct {
	Server struct {
		ListenAddr      string `json:"listen_addr"`
		AdminListenAddr string `json:"admin_listen_addr"`
		TLS             struct {
			Mode string `json:"mode"`
		} `json:"tls"`
	} `json:"server"`
	Auth struct {
		ClientKeys []struct {
			ID                   string   `json:"id"`
			Active               bool     `json:"active"`
			AllowedModelPatterns []string `json:"allowed_model_patterns"`
			AllowStreaming       bool     `json:"allow_streaming"`
			AllowTools           bool     `json:"allow_tools"`
			AllowVision          bool     `json:"allow_vision"`
			AllowStructured      bool     `json:"allow_structured"`
		} `json:"client_keys"`
	} `json:"auth"`
	Models []struct {
		Pattern    string `json:"pattern"`
		RouteGroup string `json:"route_group"`
	} `json:"models"`
	Providers []struct {
		Name         string `json:"name"`
		Provider     string `json:"provider"`
		BaseURL      string `json:"base_url"`
		HasAPIKeyEnv bool   `json:"has_api_key_env"`
		Probe        struct {
			Method           string `json:"method"`
			Path             string `json:"path"`
			HasHeaders       bool   `json:"has_headers"`
			ExpectedStatuses []int  `json:"expected_statuses"`
			Interval         string `json:"interval"`
			Timeout          string `json:"timeout"`
		} `json:"probe"`
	} `json:"providers"`
	Routing struct {
		RouteGroups []struct {
			Name      string   `json:"name"`
			Primary   string   `json:"primary"`
			Fallbacks []string `json:"fallbacks"`
		} `json:"route_groups"`
	} `json:"routing"`
	Health struct {
		ProbeInterval       string `json:"probe_interval"`
		ProbeTimeout        string `json:"probe_timeout"`
		RequireInitialProbe bool   `json:"require_initial_probe"`
	} `json:"health"`
	Breaker struct {
		FailureThreshold         int    `json:"failure_threshold"`
		RecoverySuccessThreshold int    `json:"recovery_success_threshold"`
		OpenInterval             string `json:"open_interval"`
	} `json:"breaker"`
	Limits struct {
		MaxRequestBytes int64 `json:"max_request_bytes"`
	} `json:"limits"`
}

func decodeExactAdminConfigSummary(t *testing.T, body []byte) adminConfigSummaryPayload {
	t.Helper()

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	assertExactJSONKeys(t, envelope, []string{
		"server",
		"auth",
		"models",
		"providers",
		"routing",
		"health",
		"breaker",
		"limits",
	})

	var serverFields map[string]json.RawMessage
	if err := json.Unmarshal(envelope["server"], &serverFields); err != nil {
		t.Fatalf("json.Unmarshal(server) error = %v", err)
	}
	assertExactJSONKeys(t, serverFields, []string{"listen_addr", "admin_listen_addr", "tls"})

	var tlsFields map[string]json.RawMessage
	if err := json.Unmarshal(serverFields["tls"], &tlsFields); err != nil {
		t.Fatalf("json.Unmarshal(server.tls) error = %v", err)
	}
	assertExactJSONKeys(t, tlsFields, []string{"mode"})

	var authFields map[string]json.RawMessage
	if err := json.Unmarshal(envelope["auth"], &authFields); err != nil {
		t.Fatalf("json.Unmarshal(auth) error = %v", err)
	}
	assertExactJSONKeys(t, authFields, []string{"client_keys"})

	var clientKeys []json.RawMessage
	if err := json.Unmarshal(authFields["client_keys"], &clientKeys); err != nil {
		t.Fatalf("json.Unmarshal(auth.client_keys) error = %v", err)
	}
	for _, key := range clientKeys {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(key, &fields); err != nil {
			t.Fatalf("json.Unmarshal(auth.client_keys[*]) error = %v", err)
		}
		assertExactJSONKeys(t, fields, []string{
			"id",
			"active",
			"allowed_model_patterns",
			"allow_streaming",
			"allow_tools",
			"allow_vision",
			"allow_structured",
		})
	}

	var models []json.RawMessage
	if err := json.Unmarshal(envelope["models"], &models); err != nil {
		t.Fatalf("json.Unmarshal(models) error = %v", err)
	}
	for _, model := range models {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(model, &fields); err != nil {
			t.Fatalf("json.Unmarshal(models[*]) error = %v", err)
		}
		assertExactJSONKeys(t, fields, []string{"pattern", "route_group"})
	}

	var providers []json.RawMessage
	if err := json.Unmarshal(envelope["providers"], &providers); err != nil {
		t.Fatalf("json.Unmarshal(providers) error = %v", err)
	}
	for _, provider := range providers {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(provider, &fields); err != nil {
			t.Fatalf("json.Unmarshal(providers[*]) error = %v", err)
		}
		assertExactJSONKeys(t, fields, []string{"name", "provider", "base_url", "has_api_key_env", "probe"})

		var probeFields map[string]json.RawMessage
		if err := json.Unmarshal(fields["probe"], &probeFields); err != nil {
			t.Fatalf("json.Unmarshal(providers[*].probe) error = %v", err)
		}
		assertExactJSONKeys(t, probeFields, []string{"method", "path", "has_headers", "expected_statuses", "interval", "timeout"})
	}

	var routingFields map[string]json.RawMessage
	if err := json.Unmarshal(envelope["routing"], &routingFields); err != nil {
		t.Fatalf("json.Unmarshal(routing) error = %v", err)
	}
	assertExactJSONKeys(t, routingFields, []string{"route_groups"})

	var routeGroups []json.RawMessage
	if err := json.Unmarshal(routingFields["route_groups"], &routeGroups); err != nil {
		t.Fatalf("json.Unmarshal(routing.route_groups) error = %v", err)
	}
	for _, group := range routeGroups {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(group, &fields); err != nil {
			t.Fatalf("json.Unmarshal(routing.route_groups[*]) error = %v", err)
		}
		assertExactJSONKeys(t, fields, []string{"name", "primary", "fallbacks"})
	}

	var healthFields map[string]json.RawMessage
	if err := json.Unmarshal(envelope["health"], &healthFields); err != nil {
		t.Fatalf("json.Unmarshal(health) error = %v", err)
	}
	assertExactJSONKeys(t, healthFields, []string{"probe_interval", "probe_timeout", "require_initial_probe"})

	var breakerFields map[string]json.RawMessage
	if err := json.Unmarshal(envelope["breaker"], &breakerFields); err != nil {
		t.Fatalf("json.Unmarshal(breaker) error = %v", err)
	}
	assertExactJSONKeys(t, breakerFields, []string{"failure_threshold", "recovery_success_threshold", "open_interval"})

	var limitsFields map[string]json.RawMessage
	if err := json.Unmarshal(envelope["limits"], &limitsFields); err != nil {
		t.Fatalf("json.Unmarshal(limits) error = %v", err)
	}
	assertExactJSONKeys(t, limitsFields, []string{"max_request_bytes"})

	var payload adminConfigSummaryPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal(payload) error = %v", err)
	}

	return payload
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
