package health

import (
	"testing"
	"time"
)

func TestRuntimeSnapshotContractAtStartup(t *testing.T) {
	now := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	rt := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
		},
		FailureThreshold:         3,
		RecoverySuccessThreshold: 1,
		OpenInterval:             30 * time.Second,
		Now:                      func() time.Time { return now },
	})
	rt.MarkStarted()

	snap := rt.Snapshot()
	if !snap.Started {
		t.Fatal("snapshot.started = false, want true")
	}
	if snap.InitialProbeComplete {
		t.Fatal("snapshot.initial_probe_complete = true, want false")
	}
	if snap.HasEligibleUpstream {
		t.Fatal("snapshot.has_eligible_upstream = true, want false")
	}
	if len(snap.Upstreams) != 1 {
		t.Fatalf("len(snapshot.upstreams) = %d, want 1", len(snap.Upstreams))
	}

	up := snap.Upstreams[0]
	if up.Name != "openai-main" {
		t.Fatalf("upstream.name = %q, want %q", up.Name, "openai-main")
	}
	if up.Provider != "openai" {
		t.Fatalf("upstream.provider = %q, want %q", up.Provider, "openai")
	}
	if up.State != StateUnknown {
		t.Fatalf("upstream.state = %q, want %q", up.State, StateUnknown)
	}
	if up.BreakerState != BreakerStateClosed {
		t.Fatalf("upstream.breaker_state = %q, want %q", up.BreakerState, BreakerStateClosed)
	}
	if up.Source != SourceStartup {
		t.Fatalf("upstream.source = %q, want %q", up.Source, SourceStartup)
	}
	if up.Eligible {
		t.Fatal("upstream.eligible = true, want false")
	}
	if !up.EjectedUntil.IsZero() {
		t.Fatalf("upstream.ejected_until = %v, want zero time", up.EjectedUntil)
	}
}

func TestRuntimeOpensAfterFailureThreshold(t *testing.T) {
	now := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	rt := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
		},
		FailureThreshold:         3,
		RecoverySuccessThreshold: 1,
		OpenInterval:             30 * time.Second,
		Now:                      func() time.Time { return now },
	})

	rt.RecordProbeFailure("openai-main", now, "timeout")
	rt.RecordProbeFailure("openai-main", now, "timeout")
	rt.RecordProbeFailure("openai-main", now, "timeout")

	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	if up.State != StateOpen {
		t.Fatalf("state = %q, want %q", up.State, StateOpen)
	}
	if up.BreakerState != BreakerStateOpen {
		t.Fatalf("breaker_state = %q, want %q", up.BreakerState, BreakerStateOpen)
	}
	if up.Source != SourceProbe {
		t.Fatalf("source = %q, want %q", up.Source, SourceProbe)
	}
	if up.Eligible {
		t.Fatal("eligible = true, want false")
	}
	if up.ConsecutiveFailures != 3 {
		t.Fatalf("consecutive_failures = %d, want 3", up.ConsecutiveFailures)
	}
	wantEjectedUntil := now.Add(30 * time.Second)
	if !up.EjectedUntil.Equal(wantEjectedUntil) {
		t.Fatalf("ejected_until = %v, want %v", up.EjectedUntil, wantEjectedUntil)
	}
}

func TestRuntimeOpenTransitionsToHalfOpenAfterCooldown(t *testing.T) {
	current := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	rt := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
		},
		FailureThreshold:         1,
		RecoverySuccessThreshold: 1,
		OpenInterval:             30 * time.Second,
		Now:                      func() time.Time { return current },
	})

	rt.RecordProbeFailure("openai-main", current, "timeout")
	current = current.Add(31 * time.Second)

	if rt.IsEligible("openai-main") {
		t.Fatal("IsEligible(openai-main) = true, want false in half-open")
	}

	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	if up.State != StateHalfOpen {
		t.Fatalf("state = %q, want %q", up.State, StateHalfOpen)
	}
	if up.BreakerState != BreakerStateHalfOpen {
		t.Fatalf("breaker_state = %q, want %q", up.BreakerState, BreakerStateHalfOpen)
	}
	if !up.EjectedUntil.IsZero() {
		t.Fatalf("ejected_until = %v, want zero time in half-open", up.EjectedUntil)
	}
}

func TestRuntimeHalfOpenRequiresRecoverySuccessThreshold(t *testing.T) {
	current := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	rt := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
		},
		FailureThreshold:         1,
		RecoverySuccessThreshold: 2,
		OpenInterval:             30 * time.Second,
		Now:                      func() time.Time { return current },
	})

	rt.RecordProbeFailure("openai-main", current, "timeout")
	current = current.Add(31 * time.Second)
	_ = rt.Snapshot()

	rt.RecordProbeSuccess("openai-main", current)
	upAfterOne := upstreamByName(t, rt.Snapshot(), "openai-main")
	if upAfterOne.State != StateHalfOpen {
		t.Fatalf("state after 1 half-open success = %q, want %q", upAfterOne.State, StateHalfOpen)
	}
	if upAfterOne.Eligible {
		t.Fatal("eligible after 1 half-open success = true, want false")
	}
	if !upAfterOne.EjectedUntil.IsZero() {
		t.Fatalf("ejected_until after 1 half-open success = %v, want zero time", upAfterOne.EjectedUntil)
	}

	rt.RecordProbeSuccess("openai-main", current)
	upAfterTwo := upstreamByName(t, rt.Snapshot(), "openai-main")
	if upAfterTwo.State != StateHealthy {
		t.Fatalf("state after 2 half-open successes = %q, want %q", upAfterTwo.State, StateHealthy)
	}
	if upAfterTwo.BreakerState != BreakerStateClosed {
		t.Fatalf("breaker_state after 2 half-open successes = %q, want %q", upAfterTwo.BreakerState, BreakerStateClosed)
	}
	if !upAfterTwo.Eligible {
		t.Fatal("eligible after 2 half-open successes = false, want true")
	}
	if upAfterTwo.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive_failures after recovery = %d, want 0", upAfterTwo.ConsecutiveFailures)
	}
	if !upAfterTwo.EjectedUntil.IsZero() {
		t.Fatalf("ejected_until after recovery = %v, want zero time", upAfterTwo.EjectedUntil)
	}
}

func TestRuntimeHalfOpenIgnoresRequestFailureForEjection(t *testing.T) {
	current := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	rt := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
		},
		FailureThreshold:         1,
		RecoverySuccessThreshold: 1,
		OpenInterval:             30 * time.Second,
		Now:                      func() time.Time { return current },
	})

	rt.RecordProbeFailure("openai-main", current, "timeout")
	current = current.Add(31 * time.Second)
	_ = rt.Snapshot() // trigger open -> half_open on read

	before := upstreamByName(t, rt.Snapshot(), "openai-main")
	if before.State != StateHalfOpen {
		t.Fatalf("state before request failure = %q, want %q", before.State, StateHalfOpen)
	}

	rt.RecordRequestFailure("openai-main", current, true, false, "request-timeout")
	after := upstreamByName(t, rt.Snapshot(), "openai-main")
	if after.State != StateHalfOpen {
		t.Fatalf("state after request failure in half_open = %q, want %q", after.State, StateHalfOpen)
	}
	if after.BreakerState != BreakerStateHalfOpen {
		t.Fatalf("breaker_state after request failure in half_open = %q, want %q", after.BreakerState, BreakerStateHalfOpen)
	}
	if !after.EjectedUntil.IsZero() {
		t.Fatalf("ejected_until after request failure in half_open = %v, want zero time", after.EjectedUntil)
	}
	if after.Source != SourceProbe {
		t.Fatalf("source after request failure in half_open = %q, want %q", after.Source, SourceProbe)
	}
}

func TestRuntimeSuccessNoOpDoesNotOverwriteSource(t *testing.T) {
	now := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	rt := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
		},
		FailureThreshold:         2,
		RecoverySuccessThreshold: 1,
		OpenInterval:             30 * time.Second,
		Now:                      func() time.Time { return now },
	})

	rt.RecordProbeSuccess("openai-main", now)
	afterProbeTransition := upstreamByName(t, rt.Snapshot(), "openai-main")
	if afterProbeTransition.Source != SourceProbe {
		t.Fatalf("source after unknown->healthy probe success = %q, want %q", afterProbeTransition.Source, SourceProbe)
	}

	rt.RecordRequestSuccess("openai-main", now)
	afterRequestNoOp := upstreamByName(t, rt.Snapshot(), "openai-main")
	if afterRequestNoOp.State != StateHealthy {
		t.Fatalf("state after healthy->healthy request success = %q, want %q", afterRequestNoOp.State, StateHealthy)
	}
	if afterRequestNoOp.Source != SourceProbe {
		t.Fatalf("source after healthy->healthy request success = %q, want unchanged %q", afterRequestNoOp.Source, SourceProbe)
	}

	rt.RecordProbeSuccess("openai-main", now)
	afterProbeNoOp := upstreamByName(t, rt.Snapshot(), "openai-main")
	if afterProbeNoOp.State != StateHealthy {
		t.Fatalf("state after healthy->healthy probe success = %q, want %q", afterProbeNoOp.State, StateHealthy)
	}
	if afterProbeNoOp.Source != SourceProbe {
		t.Fatalf("source after healthy->healthy probe success = %q, want unchanged %q", afterProbeNoOp.Source, SourceProbe)
	}
}

func TestRuntimeRequestFailuresRespectRetryableAndOutputCommitted(t *testing.T) {
	now := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	rt := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
		},
		FailureThreshold:         2,
		RecoverySuccessThreshold: 1,
		OpenInterval:             30 * time.Second,
		Now:                      func() time.Time { return now },
	})

	rt.RecordProbeSuccess("openai-main", now)

	rt.RecordRequestFailure("openai-main", now, false, false, "bad-request")
	upAfterNonRetryable := upstreamByName(t, rt.Snapshot(), "openai-main")
	if upAfterNonRetryable.State != StateHealthy {
		t.Fatalf("state after non-retryable failure = %q, want %q", upAfterNonRetryable.State, StateHealthy)
	}
	if upAfterNonRetryable.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive_failures after non-retryable failure = %d, want 0", upAfterNonRetryable.ConsecutiveFailures)
	}

	rt.RecordRequestFailure("openai-main", now, true, true, "stream-reset")
	upAfterPostOutput := upstreamByName(t, rt.Snapshot(), "openai-main")
	if upAfterPostOutput.State != StateHealthy {
		t.Fatalf("state after post-output retryable failure = %q, want %q", upAfterPostOutput.State, StateHealthy)
	}
	if upAfterPostOutput.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive_failures after post-output retryable failure = %d, want 0", upAfterPostOutput.ConsecutiveFailures)
	}
	if upAfterPostOutput.Source != SourceProbe {
		t.Fatalf("source after post-output retryable failure = %q, want %q", upAfterPostOutput.Source, SourceProbe)
	}
	if upAfterPostOutput.LastError != "stream-reset" {
		t.Fatalf("last_error after post-output retryable failure = %q, want %q", upAfterPostOutput.LastError, "stream-reset")
	}

	rt.RecordRequestFailure("openai-main", now, true, false, "timeout")
	rt.RecordRequestFailure("openai-main", now, true, false, "timeout")

	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	if up.State != StateOpen {
		t.Fatalf("state = %q, want %q after pre-output retryable failures", up.State, StateOpen)
	}
	if up.BreakerState != BreakerStateOpen {
		t.Fatalf("breaker_state = %q, want %q", up.BreakerState, BreakerStateOpen)
	}
	if up.Source != SourceRequest {
		t.Fatalf("source = %q, want %q", up.Source, SourceRequest)
	}
	if up.Eligible {
		t.Fatal("eligible = true, want false")
	}
	if !up.EjectedUntil.Equal(now.Add(30 * time.Second)) {
		t.Fatalf("ejected_until = %v, want %v", up.EjectedUntil, now.Add(30*time.Second))
	}
}

func upstreamByName(t *testing.T, snap RuntimeSnapshot, name string) UpstreamStatus {
	t.Helper()

	for _, up := range snap.Upstreams {
		if up.Name == name {
			return up
		}
	}

	t.Fatalf("upstream %q not found in snapshot", name)
	return UpstreamStatus{}
}
