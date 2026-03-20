package config

import (
	"errors"
	"fmt"
)

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
