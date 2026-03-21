package health

import (
	"sync"
	"time"
)

type RuntimeUpstream struct {
	Name     string
	Provider string
}

type RuntimeOptions struct {
	Upstreams                []RuntimeUpstream
	FailureThreshold         int
	RecoverySuccessThreshold int
	OpenInterval             time.Duration
	Now                      func() time.Time
}

type Runtime struct {
	mu                       sync.RWMutex
	now                      func() time.Time
	failureThreshold         int
	recoverySuccessThreshold int
	openInterval             time.Duration
	started                  bool
	initialProbeComplete     bool
	order                    []string
	upstreams                map[string]*upstreamState
}

type upstreamState struct {
	name                string
	provider            string
	state               State
	consecutiveFailures int
	ejectedUntil        time.Time
	lastProbeAt         time.Time
	lastProbeOK         bool
	lastError           string
	source              Source
	halfOpenSuccesses   int
	lastRequestEventAt  time.Time
}

func NewRuntime(opts RuntimeOptions) *Runtime {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	failureThreshold := opts.FailureThreshold
	if failureThreshold <= 0 {
		failureThreshold = 1
	}

	recoverySuccessThreshold := opts.RecoverySuccessThreshold
	if recoverySuccessThreshold <= 0 {
		recoverySuccessThreshold = 1
	}

	openInterval := opts.OpenInterval
	if openInterval <= 0 {
		openInterval = 30 * time.Second
	}

	rt := &Runtime{
		now:                      nowFn,
		failureThreshold:         failureThreshold,
		recoverySuccessThreshold: recoverySuccessThreshold,
		openInterval:             openInterval,
		upstreams:                make(map[string]*upstreamState, len(opts.Upstreams)),
	}

	for _, up := range opts.Upstreams {
		if up.Name == "" {
			continue
		}
		if _, exists := rt.upstreams[up.Name]; exists {
			continue
		}
		rt.order = append(rt.order, up.Name)
		rt.upstreams[up.Name] = &upstreamState{
			name:     up.Name,
			provider: up.Provider,
			state:    StateUnknown,
			source:   SourceStartup,
		}
	}

	return rt
}

func (r *Runtime) IsEligible(upstream string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, ok := r.upstreams[upstream]
	if !ok {
		return false
	}
	r.refreshLocked(state)

	return state.state == StateHealthy
}

func (r *Runtime) Snapshot() RuntimeSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	snap := RuntimeSnapshot{
		Started:              r.started,
		InitialProbeComplete: r.initialProbeComplete,
		Upstreams:            make([]UpstreamStatus, 0, len(r.order)),
	}

	for _, name := range r.order {
		state := r.upstreams[name]
		if state == nil {
			continue
		}

		r.refreshLocked(state)
		eligible := state.state == StateHealthy
		if eligible {
			snap.HasEligibleUpstream = true
		}

		ejectedUntil := state.ejectedUntil
		if state.state != StateOpen {
			ejectedUntil = time.Time{}
		}

		snap.Upstreams = append(snap.Upstreams, UpstreamStatus{
			Name:                state.name,
			Provider:            state.provider,
			State:               state.state,
			Eligible:            eligible,
			ConsecutiveFailures: state.consecutiveFailures,
			EjectedUntil:        ejectedUntil,
			LastProbeAt:         state.lastProbeAt,
			LastProbeOK:         state.lastProbeOK,
			LastError:           state.lastError,
			BreakerState:        breakerStateFor(state.state),
			Source:              state.source,
		})
	}

	return snap
}

func (r *Runtime) MarkStarted() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.started = true
}

func (r *Runtime) MarkInitialProbeComplete() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.initialProbeComplete = true
}

func (r *Runtime) RecordProbeSuccess(upstream string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, ok := r.upstreams[upstream]
	if !ok {
		return
	}
	r.refreshLocked(state)

	state.lastProbeAt = at
	state.lastProbeOK = true
	state.lastError = ""

	switch state.state {
	case StateHalfOpen:
		state.halfOpenSuccesses++
		if state.halfOpenSuccesses >= r.recoverySuccessThreshold {
			state.state = StateHealthy
			state.consecutiveFailures = 0
			state.halfOpenSuccesses = 0
			state.ejectedUntil = time.Time{}
			state.source = SourceProbe
		}
	case StateOpen:
		// Stay open until cooldown transitions to half-open.
	default:
		state.consecutiveFailures = 0
		state.halfOpenSuccesses = 0
		state.ejectedUntil = time.Time{}
		if state.state != StateHealthy {
			state.state = StateHealthy
			state.source = SourceProbe
		}
	}
}

func (r *Runtime) RecordProbeFailure(upstream string, at time.Time, errSummary string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, ok := r.upstreams[upstream]
	if !ok {
		return
	}
	r.refreshLocked(state)

	state.lastProbeAt = at
	state.lastProbeOK = false
	state.lastError = errSummary

	switch state.state {
	case StateOpen:
		return
	case StateHalfOpen:
		state.consecutiveFailures++
		state.state = StateOpen
		state.source = SourceProbe
		state.halfOpenSuccesses = 0
		state.ejectedUntil = at.Add(r.openInterval)
		return
	}

	state.consecutiveFailures++
	if state.consecutiveFailures >= r.failureThreshold {
		state.state = StateOpen
		state.source = SourceProbe
		state.halfOpenSuccesses = 0
		state.ejectedUntil = at.Add(r.openInterval)
	}
}

func (r *Runtime) RecordRequestSuccess(upstream string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, ok := r.upstreams[upstream]
	if !ok {
		return
	}
	r.refreshLocked(state)
	if at.Before(state.lastRequestEventAt) {
		return
	}
	state.lastRequestEventAt = at

	switch state.state {
	case StateOpen, StateHalfOpen:
		return
	default:
		state.consecutiveFailures = 0
		state.halfOpenSuccesses = 0
		state.ejectedUntil = time.Time{}
		state.lastError = ""
		if state.state != StateHealthy {
			state.state = StateHealthy
			state.source = SourceRequest
		}
	}
}

func (r *Runtime) RecordRequestFailure(upstream string, at time.Time, retryable bool, outputCommitted bool, errSummary string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, ok := r.upstreams[upstream]
	if !ok {
		return
	}
	r.refreshLocked(state)
	if at.Before(state.lastRequestEventAt) {
		return
	}
	state.lastRequestEventAt = at
	state.lastError = errSummary

	if !retryable || outputCommitted {
		return
	}

	switch state.state {
	case StateOpen:
		return
	case StateHalfOpen:
		return
	}

	state.consecutiveFailures++
	if state.consecutiveFailures >= r.failureThreshold {
		state.state = StateOpen
		state.source = SourceRequest
		state.halfOpenSuccesses = 0
		state.ejectedUntil = at.Add(r.openInterval)
	}
}

func (r *Runtime) refreshLocked(state *upstreamState) {
	if state.state != StateOpen {
		return
	}
	if state.ejectedUntil.IsZero() {
		return
	}
	if r.now().Before(state.ejectedUntil) {
		return
	}

	state.state = StateHalfOpen
	state.ejectedUntil = time.Time{}
	state.halfOpenSuccesses = 0
}

func breakerStateFor(state State) BreakerState {
	switch state {
	case StateOpen:
		return BreakerStateOpen
	case StateHalfOpen:
		return BreakerStateHalfOpen
	default:
		return BreakerStateClosed
	}
}
