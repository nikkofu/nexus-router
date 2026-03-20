package capabilities

import "github.com/nikkofu/nexus-router/internal/canonical"

type Registry struct {
	profiles []Profile
}

type Profile struct {
	Name               string
	ModelPattern       string
	SupportsVision     bool
	SupportsStructured bool
	SupportsTools      bool
	SupportedEndpoints []canonical.EndpointKind
}

func DefaultRegistry() Registry {
	return Registry{
		profiles: []Profile{
			{
				Name:               "openai-family",
				ModelPattern:       "openai/gpt-*",
				SupportsVision:     true,
				SupportsStructured: true,
				SupportsTools:      true,
				SupportedEndpoints: []canonical.EndpointKind{
					canonical.EndpointKindChatCompletions,
					canonical.EndpointKindResponses,
				},
			},
			{
				Name:               "anthropic-family",
				ModelPattern:       "anthropic/claude-*",
				SupportsVision:     true,
				SupportsStructured: true,
				SupportsTools:      true,
				SupportedEndpoints: []canonical.EndpointKind{
					canonical.EndpointKindChatCompletions,
					canonical.EndpointKindResponses,
				},
			},
		},
	}
}
