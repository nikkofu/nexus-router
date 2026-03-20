package router

import (
	"strings"

	"github.com/nikkofu/nexus-router/internal/config"
)

func matchModel(publicModel string, models []config.ModelConfig) (config.ModelConfig, bool) {
	for _, model := range models {
		if matchesPattern(publicModel, model.Pattern) {
			return model, true
		}
	}

	return config.ModelConfig{}, false
}

func findRouteGroup(name string, groups []config.RouteGroupConfig) (config.RouteGroupConfig, bool) {
	for _, group := range groups {
		if group.Name == name {
			return group, true
		}
	}

	return config.RouteGroupConfig{}, false
}

func matchesPattern(value, pattern string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}

	return value == pattern
}
