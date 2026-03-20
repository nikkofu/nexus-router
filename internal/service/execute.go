package service

import (
	"context"

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
	if err := capabilities.ValidatePublicTextOnly(req); err != nil {
		return providers.Result{}, nil, err
	}

	if err := capabilities.ValidateRequest(s.capabilities, policy, req); err != nil {
		return providers.Result{}, nil, err
	}

	outcome, err := orchestrator.New(s.planner, s.executor).Run(ctx, req)
	if err != nil {
		return providers.Result{}, outcome.Attempts, err
	}

	return outcome.Result, outcome.Attempts, nil
}
