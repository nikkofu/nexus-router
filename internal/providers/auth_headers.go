package providers

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/nikkofu/nexus-router/internal/config"
)

func ProviderAuthHeaders(provider config.ProviderConfig) (http.Header, error) {
	headers := make(http.Header)
	if err := MergeProviderAuthHeaders(headers, provider); err != nil {
		return nil, err
	}

	return headers, nil
}

func MergeProviderAuthHeaders(headers http.Header, provider config.ProviderConfig) error {
	apiKey, err := lookupProviderAPIKey(provider.APIKeyEnv)

	switch provider.Provider {
	case "openai":
		if err != nil {
			return err
		}
		headers.Set("Authorization", "Bearer "+apiKey)
	case "anthropic":
		headers.Set("anthropic-version", "2023-06-01")
		if err != nil {
			return err
		}
		headers.Set("x-api-key", apiKey)
	}

	return nil
}

func lookupProviderAPIKey(envVar string) (string, error) {
	if envVar == "" {
		return "", fmt.Errorf("missing provider api_key_env config")
	}

	value, ok := os.LookupEnv(envVar)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("missing provider API key env %s", envVar)
	}

	return value, nil
}
