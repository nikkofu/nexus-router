package httpapi

import (
	"net/http"

	"github.com/nikkofu/nexus-router/internal/auth"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/httpapi/handlers"
)

func NewRouter(cfg config.Config) http.Handler {
	mux := http.NewServeMux()
	resolver := auth.NewResolver(cfg.Auth.ClientKeys)

	mux.Handle("/livez", handlers.Livez())
	mux.Handle("/readyz", handlers.Readyz(cfg))
	mux.Handle("/admin/routes", handlers.AdminRoutes(cfg))
	mux.Handle("/v1/chat/completions", RequireBearer(resolver, handlers.NotImplemented()))
	mux.Handle("/v1/responses", RequireBearer(resolver, handlers.NotImplemented()))

	return mux
}
