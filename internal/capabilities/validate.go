package capabilities

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nikkofu/nexus-router/internal/auth"
	"github.com/nikkofu/nexus-router/internal/canonical"
)

func ValidateRequest(registry Registry, policy auth.ClientPolicy, req canonical.Request) error {
	profile, ok := registry.Match(req.PublicModel)
	if !ok {
		return fmt.Errorf("unsupported managed model family %q", req.PublicModel)
	}

	if requiresVision(req) {
		if !policy.AllowVision {
			return errors.New("vision requests are not allowed for this client policy")
		}
		if !profile.SupportsVision {
			return errors.New("vision is not supported for this model family")
		}
	}

	if len(req.Tools) > 0 {
		if !policy.AllowTools {
			return errors.New("tool use is not allowed for this client policy")
		}
		if !profile.SupportsTools {
			return errors.New("tool use is not supported for this model family")
		}
		for _, tool := range req.Tools {
			if err := ValidateSchemaSubset(tool.Schema); err != nil {
				return fmt.Errorf("tool %q schema invalid: %w", tool.Name, err)
			}
		}
	}

	if req.ResponseContract.Kind == canonical.ResponseContractJSONSchema {
		if !policy.AllowStructured {
			return errors.New("structured outputs are not allowed for this client policy")
		}
		if !profile.SupportsStructured {
			return errors.New("structured outputs are not supported for this model family")
		}
		if err := ValidateSchemaSubset(req.ResponseContract.Schema); err != nil {
			return err
		}
	}

	return nil
}

func (r Registry) Match(publicModel string) (Profile, bool) {
	for _, profile := range r.profiles {
		if matchesPattern(publicModel, profile.ModelPattern) {
			return profile, true
		}
	}

	return Profile{}, false
}

func requiresVision(req canonical.Request) bool {
	for _, turn := range req.Conversation {
		for _, block := range turn.Content {
			if block.Type == canonical.ContentTypeImage {
				return true
			}
		}
	}

	return false
}

func matchesPattern(value, pattern string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}

	return value == pattern
}
