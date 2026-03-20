package app

import (
	"net/http"

	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/httpapi"
	"github.com/nikkofu/nexus-router/internal/observability"
)

type Service struct {
	server  *http.Server
	handler http.Handler
}

func New(cfg config.Config) (*Service, error) {
	handler := httpapi.NewRouter(cfg)

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
	}, nil
}

func (s *Service) Handler() http.Handler {
	return s.handler
}
