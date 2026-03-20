package orchestrator

import (
	"context"
	"errors"

	"github.com/nikkofu/nexus-router/internal/canonical"
	"github.com/nikkofu/nexus-router/internal/providers"
	"github.com/nikkofu/nexus-router/internal/router"
)

type planner interface {
	Plan(publicModel string) (router.Plan, error)
}

type Runner struct {
	planner  planner
	executor providers.Executor
}

type Outcome struct {
	Attempts []string
	Result   providers.Result
}

func New(planner planner, executor providers.Executor) *Runner {
	return &Runner{
		planner:  planner,
		executor: executor,
	}
}

func (r *Runner) Run(ctx context.Context, req canonical.Request) (Outcome, error) {
	plan, err := r.planner.Plan(req.PublicModel)
	if err != nil {
		return Outcome{}, err
	}

	outcome := Outcome{
		Attempts: make([]string, 0, len(plan.Attempts)),
	}

	var lastErr error
	for _, attempt := range plan.Attempts {
		outcome.Attempts = append(outcome.Attempts, attempt.Upstream)

		result, err := r.executor.Execute(ctx, attempt.Upstream, req)
		if err == nil {
			outcome.Result = result
			return outcome, nil
		}

		lastErr = err
		execErr, ok := err.(*providers.ExecutionError)
		if !ok {
			return outcome, err
		}
		if execErr.OutputCommitted || !execErr.Retryable {
			return outcome, err
		}
	}

	if lastErr == nil {
		lastErr = errors.New("no route attempts available")
	}

	return outcome, lastErr
}
