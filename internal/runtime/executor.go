package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/providers"
	"github.com/nikkofu/nexus-router/internal/providers/anthropic"
	"github.com/nikkofu/nexus-router/internal/providers/openai"
)

type providerAdapter interface {
	Execute(ctx context.Context, upstream config.ProviderConfig, req canonical.Request) (providers.Result, error)
}

type Executor struct {
	registry  Registry
	openai    providerAdapter
	anthropic providerAdapter
}

func NewExecutor(registry Registry, client *http.Client) *Executor {
	return &Executor{
		registry:  registry,
		openai:    openai.NewAdapter(client),
		anthropic: anthropic.NewAdapter(client),
	}
}

func (e *Executor) Execute(ctx context.Context, upstream string, req canonical.Request) (providers.Result, error) {
	providerConfig, err := e.registry.Resolve(upstream)
	if err != nil {
		return providers.Result{}, fmt.Errorf("runtime executor: resolve upstream %q: %w", upstream, err)
	}

	var adapter providerAdapter
	switch providerConfig.Provider {
	case "openai":
		adapter = e.openai
	case "anthropic":
		adapter = e.anthropic
	default:
		return providers.Result{}, fmt.Errorf("runtime executor: unknown provider %q for upstream %q", providerConfig.Provider, upstream)
	}

	result, err := adapter.Execute(ctx, providerConfig, req)
	if err != nil {
		return providers.Result{}, &providers.ExecutionError{
			Err:             fmt.Errorf("runtime executor: execute provider %q for upstream %q: %w", providerConfig.Provider, upstream, err),
			Retryable:       isRetryableProviderError(err),
			OutputCommitted: false,
		}
	}

	return result, nil
}

func isRetryableProviderError(err error) bool {
	var openaiErr *openai.ClassifiedError
	if errors.As(err, &openaiErr) {
		return openaiErr.Retryable
	}

	var anthropicErr *anthropic.ClassifiedError
	if errors.As(err, &anthropicErr) {
		return anthropicErr.Retryable
	}

	return false
}
