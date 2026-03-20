package router

import (
	"fmt"

	"github.com/nikkofu/nexus-router/internal/config"
)

type Eligibility interface {
	IsEligible(upstream string) bool
}

type Planner struct {
	cfg         config.Config
	eligibility Eligibility
}

type Plan struct {
	Attempts []Attempt
}

type Attempt struct {
	Upstream string
}

func NewPlanner(cfg config.Config, eligibility Eligibility) Planner {
	return Planner{
		cfg:         cfg,
		eligibility: eligibility,
	}
}

func (p Planner) Plan(publicModel string) (Plan, error) {
	model, ok := matchModel(publicModel, p.cfg.Models)
	if !ok {
		return Plan{}, fmt.Errorf("no route configured for model %q", publicModel)
	}

	group, ok := findRouteGroup(model.RouteGroup, p.cfg.Routing.RouteGroups)
	if !ok {
		return Plan{}, fmt.Errorf("route group %q not found", model.RouteGroup)
	}

	knownProviders := make(map[string]struct{}, len(p.cfg.Providers))
	for _, provider := range p.cfg.Providers {
		knownProviders[provider.Name] = struct{}{}
	}

	candidates := append([]string{group.Primary}, group.Fallbacks...)
	attempts := make([]Attempt, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := knownProviders[candidate]; !ok {
			continue
		}
		if p.eligibility != nil && !p.eligibility.IsEligible(candidate) {
			continue
		}

		attempts = append(attempts, Attempt{Upstream: candidate})
	}

	return Plan{Attempts: attempts}, nil
}
