package httpapi

import (
	"net/http"

	"github.com/nikkofu/nexus-router/internal/auth"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/httpapi/handlers"
)

func NewRouter(cfg config.Config, resolver auth.Resolver, exec handlers.ExecuteService, runtime handlers.RuntimeSnapshotReader) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("/livez", handlers.Livez())
	mux.Handle("/readyz", handlers.Readyz(cfg, runtime))
	mux.Handle("/admin/config", handlers.AdminConfig(cfg))
	mux.Handle("/admin/routes", handlers.AdminRoutes(cfg))
	mux.Handle("/admin/upstreams", handlers.AdminUpstreams(runtime))
	mux.Handle("/v1/chat/completions", RequireBearer(resolver, handlers.ChatCompletions(exec, ClientPolicyFromContext)))
	mux.Handle("/v1/responses", RequireBearer(resolver, handlers.Responses(exec, ClientPolicyFromContext)))

	return mux
}
