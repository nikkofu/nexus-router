package app

import (
	"net/http"

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
	server  *http.Server
	handler http.Handler
	tls     config.TLSConfig
}

func New(cfg config.Config) (*Service, error) {
	resolver := auth.NewResolver(cfg.Auth.ClientKeys)
	manager := health.NewManager()
	planner := routeplanner.NewPlanner(cfg, manager)
	executor := runtime.NewExecutor(runtime.NewRegistry(cfg.Providers), http.DefaultClient)
	executeService := service.NewExecuteService(capabilities.DefaultRegistry(), &planner, executor)
	handler := httpapi.NewRouter(cfg, resolver, executeService)

	logger := observability.NewLogger()
	server := &http.Server{
		Addr:    cfg.Server.ListenAddr,
		Handler: handler,
	}
	if server.Addr == "" {
		server.Addr = "127.0.0.1:8080"
	}

	logger.Info("configured nexus-router service", "addr", server.Addr)

	return &Service{
		server:  server,
		handler: handler,
		tls:     cfg.Server.TLS,
	}, nil
}

func (s *Service) Handler() http.Handler {
	return s.handler
}
