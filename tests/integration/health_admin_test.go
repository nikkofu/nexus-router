package integration

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nikkofu/nexus-router/internal/app"
	"github.com/nikkofu/nexus-router/internal/config"
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

func TestReadyzFailsWithoutEligibleUpstream(t *testing.T) {
	cfg := testServiceConfig()
	cfg.Providers = nil

	resp := performRequest(t, cfg, http.MethodGet, "/readyz", nil, "")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusServiceUnavailable)
	}
}

func performRequest(t *testing.T, cfg config.Config, method, path string, body []byte, bearer string) *httptest.ResponseRecorder {
	t.Helper()

	srv, err := app.New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
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
