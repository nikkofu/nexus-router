package app

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/nikkofu/nexus-router/internal/auth"
	"github.com/nikkofu/nexus-router/internal/capabilities"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/health"
	"github.com/nikkofu/nexus-router/internal/httpapi"
	"github.com/nikkofu/nexus-router/internal/observability"
	routeplanner "github.com/nikkofu/nexus-router/internal/router"
	"github.com/nikkofu/nexus-router/internal/runtime"
	"github.com/nikkofu/nexus-router/internal/service"
)

type Service struct {
	server        *http.Server
	adminServer   *http.Server
	handler       http.Handler
	tls           config.TLSConfig
	runtimeCancel context.CancelFunc
	runtimeDone   <-chan struct{}
}

func New(cfg config.Config) (*Service, error) {
	resolver := auth.NewResolver(cfg.Auth.ClientKeys)
	breakerOpenInterval := durationOrDefault(cfg.Breaker.OpenInterval, 30*time.Second)
	healthProbeInterval := durationOrDefault(cfg.Health.ProbeInterval, 15*time.Second)
	healthProbeTimeout := durationOrDefault(cfg.Health.ProbeTimeout, 2*time.Second)

	runtimeHealth := health.NewRuntime(health.RuntimeOptions{
		Upstreams:                runtimeUpstreams(cfg.Providers),
		FailureThreshold:         intOrDefault(cfg.Breaker.FailureThreshold, 3),
		RecoverySuccessThreshold: intOrDefault(cfg.Breaker.RecoverySuccessThreshold, 1),
		OpenInterval:             breakerOpenInterval,
	})
	if !cfg.Health.RequireInitialProbe {
		now := time.Now()
		for _, provider := range cfg.Providers {
			if provider.Name == "" {
				continue
			}
			runtimeHealth.RecordRequestSuccess(provider.Name, now)
		}
	}

	planner := routeplanner.NewPlanner(cfg, runtimeHealth)
	executor := runtime.NewExecutor(runtime.NewRegistry(cfg.Providers), http.DefaultClient, runtimeHealth)
	executeService := service.NewExecuteService(capabilities.DefaultRegistry(), &planner, executor)
	handler := httpapi.NewRouter(cfg, resolver, executeService, runtimeHealth)
	publicHandler := handler
	var adminServer *http.Server
	if strings.TrimSpace(cfg.Server.AdminListenAddr) != "" {
		publicHandler = httpapi.NewPublicRouter(cfg, resolver, executeService, runtimeHealth)
		adminServer = &http.Server{
			Addr:    cfg.Server.AdminListenAddr,
			Handler: httpapi.NewAdminRouter(cfg, runtimeHealth),
		}
	}

	logger := observability.NewLogger()
	server := &http.Server{
		Addr:    cfg.Server.ListenAddr,
		Handler: publicHandler,
	}
	if server.Addr == "" {
		server.Addr = "127.0.0.1:8080"
	}

	runtimeCtx, runtimeCancel := context.WithCancel(context.Background())
	runtimeDone := make(chan struct{})
	go func() {
		defer close(runtimeDone)
		health.NewProber(health.ProberOptions{
			Providers:     cfg.Providers,
			Runtime:       runtimeHealth,
			Client:        http.DefaultClient,
			ProbeInterval: healthProbeInterval,
			ProbeTimeout:  healthProbeTimeout,
		}).Start(runtimeCtx)
	}()

	logger.Info("configured nexus-router service", "addr", server.Addr)

	return &Service{
		server:        server,
		adminServer:   adminServer,
		handler:       handler,
		tls:           cfg.Server.TLS,
		runtimeCancel: runtimeCancel,
		runtimeDone:   runtimeDone,
	}, nil
}

func (s *Service) Handler() http.Handler {
	return s.handler
}

func runtimeUpstreams(providers []config.ProviderConfig) []health.RuntimeUpstream {
	upstreams := make([]health.RuntimeUpstream, 0, len(providers))
	for _, provider := range providers {
		if provider.Name == "" {
			continue
		}
		upstreams = append(upstreams, health.RuntimeUpstream{
			Name:     provider.Name,
			Provider: provider.Provider,
		})
	}

	return upstreams
}

func durationOrDefault(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}

func intOrDefault(value, fallback int) int {
	if value <= 0 {
		return fallback
	}

	return value
}

func (s *Service) stopRuntimeWork() {
	if s.runtimeCancel != nil {
		s.runtimeCancel()
		s.runtimeCancel = nil
	}
	if s.runtimeDone != nil {
		<-s.runtimeDone
		s.runtimeDone = nil
	}
}
