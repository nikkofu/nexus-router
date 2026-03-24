package service

import (
	"context"
	"errors"

	"github.com/nikkofu/nexus-router/internal/auth"
	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/capabilities"
	"github.com/nikkofu/nexus-router/internal/orchestrator"
	"github.com/nikkofu/nexus-router/internal/providers"
	"github.com/nikkofu/nexus-router/internal/router"
)

var ErrUnsupportedCapability = capabilities.ErrUnsupportedCapability

type planner interface {
	Plan(publicModel string) (router.Plan, error)
}

type ExecuteService struct {
	capabilities capabilities.Registry
	planner      planner
	executor     providers.Executor
}

func NewExecuteService(registry capabilities.Registry, planner planner, executor providers.Executor) *ExecuteService {
	return &ExecuteService{
		capabilities: registry,
		planner:      planner,
		executor:     executor,
	}
}

func (s *ExecuteService) Execute(ctx context.Context, policy auth.ClientPolicy, req canonical.Request) (providers.Result, []string, error) {
	if err := capabilities.ValidatePublicSurface(req); err != nil {
		return providers.Result{}, nil, err
	}

	if err := capabilities.ValidateRequest(s.capabilities, policy, req); err != nil {
		return providers.Result{}, nil, err
	}

	outcome, err := orchestrator.New(s.planner, s.executor).Run(ctx, req)
	if err != nil {
		var execErr *providers.ExecutionError
		if errors.As(err, &execErr) {
			return providers.Result{}, outcome.Attempts, err
		}

		return providers.Result{}, outcome.Attempts, &providers.ExecutionError{
			Err:             err,
			Retryable:       false,
			OutputCommitted: false,
		}
	}

	return outcome.Result, outcome.Attempts, nil
}
