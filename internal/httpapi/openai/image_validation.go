package openai

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/nikkofu/nexus-router/internal/canonical"
)

func validatePublicImageURL(raw string) error {
	imageURL := strings.TrimSpace(raw)
	if imageURL == "" {
		return invalidRequestError("image_url must not be empty")
	}

	parsed, err := url.Parse(imageURL)
	if err != nil {
		return invalidRequestError("image_url must be a valid URL")
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		if parsed.Host == "" {
			return invalidRequestError("image_url must include a host")
		}
		return nil
	case "data":
		return unsupportedCapabilityError("data URL image inputs are not supported")
	default:
		return invalidRequestError("image_url must use http or https")
	}
}

func validatePublicImageRole(rawRole string, role canonical.Role) error {
	if rawRole != string(canonical.RoleUser) || role != canonical.RoleUser {
		return invalidRequestError("image content is only allowed on user messages")
	}

	return nil
}

func validateAllowedObjectKeys(raw map[string]any, allowed ...string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}

	for key := range raw {
		if _, ok := allowedSet[key]; ok {
			continue
		}
		return unsupportedCapabilityError("field %q is not supported on this public image form", key)
	}

	return nil
}

func invalidRequestError(format string, args ...any) error {
	return fmt.Errorf("invalid_request: "+format, args...)
}

func unsupportedCapabilityError(format string, args ...any) error {
	return fmt.Errorf("unsupported_capability: "+format, args...)
}
