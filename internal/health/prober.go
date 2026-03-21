package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nikkofu/nexus-router/internal/config"
)

type ProberOptions struct {
	Providers     []config.ProviderConfig
	Runtime       *Runtime
	Client        *http.Client
	ProbeInterval time.Duration
	ProbeTimeout  time.Duration
}

type Prober struct {
	runtime *Runtime
	client  *http.Client
	targets []probeTarget
}

type probeTarget struct {
	name          string
	method        string
	url           string
	headers       http.Header
	expectedCodes map[int]struct{}
	interval      time.Duration
	timeout       time.Duration
	failFastError string
}

func NewProber(opts ProberOptions) *Prober {
	client := opts.Client
	if client == nil {
		client = http.DefaultClient
	}

	targets := make([]probeTarget, 0, len(opts.Providers))
	for _, provider := range opts.Providers {
		if provider.Name == "" {
			continue
		}
		targets = append(targets, buildProbeTarget(provider, opts.ProbeInterval, opts.ProbeTimeout))
	}

	return &Prober{
		runtime: opts.Runtime,
		client:  client,
		targets: targets,
	}
}

func (p *Prober) Start(ctx context.Context) {
	if p.runtime != nil {
		p.runtime.MarkStarted()
		if len(p.targets) == 0 {
			p.runtime.MarkInitialProbeComplete()
		}
	}

	var (
		wg            sync.WaitGroup
		firstResults  int
		firstResultsM sync.Mutex
	)

	for _, target := range p.targets {
		target := target
		wg.Add(1)
		go func() {
			defer wg.Done()

			firstResultRecorded := false
			markFirstResult := func() {
				if firstResultRecorded || p.runtime == nil {
					return
				}
				firstResultRecorded = true

				firstResultsM.Lock()
				defer firstResultsM.Unlock()

				firstResults++
				if firstResults == len(p.targets) {
					p.runtime.MarkInitialProbeComplete()
				}
			}

			p.runProbeLoop(ctx, target, markFirstResult)
		}()
	}

	wg.Wait()
}

func (p *Prober) runProbeLoop(ctx context.Context, target probeTarget, markFirstResult func()) {
	p.probeOnce(ctx, target)
	markFirstResult()

	ticker := time.NewTicker(target.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.probeOnce(ctx, target)
		}
	}
}

func (p *Prober) probeOnce(ctx context.Context, target probeTarget) {
	if ctx.Err() != nil {
		return
	}

	at := time.Now()
	if target.failFastError != "" {
		if p.runtime != nil {
			p.runtime.RecordProbeFailure(target.name, at, target.failFastError)
		}
		return
	}

	probeCtx, cancel := context.WithTimeout(ctx, target.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, target.method, target.url, nil)
	if err != nil {
		if p.runtime != nil {
			p.runtime.RecordProbeFailure(target.name, at, err.Error())
		}
		return
	}
	req.Header = target.headers.Clone()

	resp, err := p.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			return
		}
		if p.runtime != nil {
			p.runtime.RecordProbeFailure(target.name, at, err.Error())
		}
		return
	}
	defer resp.Body.Close()

	if _, ok := target.expectedCodes[resp.StatusCode]; !ok {
		if p.runtime != nil {
			p.runtime.RecordProbeFailure(target.name, at, fmt.Sprintf("unexpected probe status %d", resp.StatusCode))
		}
		return
	}

	if p.runtime != nil {
		p.runtime.RecordProbeSuccess(target.name, at)
	}
}

func buildProbeTarget(provider config.ProviderConfig, defaultInterval, defaultTimeout time.Duration) probeTarget {
	method := strings.ToUpper(strings.TrimSpace(provider.Probe.Method))
	if method == "" {
		method = http.MethodGet
	}

	path := provider.Probe.Path
	if path == "" {
		path = defaultProbePath(provider.Provider)
	}

	interval := defaultInterval
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if provider.Probe.Interval != "" {
		if parsed, err := time.ParseDuration(provider.Probe.Interval); err == nil && parsed > 0 {
			interval = parsed
		}
	}

	timeout := defaultTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if provider.Probe.Timeout != "" {
		if parsed, err := time.ParseDuration(provider.Probe.Timeout); err == nil && parsed > 0 {
			timeout = parsed
		}
	}

	expectedCodes := make(map[int]struct{}, len(provider.Probe.ExpectedStatuses))
	if len(provider.Probe.ExpectedStatuses) == 0 {
		expectedCodes[http.StatusOK] = struct{}{}
	} else {
		for _, code := range provider.Probe.ExpectedStatuses {
			expectedCodes[code] = struct{}{}
		}
	}

	headers := make(http.Header, len(provider.Probe.Headers))
	for key, value := range provider.Probe.Headers {
		headers.Set(key, value)
	}

	failFastError := mergeProviderDefaultHeaders(headers, provider)

	return probeTarget{
		name:          provider.Name,
		method:        method,
		url:           strings.TrimRight(provider.BaseURL, "/") + path,
		headers:       headers,
		expectedCodes: expectedCodes,
		interval:      interval,
		timeout:       timeout,
		failFastError: failFastError,
	}
}

func defaultProbePath(provider string) string {
	switch provider {
	case "openai", "anthropic":
		return "/v1/models"
	default:
		return "/"
	}
}

func mergeProviderDefaultHeaders(headers http.Header, provider config.ProviderConfig) string {
	apiKey, missingKey := lookupAPIKey(provider.APIKeyEnv)

	switch provider.Provider {
	case "openai":
		if missingKey != "" {
			return missingKey
		}
		if apiKey != "" {
			headers.Set("Authorization", "Bearer "+apiKey)
		}
	case "anthropic":
		headers.Set("anthropic-version", "2023-06-01")
		if missingKey != "" {
			return missingKey
		}
		if apiKey != "" {
			headers.Set("x-api-key", apiKey)
		}
	}

	return ""
}

func lookupAPIKey(envVar string) (string, string) {
	if envVar == "" {
		return "", "missing probe api_key_env config"
	}

	value, ok := os.LookupEnv(envVar)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Sprintf("missing probe API key env %s", envVar)
	}

	return value, ""
}
