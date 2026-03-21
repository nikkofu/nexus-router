package health

import "time"

type State string

const (
	StateUnknown  State = "unknown"
	StateHealthy  State = "healthy"
	StateOpen     State = "open"
	StateHalfOpen State = "half_open"
)

type BreakerState string

const (
	BreakerStateClosed   BreakerState = "closed"
	BreakerStateOpen     BreakerState = "open"
	BreakerStateHalfOpen BreakerState = "half_open"
)

type Source string

const (
	SourceStartup Source = "startup"
	SourceProbe   Source = "probe"
	SourceRequest Source = "request"
)

type RuntimeSnapshot struct {
	Started              bool             `json:"started"`
	InitialProbeComplete bool             `json:"initial_probe_complete"`
	HasEligibleUpstream  bool             `json:"has_eligible_upstream"`
	Upstreams            []UpstreamStatus `json:"upstreams"`
}

type UpstreamStatus struct {
	Name                string       `json:"name"`
	Provider            string       `json:"provider"`
	State               State        `json:"state"`
	Eligible            bool         `json:"eligible"`
	ConsecutiveFailures int          `json:"consecutive_failures"`
	EjectedUntil        time.Time    `json:"ejected_until"`
	LastProbeAt         time.Time    `json:"last_probe_at"`
	LastProbeOK         bool         `json:"last_probe_ok"`
	LastError           string       `json:"last_error"`
	BreakerState        BreakerState `json:"breaker_state"`
	Source              Source       `json:"source"`
}
