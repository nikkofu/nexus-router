package handlers

import (
	"net/http"

	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/health"
)

type RuntimeSnapshotReader interface {
	Snapshot() health.RuntimeSnapshot
}

func Livez() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func Readyz(cfg config.Config, runtime RuntimeSnapshotReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if runtime == nil {
			http.Error(w, "runtime health unavailable", http.StatusServiceUnavailable)
			return
		}

		snapshot := runtime.Snapshot()
		if !snapshot.Started {
			http.Error(w, "runtime not started", http.StatusServiceUnavailable)
			return
		}
		if cfg.Health.RequireInitialProbe && !snapshot.InitialProbeComplete {
			http.Error(w, "initial probe pending", http.StatusServiceUnavailable)
			return
		}
		if !allRequiredRouteGroupsReady(cfg, snapshot) {
			http.Error(w, "required route group has no eligible upstream", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
}

func allRequiredRouteGroupsReady(cfg config.Config, snapshot health.RuntimeSnapshot) bool {
	requiredGroups := make(map[string]struct{})
	for _, model := range cfg.Models {
		if model.RouteGroup == "" {
			continue
		}
		requiredGroups[model.RouteGroup] = struct{}{}
	}

	routeGroups := make(map[string]config.RouteGroupConfig, len(cfg.Routing.RouteGroups))
	for _, group := range cfg.Routing.RouteGroups {
		if group.Name == "" {
			continue
		}
		routeGroups[group.Name] = group
	}

	providers := make(map[string]struct{}, len(cfg.Providers))
	for _, provider := range cfg.Providers {
		if provider.Name == "" {
			continue
		}
		providers[provider.Name] = struct{}{}
	}

	upstreams := make(map[string]health.UpstreamStatus, len(snapshot.Upstreams))
	for _, upstream := range snapshot.Upstreams {
		upstreams[upstream.Name] = upstream
	}

	for groupName := range requiredGroups {
		group, ok := routeGroups[groupName]
		if !ok {
			return false
		}
		if !routeGroupHasEligibleUpstream(group, providers, upstreams) {
			return false
		}
	}

	return true
}

func routeGroupHasEligibleUpstream(group config.RouteGroupConfig, providers map[string]struct{}, upstreams map[string]health.UpstreamStatus) bool {
	candidates := append([]string{group.Primary}, group.Fallbacks...)
	for _, upstreamName := range candidates {
		if upstreamName == "" {
			continue
		}
		if _, ok := providers[upstreamName]; !ok {
			continue
		}
		upstream, ok := upstreams[upstreamName]
		if !ok {
			continue
		}
		if upstream.Eligible {
			return true
		}
	}

	return false
}
