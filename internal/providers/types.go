package providers

import (
	"context"

	"github.com/nikkofu/nexus-router/internal/canonical"
)

type Result struct {
	Events []canonical.Event
}

type Executor interface {
	Execute(ctx context.Context, upstream string, req canonical.Request) (Result, error)
}

type ExecutionError struct {
	Err             error
	Retryable       bool
	OutputCommitted bool
}

func (e *ExecutionError) Error() string {
	if e.Err == nil {
		return "provider execution error"
	}

	return e.Err.Error()
}
