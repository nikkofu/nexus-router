package openai

import "github.com/nikkofu/nexus-router/internal/canonical"

func EncodeRequest(req canonical.Request) ([]byte, error) {
	switch req.EndpointKind {
	case canonical.EndpointKindResponses:
		return marshal(map[string]any{
			"model":  req.PublicModel,
			"stream": req.Stream,
			"input":  []any{},
		})
	default:
		return marshal(map[string]any{
			"model":    req.PublicModel,
			"stream":   req.Stream,
			"messages": []any{},
		})
	}
}
