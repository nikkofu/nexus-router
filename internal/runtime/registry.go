package runtime

import (
	"fmt"

	"github.com/nikkofu/nexus-router/internal/config"
)

type Registry struct {
	providers map[string]config.ProviderConfig
}

func NewRegistry(providers []config.ProviderConfig) Registry {
	index := make(map[string]config.ProviderConfig, len(providers))
	for _, provider := range providers {
		index[provider.Name] = provider
	}

	return Registry{providers: index}
}

func (r Registry) Resolve(upstream string) (config.ProviderConfig, error) {
	provider, ok := r.providers[upstream]
	if !ok {
		return config.ProviderConfig{}, fmt.Errorf("runtime registry: unknown upstream %q", upstream)
	}

	return provider, nil
}
