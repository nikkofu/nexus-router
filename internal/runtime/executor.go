package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/config"
	"github.com/nikkofu/nexus-router/internal/providers"
	"github.com/nikkofu/nexus-router/internal/providers/anthropic"
	"github.com/nikkofu/nexus-router/internal/providers/openai"
)

type providerAdapter interface {
	Execute(ctx context.Context, upstream config.ProviderConfig, req canonical.Request) (providers.Result, error)
}

type requestOutcomeWriter interface {
	RecordRequestSuccess(upstream string, at time.Time)
	RecordRequestFailure(upstream string, at time.Time, retryable bool, outputCommitted bool, errSummary string)
}

type Executor struct {
	registry  Registry
	openai    providerAdapter
	anthropic providerAdapter
	runtime   requestOutcomeWriter
	now       func() time.Time
}

func NewExecutor(registry Registry, client *http.Client, runtime requestOutcomeWriter) *Executor {
	return &Executor{
		registry:  registry,
		openai:    openai.NewAdapter(client),
		anthropic: anthropic.NewAdapter(client),
		runtime:   runtime,
		now:       time.Now,
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
		execErr := wrapExecutionError(providerConfig, upstream, err)
		if e.runtime != nil {
			e.runtime.RecordRequestFailure(upstream, e.now(), execErr.Retryable, execErr.OutputCommitted, execErr.Error())
		}

		return providers.Result{}, execErr
	}

	if e.runtime != nil {
		e.runtime.RecordRequestSuccess(upstream, e.now())
	}

	return result, nil
}

func wrapExecutionError(providerConfig config.ProviderConfig, upstream string, err error) *providers.ExecutionError {
	var execErr *providers.ExecutionError
	if errors.As(err, &execErr) {
		return &providers.ExecutionError{
			Err:             fmt.Errorf("runtime executor: execute provider %q for upstream %q: %w", providerConfig.Provider, upstream, execErr.Err),
			Retryable:       execErr.Retryable,
			OutputCommitted: execErr.OutputCommitted,
		}
	}

	return &providers.ExecutionError{
		Err:             fmt.Errorf("runtime executor: execute provider %q for upstream %q: %w", providerConfig.Provider, upstream, err),
		Retryable:       isRetryableProviderError(err),
		OutputCommitted: false,
	}
}

func isRetryableProviderError(err error) bool {
	var execErr *providers.ExecutionError
	if errors.As(err, &execErr) {
		return execErr.Retryable
	}

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
