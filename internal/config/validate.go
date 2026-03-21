package config

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

var supportedProbeMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodPatch:   {},
	http.MethodDelete:  {},
	http.MethodConnect: {},
	http.MethodOptions: {},
	http.MethodTrace:   {},
}

func Validate(cfg Config) error {
	if len(cfg.Auth.ClientKeys) == 0 {
		return errors.New("auth.client_keys must not be empty")
	}

	routeGroups := make(map[string]struct{}, len(cfg.Routing.RouteGroups))
	for _, group := range cfg.Routing.RouteGroups {
		if group.Name == "" {
			return errors.New("routing.route_groups[].name must not be empty")
		}
		routeGroups[group.Name] = struct{}{}
	}

	seenPatterns := make(map[string]struct{}, len(cfg.Models))
	for _, model := range cfg.Models {
		if model.Pattern == "" {
			return errors.New("models[].pattern must not be empty")
		}
		if _, exists := seenPatterns[model.Pattern]; exists {
			return fmt.Errorf("duplicate model pattern %q", model.Pattern)
		}
		seenPatterns[model.Pattern] = struct{}{}

		if _, exists := routeGroups[model.RouteGroup]; !exists {
			return fmt.Errorf("models[].route_group %q is undefined", model.RouteGroup)
		}
	}

	for _, provider := range cfg.Providers {
		if provider.Name == "" || provider.Provider == "" || provider.BaseURL == "" {
			return fmt.Errorf("provider entries require name, provider, and base_url: %#v", provider)
		}

		if provider.Probe.Method != "" {
			if _, ok := supportedProbeMethods[provider.Probe.Method]; !ok {
				return fmt.Errorf("providers[].probe.method %q is unsupported", provider.Probe.Method)
			}
		}
		if provider.Probe.Path != "" && provider.Probe.Path[0] != '/' {
			return fmt.Errorf("providers[].probe.path %q must start with '/'", provider.Probe.Path)
		}
		for _, status := range provider.Probe.ExpectedStatuses {
			if status < 100 || status > 599 {
				return fmt.Errorf("providers[].probe.expected_statuses contains invalid status %d", status)
			}
		}
		if provider.Probe.Interval != "" {
			if _, err := time.ParseDuration(provider.Probe.Interval); err != nil {
				return fmt.Errorf("providers[].probe.interval %q is invalid duration: %w", provider.Probe.Interval, err)
			}
		}
		if provider.Probe.Timeout != "" {
			if _, err := time.ParseDuration(provider.Probe.Timeout); err != nil {
				return fmt.Errorf("providers[].probe.timeout %q is invalid duration: %w", provider.Probe.Timeout, err)
			}
		}
	}

	if cfg.Health.ProbeInterval != "" {
		if _, err := time.ParseDuration(cfg.Health.ProbeInterval); err != nil {
			return fmt.Errorf("health.probe_interval %q is invalid duration: %w", cfg.Health.ProbeInterval, err)
		}
	}
	if cfg.Health.ProbeTimeout != "" {
		if _, err := time.ParseDuration(cfg.Health.ProbeTimeout); err != nil {
			return fmt.Errorf("health.probe_timeout %q is invalid duration: %w", cfg.Health.ProbeTimeout, err)
		}
	}

	if cfg.Breaker.FailureThreshold < 1 {
		return errors.New("breaker.failure_threshold must be >= 1")
	}
	if cfg.Breaker.RecoverySuccessThreshold < 1 {
		return errors.New("breaker.recovery_success_threshold must be >= 1")
	}

	switch cfg.Server.TLS.Mode {
	case "", "disabled":
		if cfg.Server.TLS.CertFile != "" || cfg.Server.TLS.KeyFile != "" {
			return errors.New("server.tls cert/key require tls mode 'file'")
		}
	case "file":
		if cfg.Server.TLS.CertFile == "" || cfg.Server.TLS.KeyFile == "" {
			return errors.New("server.tls mode 'file' requires cert_file and key_file")
		}
	default:
		return fmt.Errorf("unsupported server.tls.mode %q", cfg.Server.TLS.Mode)
	}

	return nil
}
