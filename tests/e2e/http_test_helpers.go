package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nikkofu/nexus-router/internal/app"
	"github.com/nikkofu/nexus-router/internal/auth"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/health"
)

type requestCapture struct {
	mu   sync.RWMutex
	path string
}

func (c *requestCapture) setPath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.path = path
}

func (c *requestCapture) Path() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.path
}

func newProviderDispatchStubServer(t *testing.T) (*httptest.Server, *requestCapture) {
	t.Helper()

	capture := &requestCapture{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.setPath(r.URL.Path)

		switch r.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		case "/v1/messages":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"delta\":{\"text\":\"hello\"}}\n\n")
			_, _ = fmt.Fprint(w, "event: message_stop\ndata: {}\n\n")
		default:
			http.Error(w, "unknown path", http.StatusNotFound)
		}
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	server.Start()

	return server, capture
}

func newTestResolver(keys ...config.ClientKeyConfig) auth.Resolver {
	return auth.NewResolver(keys)
}

type providerCapture struct {
	mu      sync.RWMutex
	hits    int
	path    string
	body    string
	headers http.Header
}

func (c *providerCapture) record(r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	c.mu.Lock()
	defer c.mu.Unlock()
	if r.URL.Path == "/v1/models" {
		return
	}
	c.hits++
	c.path = r.URL.Path
	c.body = string(body)
	c.headers = r.Header.Clone()
}

func (c *providerCapture) Hits() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits
}

func (c *providerCapture) Path() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.path
}

func (c *providerCapture) Body() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.body
}

func (c *providerCapture) Header(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.headers == nil {
		return ""
	}
	return c.headers.Get(key)
}

type httpTestEnv struct {
	BaseURL string
	Client  *http.Client
	Token   string
	Primary *providerCapture
	Backup  *providerCapture
	Service *app.Service
	closeFn func()
}

func (e httpTestEnv) Close() {
	if e.closeFn != nil {
		e.closeFn()
	}
}

func startHTTPTestEnv(t *testing.T, scenario string) httpTestEnv {
	t.Helper()
	return startHTTPTestEnvWithConfigMutator(t, scenario, nil)
}

func startHTTPTestEnvWithConfigMutator(t *testing.T, scenario string, mutate func(*config.Config)) httpTestEnv {
	t.Helper()

	const token = "sk-nx-local-dev"

	var (
		publicModel string
		providers   []config.ProviderConfig
		routeGroup  config.RouteGroupConfig
		primarySrv  *httptest.Server
		backupSrv   *httptest.Server
		primaryCap  *providerCapture
		backupCap   *providerCapture
	)

	switch scenario {
	case "openai_text":
		publicModel = "openai/gpt-4.1"
		primarySrv, primaryCap = newHTTPProviderStub(t, "openai_text")
		providers = []config.ProviderConfig{{
			Name:     "openai-main",
			Provider: "openai",
			BaseURL:  primarySrv.URL,
		}}
		routeGroup = config.RouteGroupConfig{
			Name:    "openai-family",
			Primary: "openai-main",
		}
	case "openai_text_usage":
		publicModel = "openai/gpt-4.1"
		primarySrv, primaryCap = newHTTPProviderStub(t, "openai_text_usage")
		providers = []config.ProviderConfig{{
			Name:     "openai-main",
			Provider: "openai",
			BaseURL:  primarySrv.URL,
		}}
		routeGroup = config.RouteGroupConfig{
			Name:    "openai-family",
			Primary: "openai-main",
		}
	case "openai_text_length":
		publicModel = "openai/gpt-4.1"
		primarySrv, primaryCap = newHTTPProviderStub(t, "openai_text_length")
		providers = []config.ProviderConfig{{
			Name:     "openai-main",
			Provider: "openai",
			BaseURL:  primarySrv.URL,
		}}
		routeGroup = config.RouteGroupConfig{
			Name:    "openai-family",
			Primary: "openai-main",
		}
	case "openai_responses":
		publicModel = "openai/gpt-4.1"
		primarySrv, primaryCap = newHTTPProviderStub(t, "openai_responses")
		providers = []config.ProviderConfig{{
			Name:     "openai-main",
			Provider: "openai",
			BaseURL:  primarySrv.URL,
		}}
		routeGroup = config.RouteGroupConfig{
			Name:    "openai-family",
			Primary: "openai-main",
		}
	case "openai_responses_usage":
		publicModel = "openai/gpt-4.1"
		primarySrv, primaryCap = newHTTPProviderStub(t, "openai_responses_usage")
		providers = []config.ProviderConfig{{
			Name:     "openai-main",
			Provider: "openai",
			BaseURL:  primarySrv.URL,
		}}
		routeGroup = config.RouteGroupConfig{
			Name:    "openai-family",
			Primary: "openai-main",
		}
	case "anthropic_text":
		publicModel = "anthropic/claude-sonnet-4-5"
		primarySrv, primaryCap = newHTTPProviderStub(t, "anthropic_text")
		providers = []config.ProviderConfig{{
			Name:     "anthropic-main",
			Provider: "anthropic",
			BaseURL:  primarySrv.URL,
		}}
		routeGroup = config.RouteGroupConfig{
			Name:    "anthropic-family",
			Primary: "anthropic-main",
		}
	case "anthropic_text_usage":
		publicModel = "anthropic/claude-sonnet-4-5"
		primarySrv, primaryCap = newHTTPProviderStub(t, "anthropic_text_usage")
		providers = []config.ProviderConfig{{
			Name:     "anthropic-main",
			Provider: "anthropic",
			BaseURL:  primarySrv.URL,
		}}
		routeGroup = config.RouteGroupConfig{
			Name:    "anthropic-family",
			Primary: "anthropic-main",
		}
	case "primary_429_backup_success":
		publicModel = "openai/gpt-4.1"
		primarySrv, primaryCap = newHTTPProviderStub(t, "rate_limit")
		backupSrv, backupCap = newHTTPProviderStub(t, "openai_text")
		providers = []config.ProviderConfig{
			{
				Name:     "openai-main",
				Provider: "openai",
				BaseURL:  primarySrv.URL,
			},
			{
				Name:     "openai-backup",
				Provider: "openai",
				BaseURL:  backupSrv.URL,
			},
		}
		routeGroup = config.RouteGroupConfig{
			Name:      "openai-family",
			Primary:   "openai-main",
			Fallbacks: []string{"openai-backup"},
		}
	case "partial_stream_interrupt_after_output":
		publicModel = "openai/gpt-4.1"
		primarySrv, primaryCap = newHTTPProviderStub(t, "partial_stream_interrupt_after_output")
		backupSrv, backupCap = newHTTPProviderStub(t, "openai_text")
		providers = []config.ProviderConfig{
			{
				Name:     "openai-main",
				Provider: "openai",
				BaseURL:  primarySrv.URL,
			},
			{
				Name:     "openai-backup",
				Provider: "openai",
				BaseURL:  backupSrv.URL,
			},
		}
		routeGroup = config.RouteGroupConfig{
			Name:      "openai-family",
			Primary:   "openai-main",
			Fallbacks: []string{"openai-backup"},
		}
	default:
		t.Fatalf("unknown HTTP test scenario %q", scenario)
	}

	for i := range providers {
		applyTestProviderAuthEnv(t, &providers[i])
	}

	cfg := config.Config{
		Server: config.ServerConfig{
			ListenAddr: "127.0.0.1:0",
		},
		Auth: config.AuthConfig{
			ClientKeys: []config.ClientKeyConfig{
				{
					ID:                   "local-dev",
					Secret:               token,
					Active:               true,
					AllowedModelPatterns: []string{publicModel},
					AllowStreaming:       true,
					AllowTools:           true,
					AllowVision:          true,
					AllowStructured:      true,
				},
			},
		},
		Models: []config.ModelConfig{
			{
				Pattern:    publicModel,
				RouteGroup: routeGroup.Name,
			},
		},
		Providers: providers,
		Routing: config.RoutingConfig{
			RouteGroups: []config.RouteGroupConfig{routeGroup},
		},
	}
	if mutate != nil {
		mutate(&cfg)
	}

	svc, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New() error = %v", err)
	}

	publicSrv := newHTTPServer(t, svc.Handler())

	return httpTestEnv{
		BaseURL: publicSrv.URL,
		Client:  publicSrv.Client(),
		Token:   token,
		Primary: primaryCap,
		Backup:  backupCap,
		Service: svc,
		closeFn: func() {
			publicSrv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = svc.Shutdown(ctx)
			if primarySrv != nil {
				primarySrv.Close()
			}
			if backupSrv != nil {
				backupSrv.Close()
			}
		},
	}
}

type probeStubServer struct {
	server    *httptest.Server
	releaseCh chan struct{}
}

func (s *probeStubServer) URL() string {
	if s == nil || s.server == nil {
		return ""
	}
	return s.server.URL
}

func (s *probeStubServer) Release() {
	if s == nil || s.releaseCh == nil {
		return
	}
	select {
	case <-s.releaseCh:
	default:
		close(s.releaseCh)
	}
}

func (s *probeStubServer) Close() {
	if s == nil {
		return
	}
	s.Release()
	if s.server != nil {
		s.server.Close()
	}
}

func newProbeStubServer(t *testing.T, status int) *probeStubServer {
	t.Helper()

	return newProbeStubServerWithHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
	}))
}

func newBlockingProbeStubServer(t *testing.T) *probeStubServer {
	t.Helper()

	releaseCh := make(chan struct{})
	return newProbeStubServerWithHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		<-releaseCh
		w.WriteHeader(http.StatusOK)
	}), releaseCh)
}

func newProbeStubServerWithHandler(t *testing.T, handler http.Handler, releaseCh ...chan struct{}) *probeStubServer {
	t.Helper()

	stub := &probeStubServer{}
	if len(releaseCh) > 0 {
		stub.releaseCh = releaseCh[0]
	}
	stub.server = newHTTPServer(t, handler)
	return stub
}

func startCustomHTTPTestEnv(t *testing.T, cfg config.Config) httpTestEnv {
	t.Helper()

	svc, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New() error = %v", err)
	}

	publicSrv := newHTTPServer(t, svc.Handler())
	token := ""
	for _, key := range cfg.Auth.ClientKeys {
		if key.Active && key.Secret != "" {
			token = key.Secret
			break
		}
	}

	return httpTestEnv{
		BaseURL: publicSrv.URL,
		Client:  publicSrv.Client(),
		Token:   token,
		Service: svc,
		closeFn: func() {
			publicSrv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = svc.Shutdown(ctx)
		},
	}
}

func get(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("client.Get() error = %v", err)
	}

	return resp
}

func waitForStatus(t *testing.T, client *http.Client, url string, want int) *http.Response {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp := get(t, client, url)
		body := readBody(t, resp)
		if resp.StatusCode == want {
			resp.Body = io.NopCloser(strings.NewReader(body))
			return resp
		}
		time.Sleep(25 * time.Millisecond)
	}

	resp := get(t, client, url)
	body := readBody(t, resp)
	t.Fatalf("status = %d, want %d, body = %q", resp.StatusCode, want, body)
	return nil
}

func getRuntimeSnapshot(t *testing.T, client *http.Client, baseURL string) health.RuntimeSnapshot {
	t.Helper()

	resp := get(t, client, baseURL+"/admin/upstreams")
	assertStatus(t, resp, http.StatusOK)

	body := readBody(t, resp)

	var snapshot health.RuntimeSnapshot
	if err := json.Unmarshal([]byte(body), &snapshot); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	return snapshot
}

func waitForRuntimeSnapshot(t *testing.T, client *http.Client, baseURL string, predicate func(health.RuntimeSnapshot) bool) health.RuntimeSnapshot {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := getRuntimeSnapshot(t, client, baseURL)
		if predicate(snapshot) {
			return snapshot
		}
		time.Sleep(25 * time.Millisecond)
	}

	snapshot := getRuntimeSnapshot(t, client, baseURL)
	t.Fatalf("runtime snapshot did not satisfy predicate: %+v", snapshot)
	return health.RuntimeSnapshot{}
}

func newHTTPProviderStub(t *testing.T, scenario string) (*httptest.Server, *providerCapture) {
	t.Helper()

	capture := &providerCapture{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.record(r)

		if r.URL.Path == "/v1/models" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if !writeHTTPProviderScenario(w, r, scenario) {
			http.Error(w, "unknown scenario", http.StatusInternalServerError)
		}
	})

	return newHTTPServer(t, handler), capture
}

func applyTestProviderAuthEnv(t *testing.T, provider *config.ProviderConfig) {
	t.Helper()

	switch provider.Provider {
	case "openai":
		if provider.APIKeyEnv == "" {
			provider.APIKeyEnv = "OPENAI_API_KEY"
		}
		t.Setenv(provider.APIKeyEnv, "openai-test-key")
	case "anthropic":
		if provider.APIKeyEnv == "" {
			provider.APIKeyEnv = "ANTHROPIC_API_KEY"
		}
		t.Setenv(provider.APIKeyEnv, "anthropic-test-key")
	}
}

type mutableHTTPProviderStub struct {
	mu          sync.RWMutex
	scenario    string
	probeStatus int
	capture     *providerCapture
	server      *httptest.Server
}

func newMutableHTTPProviderStub(t *testing.T, scenario string, probeStatus int) *mutableHTTPProviderStub {
	t.Helper()

	stub := &mutableHTTPProviderStub{
		scenario:    scenario,
		probeStatus: probeStatus,
		capture:     &providerCapture{},
	}
	stub.server = newHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.capture.record(r)

		if r.URL.Path == "/v1/models" {
			w.WriteHeader(stub.ProbeStatus())
			return
		}

		if !writeHTTPProviderScenario(w, r, stub.Scenario()) {
			http.Error(w, "unknown scenario", http.StatusInternalServerError)
		}
	}))

	return stub
}

func (s *mutableHTTPProviderStub) Close() {
	if s != nil && s.server != nil {
		s.server.Close()
	}
}

func (s *mutableHTTPProviderStub) URL() string {
	if s == nil || s.server == nil {
		return ""
	}
	return s.server.URL
}

func (s *mutableHTTPProviderStub) Hits() int {
	if s == nil || s.capture == nil {
		return 0
	}
	return s.capture.Hits()
}

func (s *mutableHTTPProviderStub) Scenario() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scenario
}

func (s *mutableHTTPProviderStub) SetScenario(scenario string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scenario = scenario
}

func (s *mutableHTTPProviderStub) ProbeStatus() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.probeStatus
}

func (s *mutableHTTPProviderStub) SetProbeStatus(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probeStatus = status
}

func writeHTTPProviderScenario(w http.ResponseWriter, r *http.Request, scenario string) bool {
	switch scenario {
	case "openai_text":
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return true
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		return true
	case "openai_text_usage":
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return true
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18}}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		return true
	case "openai_text_length":
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return true
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		return true
	case "openai_responses":
		if r.URL.Path != "/v1/responses" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return true
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hel\"}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"lo\"}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\"}\n\n")
		return true
	case "openai_responses_usage":
		if r.URL.Path != "/v1/responses" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return true
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hel\"}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"lo\"}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":11,\"output_tokens\":7,\"total_tokens\":18}}}\n\n")
		return true
	case "anthropic_text":
		if r.URL.Path != "/v1/messages" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return true
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"delta\":{\"text\":\"hel\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"delta\":{\"text\":\"lo\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: message_stop\ndata: {}\n\n")
		return true
	case "anthropic_text_usage":
		if r.URL.Path != "/v1/messages" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return true
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: message_start\ndata: {\"message\":{\"usage\":{\"input_tokens\":11,\"output_tokens\":0}}}\n\n")
		_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"delta\":{\"text\":\"hel\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"delta\":{\"text\":\"lo\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: message_delta\ndata: {\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":7}}\n\n")
		_, _ = fmt.Fprint(w, "event: message_stop\ndata: {}\n\n")
		return true
	case "rate_limit":
		http.Error(w, `{"error":{"message":"rate limit"}}`, http.StatusTooManyRequests)
		return true
	case "partial_stream_interrupt_after_output":
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return true
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = fmt.Fprint(w, "data: {\"broken\":\n\n")
		return true
	default:
		return false
	}
}

func newHTTPServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}

	server := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	server.Start()
	return server
}

func postJSON(t *testing.T, client *http.Client, url, token string, payload any) *http.Response {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do() error = %v", err)
	}

	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("resp.Body.Close() error = %v", err)
	}

	return string(body)
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d", resp.StatusCode, want)
	}
}

func assertHeaderContains(t *testing.T, resp *http.Response, header, want string) {
	t.Helper()
	if got := resp.Header.Get(header); !strings.Contains(got, want) {
		t.Fatalf("%s = %q, want to contain %q", header, got, want)
	}
}

func assertBodyContains(t *testing.T, body string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(body, part) {
			t.Fatalf("body = %q, want to contain %q", body, part)
		}
	}
}

func assertJSONErrorType(t *testing.T, body, wantType string) {
	t.Helper()

	var payload struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Error.Type != wantType {
		t.Fatalf("error.type = %q, want %q", payload.Error.Type, wantType)
	}
}

func chatTextRequest(model string, stream bool) map[string]any {
	return map[string]any{
		"model":  model,
		"stream": stream,
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": "hello",
			},
		},
	}
}

func responsesTextRequest(model string, stream bool) map[string]any {
	return map[string]any{
		"model":  model,
		"stream": stream,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": "hello",
					},
				},
			},
		},
	}
}

func chatToolsRequest() map[string]any {
	req := chatTextRequest("openai/gpt-4.1", false)
	req["tools"] = []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name": "lookup_weather",
				"parameters": map[string]any{
					"type": "object",
				},
			},
		},
	}
	return req
}

func responsesVisionRequest() map[string]any {
	return map[string]any{
		"model":  "openai/gpt-4.1",
		"stream": false,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type":      "input_image",
						"image_url": "https://example.com/cat.png",
					},
				},
			},
		},
	}
}

func responsesStructuredOutputRequest() map[string]any {
	req := responsesTextRequest("openai/gpt-4.1", false)
	req["text"] = map[string]any{
		"format": map[string]any{
			"type": "json_schema",
			"name": "answer",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"answer": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}
	return req
}
