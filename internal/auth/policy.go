package auth

import "github.com/nikkofu/nexus-router/internal/config"

type ClientPolicy struct {
	ID                   string
	AllowedModelPatterns []string
	AllowStreaming       bool
	AllowTools           bool
	AllowVision          bool
	AllowStructured      bool
}

type Resolver struct {
	policies map[string]ClientPolicy
}

func NewResolver(keys []config.ClientKeyConfig) Resolver {
	policies := make(map[string]ClientPolicy, len(keys))
	for _, key := range keys {
		if !key.Active || key.Secret == "" {
			continue
		}

		policies[key.Secret] = ClientPolicy{
			ID:                   key.ID,
			AllowedModelPatterns: key.AllowedModelPatterns,
			AllowStreaming:       key.AllowStreaming,
			AllowTools:           key.AllowTools,
			AllowVision:          key.AllowVision,
			AllowStructured:      key.AllowStructured,
		}
	}

	return Resolver{policies: policies}
}

func (r Resolver) ResolveBearer(token string) (ClientPolicy, bool) {
	policy, ok := r.policies[token]
	return policy, ok
}
