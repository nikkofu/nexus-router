package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/nikkofu/nexus-router/internal/config"
)

type routeSummary struct {
	Pattern    string `json:"pattern"`
	RouteGroup string `json:"route_group"`
}

func AdminRoutes(cfg config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		summary := make([]routeSummary, 0, len(cfg.Models))
		for _, model := range cfg.Models {
			summary = append(summary, routeSummary{
				Pattern:    model.Pattern,
				RouteGroup: model.RouteGroup,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"routes": summary,
		})
	})
}

func AdminUpstreams(runtime RuntimeSnapshotReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if runtime == nil {
			http.Error(w, "runtime health unavailable", http.StatusServiceUnavailable)
			return
		}

		writeJSON(w, runtime.Snapshot())
	})
}
