package handlers

import (
	"net/http"
	"strings"

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

		writeJSON(w, map[string]any{
			"routes": summary,
		})
	})
}

type adminConfigSummary struct {
	Server    adminServerSummary     `json:"server"`
	Auth      adminAuthSummary       `json:"auth"`
	Models    []adminModelSummary    `json:"models"`
	Providers []adminProviderSummary `json:"providers"`
	Routing   adminRoutingSummary    `json:"routing"`
	Health    adminHealthSummary     `json:"health"`
	Breaker   adminBreakerSummary    `json:"breaker"`
	Limits    adminLimitsSummary     `json:"limits"`
}

type adminServerSummary struct {
	ListenAddr      string          `json:"listen_addr"`
	AdminListenAddr string          `json:"admin_listen_addr"`
	TLS             adminTLSSummary `json:"tls"`
}

type adminTLSSummary struct {
	Mode string `json:"mode"`
}

type adminAuthSummary struct {
	ClientKeys []adminClientKeySummary `json:"client_keys"`
}

type adminClientKeySummary struct {
	ID                   string   `json:"id"`
	Active               bool     `json:"active"`
	AllowedModelPatterns []string `json:"allowed_model_patterns"`
	AllowStreaming       bool     `json:"allow_streaming"`
	AllowTools           bool     `json:"allow_tools"`
	AllowVision          bool     `json:"allow_vision"`
	AllowStructured      bool     `json:"allow_structured"`
}

type adminModelSummary struct {
	Pattern    string `json:"pattern"`
	RouteGroup string `json:"route_group"`
}

type adminProviderSummary struct {
	Name         string            `json:"name"`
	Provider     string            `json:"provider"`
	BaseURL      string            `json:"base_url"`
	HasAPIKeyEnv bool              `json:"has_api_key_env"`
	Probe        adminProbeSummary `json:"probe"`
}

type adminProbeSummary struct {
	Method           string `json:"method"`
	Path             string `json:"path"`
	HasHeaders       bool   `json:"has_headers"`
	ExpectedStatuses []int  `json:"expected_statuses"`
	Interval         string `json:"interval"`
	Timeout          string `json:"timeout"`
}

type adminRoutingSummary struct {
	RouteGroups []adminRouteGroupSummary `json:"route_groups"`
}

type adminRouteGroupSummary struct {
	Name      string   `json:"name"`
	Primary   string   `json:"primary"`
	Fallbacks []string `json:"fallbacks"`
}

type adminHealthSummary struct {
	ProbeInterval       string `json:"probe_interval"`
	ProbeTimeout        string `json:"probe_timeout"`
	RequireInitialProbe bool   `json:"require_initial_probe"`
}

type adminBreakerSummary struct {
	FailureThreshold         int    `json:"failure_threshold"`
	RecoverySuccessThreshold int    `json:"recovery_success_threshold"`
	OpenInterval             string `json:"open_interval"`
}

type adminLimitsSummary struct {
	MaxRequestBytes int64 `json:"max_request_bytes"`
}

func AdminConfig(cfg config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, newAdminConfigSummary(cfg))
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

func newAdminConfigSummary(cfg config.Config) adminConfigSummary {
	clientKeys := make([]adminClientKeySummary, 0, len(cfg.Auth.ClientKeys))
	for _, key := range cfg.Auth.ClientKeys {
		clientKeys = append(clientKeys, adminClientKeySummary{
			ID:                   key.ID,
			Active:               key.Active,
			AllowedModelPatterns: append([]string(nil), key.AllowedModelPatterns...),
			AllowStreaming:       key.AllowStreaming,
			AllowTools:           key.AllowTools,
			AllowVision:          key.AllowVision,
			AllowStructured:      key.AllowStructured,
		})
	}

	providers := make([]adminProviderSummary, 0, len(cfg.Providers))
	for _, provider := range cfg.Providers {
		providers = append(providers, adminProviderSummary{
			Name:         provider.Name,
			Provider:     provider.Provider,
			BaseURL:      provider.BaseURL,
			HasAPIKeyEnv: strings.TrimSpace(provider.APIKeyEnv) != "",
			Probe: adminProbeSummary{
				Method:           provider.Probe.Method,
				Path:             provider.Probe.Path,
				HasHeaders:       len(provider.Probe.Headers) > 0,
				ExpectedStatuses: append([]int(nil), provider.Probe.ExpectedStatuses...),
				Interval:         provider.Probe.Interval,
				Timeout:          provider.Probe.Timeout,
			},
		})
	}

	models := make([]adminModelSummary, 0, len(cfg.Models))
	for _, model := range cfg.Models {
		models = append(models, adminModelSummary{
			Pattern:    model.Pattern,
			RouteGroup: model.RouteGroup,
		})
	}

	routeGroups := make([]adminRouteGroupSummary, 0, len(cfg.Routing.RouteGroups))
	for _, group := range cfg.Routing.RouteGroups {
		routeGroups = append(routeGroups, adminRouteGroupSummary{
			Name:      group.Name,
			Primary:   group.Primary,
			Fallbacks: append([]string(nil), group.Fallbacks...),
		})
	}

	return adminConfigSummary{
		Server: adminServerSummary{
			ListenAddr:      cfg.Server.ListenAddr,
			AdminListenAddr: cfg.Server.AdminListenAddr,
			TLS: adminTLSSummary{
				Mode: cfg.Server.TLS.Mode,
			},
		},
		Auth: adminAuthSummary{
			ClientKeys: clientKeys,
		},
		Models:    models,
		Providers: providers,
		Routing: adminRoutingSummary{
			RouteGroups: routeGroups,
		},
		Health: adminHealthSummary{
			ProbeInterval:       cfg.Health.ProbeInterval,
			ProbeTimeout:        cfg.Health.ProbeTimeout,
			RequireInitialProbe: cfg.Health.RequireInitialProbe,
		},
		Breaker: adminBreakerSummary{
			FailureThreshold:         cfg.Breaker.FailureThreshold,
			RecoverySuccessThreshold: cfg.Breaker.RecoverySuccessThreshold,
			OpenInterval:             cfg.Breaker.OpenInterval,
		},
		Limits: adminLimitsSummary{
			MaxRequestBytes: cfg.Limits.MaxRequestBytes,
		},
	}
}
