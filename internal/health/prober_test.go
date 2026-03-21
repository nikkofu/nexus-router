package health

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nikkofu/nexus-router/internal/config"
)

func TestProberMarksInitialSweepCompleteAfterAllUpstreamsRun(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-test-key")
	t.Setenv("ANTHROPIC_API_KEY", "anthropic-test-key")

	var (
		mu       sync.Mutex
		requests = make(map[string]int)
	)

	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			requests[req.Header.Get("Authorization")+"|"+req.Header.Get("x-api-key")+"|"+req.URL.String()]++
			mu.Unlock()

			return jsonResponse(req, http.StatusOK), nil
		}),
	}

	runtime := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
			{Name: "anthropic-main", Provider: "anthropic"},
		},
	})

	prober := NewProber(ProberOptions{
		Providers: []config.ProviderConfig{
			{
				Name:      "openai-main",
				Provider:  "openai",
				BaseURL:   "https://openai.test",
				APIKeyEnv: "OPENAI_API_KEY",
			},
			{
				Name:      "anthropic-main",
				Provider:  "anthropic",
				BaseURL:   "https://anthropic.test",
				APIKeyEnv: "ANTHROPIC_API_KEY",
			},
		},
		Runtime:       runtime,
		Client:        client,
		ProbeInterval: 50 * time.Millisecond,
		ProbeTimeout:  time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := startProber(ctx, prober)

	waitFor(t, time.Second, func() bool {
		snap := runtime.Snapshot()
		if !snap.Started || !snap.InitialProbeComplete {
			return false
		}

		openai := proberUpstreamByName(t, snap, "openai-main")
		anthropic := proberUpstreamByName(t, snap, "anthropic-main")
		return openai.LastProbeOK && anthropic.LastProbeOK
	})

	cancel()
	waitForClosed(t, done)

	mu.Lock()
	defer mu.Unlock()

	if requests["Bearer openai-test-key||https://openai.test/v1/models"] == 0 {
		t.Fatalf("openai probe request missing, got %v", requests)
	}
	if requests["|anthropic-test-key|https://anthropic.test/v1/models"] == 0 {
		t.Fatalf("anthropic probe request missing, got %v", requests)
	}
}

func TestProberAppliesProviderFamilyDefaultsAndMergesHeaders(t *testing.T) {
	t.Run("openai", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "openai-test-key")

		var captured *http.Request
		client := &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				captured = req.Clone(req.Context())
				return jsonResponse(req, http.StatusOK), nil
			}),
		}

		runtime := NewRuntime(RuntimeOptions{
			Upstreams: []RuntimeUpstream{{Name: "openai-main", Provider: "openai"}},
		})

		prober := NewProber(ProberOptions{
			Providers: []config.ProviderConfig{
				{
					Name:      "openai-main",
					Provider:  "openai",
					BaseURL:   "https://openai.test",
					APIKeyEnv: "OPENAI_API_KEY",
					Probe: config.ProbeConfig{
						Headers: map[string]string{
							"Authorization": "Bearer override",
							"X-Custom":      "custom-openai",
						},
					},
				},
			},
			Runtime:       runtime,
			Client:        client,
			ProbeInterval: time.Minute,
			ProbeTimeout:  time.Second,
		})

		ctx, cancel := context.WithCancel(context.Background())
		done := startProber(ctx, prober)
		waitFor(t, time.Second, func() bool {
			return runtime.Snapshot().InitialProbeComplete
		})
		cancel()
		waitForClosed(t, done)

		if captured == nil {
			t.Fatal("expected probe request to be captured")
		}
		if captured.Method != http.MethodGet {
			t.Fatalf("method = %q, want %q", captured.Method, http.MethodGet)
		}
		if captured.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want %q", captured.URL.Path, "/v1/models")
		}
		if got := captured.Header.Get("Authorization"); got != "Bearer openai-test-key" {
			t.Fatalf("authorization = %q, want %q", got, "Bearer openai-test-key")
		}
		if got := captured.Header.Get("X-Custom"); got != "custom-openai" {
			t.Fatalf("x-custom = %q, want %q", got, "custom-openai")
		}
	})

	t.Run("anthropic", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "anthropic-test-key")

		var captured *http.Request
		client := &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				captured = req.Clone(req.Context())
				return jsonResponse(req, http.StatusOK), nil
			}),
		}

		runtime := NewRuntime(RuntimeOptions{
			Upstreams: []RuntimeUpstream{{Name: "anthropic-main", Provider: "anthropic"}},
		})

		prober := NewProber(ProberOptions{
			Providers: []config.ProviderConfig{
				{
					Name:      "anthropic-main",
					Provider:  "anthropic",
					BaseURL:   "https://anthropic.test",
					APIKeyEnv: "ANTHROPIC_API_KEY",
					Probe: config.ProbeConfig{
						Headers: map[string]string{
							"x-api-key":         "override",
							"anthropic-version": "override-version",
							"X-Custom":          "custom-anthropic",
						},
					},
				},
			},
			Runtime:       runtime,
			Client:        client,
			ProbeInterval: time.Minute,
			ProbeTimeout:  time.Second,
		})

		ctx, cancel := context.WithCancel(context.Background())
		done := startProber(ctx, prober)
		waitFor(t, time.Second, func() bool {
			return runtime.Snapshot().InitialProbeComplete
		})
		cancel()
		waitForClosed(t, done)

		if captured == nil {
			t.Fatal("expected probe request to be captured")
		}
		if captured.Method != http.MethodGet {
			t.Fatalf("method = %q, want %q", captured.Method, http.MethodGet)
		}
		if captured.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want %q", captured.URL.Path, "/v1/models")
		}
		if got := captured.Header.Get("x-api-key"); got != "anthropic-test-key" {
			t.Fatalf("x-api-key = %q, want %q", got, "anthropic-test-key")
		}
		if got := captured.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Fatalf("anthropic-version = %q, want %q", got, "2023-06-01")
		}
		if got := captured.Header.Get("X-Custom"); got != "custom-anthropic" {
			t.Fatalf("x-custom = %q, want %q", got, "custom-anthropic")
		}
	})
}

func TestProberFailsFastWithoutAPIKeyAndDoesNotDial(t *testing.T) {
	var called atomic.Int32

	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			called.Add(1)
			return jsonResponse(req, http.StatusOK), nil
		}),
	}

	runtime := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{{Name: "openai-main", Provider: "openai"}},
	})

	prober := NewProber(ProberOptions{
		Providers: []config.ProviderConfig{
			{
				Name:      "openai-main",
				Provider:  "openai",
				BaseURL:   "https://openai.test",
				APIKeyEnv: "OPENAI_API_KEY",
			},
		},
		Runtime:       runtime,
		Client:        client,
		ProbeInterval: time.Minute,
		ProbeTimeout:  time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := startProber(ctx, prober)

	waitFor(t, time.Second, func() bool {
		return runtime.Snapshot().InitialProbeComplete
	})

	cancel()
	waitForClosed(t, done)

	if got := called.Load(); got != 0 {
		t.Fatalf("transport calls = %d, want 0", got)
	}

	snap := runtime.Snapshot()
	upstream := proberUpstreamByName(t, snap, "openai-main")
	if upstream.LastProbeOK {
		t.Fatalf("last_probe_ok = %v, want false", upstream.LastProbeOK)
	}
	if !strings.Contains(upstream.LastError, "OPENAI_API_KEY") {
		t.Fatalf("last_error = %q, want mention of missing env var", upstream.LastError)
	}
	if !snap.InitialProbeComplete {
		t.Fatal("initial probe should be marked complete after fail-fast result")
	}
}

func TestProberFailsFastWithoutAPIKeyEnvConfigAndDoesNotDial(t *testing.T) {
	var called atomic.Int32

	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			called.Add(1)
			return jsonResponse(req, http.StatusOK), nil
		}),
	}

	runtime := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{{Name: "openai-main", Provider: "openai"}},
	})

	prober := NewProber(ProberOptions{
		Providers: []config.ProviderConfig{
			{
				Name:     "openai-main",
				Provider: "openai",
				BaseURL:  "https://openai.test",
			},
		},
		Runtime:       runtime,
		Client:        client,
		ProbeInterval: time.Minute,
		ProbeTimeout:  time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := startProber(ctx, prober)

	waitFor(t, time.Second, func() bool {
		return runtime.Snapshot().InitialProbeComplete
	})

	cancel()
	waitForClosed(t, done)

	if got := called.Load(); got != 0 {
		t.Fatalf("transport calls = %d, want 0", got)
	}

	snap := runtime.Snapshot()
	upstream := proberUpstreamByName(t, snap, "openai-main")
	if !strings.Contains(upstream.LastError, "api_key_env") {
		t.Fatalf("last_error = %q, want mention of missing api_key_env config", upstream.LastError)
	}
}

func TestProberDoesNotOverlapProbesForSameUpstream(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-test-key")

	var (
		inFlight      atomic.Int32
		maxConcurrent atomic.Int32
		completions   atomic.Int32
	)

	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			current := inFlight.Add(1)
			for {
				max := maxConcurrent.Load()
				if current <= max || maxConcurrent.CompareAndSwap(max, current) {
					break
				}
			}

			time.Sleep(40 * time.Millisecond)
			inFlight.Add(-1)
			completions.Add(1)
			return jsonResponse(req, http.StatusOK), nil
		}),
	}

	runtime := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{{Name: "openai-main", Provider: "openai"}},
	})

	prober := NewProber(ProberOptions{
		Providers: []config.ProviderConfig{
			{
				Name:      "openai-main",
				Provider:  "openai",
				BaseURL:   "https://openai.test",
				APIKeyEnv: "OPENAI_API_KEY",
			},
		},
		Runtime:       runtime,
		Client:        client,
		ProbeInterval: 5 * time.Millisecond,
		ProbeTimeout:  time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := startProber(ctx, prober)

	waitFor(t, time.Second, func() bool {
		return completions.Load() >= 2
	})

	cancel()
	waitForClosed(t, done)

	if got := maxConcurrent.Load(); got != 1 {
		t.Fatalf("max concurrent probes = %d, want 1", got)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(req *http.Request, status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		Request:    req,
	}
}

func startProber(ctx context.Context, prober *Prober) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		prober.Start(ctx)
	}()

	return done
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("condition not met before timeout")
}

func waitForClosed(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("prober did not stop before timeout")
	}
}

func proberUpstreamByName(t *testing.T, snap RuntimeSnapshot, name string) UpstreamStatus {
	t.Helper()

	for _, upstream := range snap.Upstreams {
		if upstream.Name == name {
			return upstream
		}
	}

	t.Fatalf("upstream %q not found in snapshot", name)
	return UpstreamStatus{}
}
