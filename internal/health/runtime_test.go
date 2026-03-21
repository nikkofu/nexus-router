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
	if up.ConsecutiveFailures != 0 {
		t.Fatalf("upstream.consecutive_failures = %d, want 0", up.ConsecutiveFailures)
	}
	if !up.LastProbeAt.IsZero() {
		t.Fatalf("upstream.last_probe_at = %v, want zero time", up.LastProbeAt)
	}
	if up.LastProbeOK {
		t.Fatal("upstream.last_probe_ok = true, want false")
	}
	if up.LastError != "" {
		t.Fatalf("upstream.last_error = %q, want empty string", up.LastError)
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
	rt.RecordProbeFailure("openai-main", now.Add(1*time.Second), "timeout")
	rt.RecordProbeFailure("openai-main", now.Add(2*time.Second), "timeout")

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
	wantEjectedUntil := now.Add(2 * time.Second).Add(30 * time.Second)
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

	rt.RecordProbeSuccess("openai-main", current.Add(1*time.Second))
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

	rt.RecordRequestFailure("openai-main", now.Add(1*time.Second), false, false, "bad-request")
	upAfterNonRetryable := upstreamByName(t, rt.Snapshot(), "openai-main")
	if upAfterNonRetryable.State != StateHealthy {
		t.Fatalf("state after non-retryable failure = %q, want %q", upAfterNonRetryable.State, StateHealthy)
	}
	if upAfterNonRetryable.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive_failures after non-retryable failure = %d, want 0", upAfterNonRetryable.ConsecutiveFailures)
	}

	rt.RecordRequestFailure("openai-main", now.Add(2*time.Second), true, true, "stream-reset")
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

	rt.RecordRequestFailure("openai-main", now.Add(3*time.Second), true, false, "timeout")
	rt.RecordRequestFailure("openai-main", now.Add(4*time.Second), true, false, "timeout")

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
	wantEjectedUntil := now.Add(4 * time.Second).Add(30 * time.Second)
	if !up.EjectedUntil.Equal(wantEjectedUntil) {
		t.Fatalf("ejected_until = %v, want %v", up.EjectedUntil, wantEjectedUntil)
	}
}

func TestRuntimeOpenEjectionUsesEventTimestampForProbeFailures(t *testing.T) {
	now := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	eventAt := now.Add(-10 * time.Second)
	rt := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
		},
		FailureThreshold:         1,
		RecoverySuccessThreshold: 1,
		OpenInterval:             30 * time.Second,
		Now:                      func() time.Time { return now },
	})

	rt.RecordProbeFailure("openai-main", eventAt, "timeout")

	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	wantEjectedUntil := eventAt.Add(30 * time.Second)
	if !up.EjectedUntil.Equal(wantEjectedUntil) {
		t.Fatalf("ejected_until = %v, want %v (anchored to event timestamp)", up.EjectedUntil, wantEjectedUntil)
	}
}

func TestRuntimeOpenEjectionUsesEventTimestampForRequestFailures(t *testing.T) {
	now := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	eventAt := now.Add(-10 * time.Second)
	rt := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
		},
		FailureThreshold:         1,
		RecoverySuccessThreshold: 1,
		OpenInterval:             30 * time.Second,
		Now:                      func() time.Time { return now },
	})

	rt.RecordRequestFailure("openai-main", eventAt, true, false, "timeout")

	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	wantEjectedUntil := eventAt.Add(30 * time.Second)
	if !up.EjectedUntil.Equal(wantEjectedUntil) {
		t.Fatalf("ejected_until = %v, want %v (anchored to event timestamp)", up.EjectedUntil, wantEjectedUntil)
	}
}

func TestRuntimeUnknownWriteEventsDoNotCreateUpstreamEntries(t *testing.T) {
	now := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	rt := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
		},
		FailureThreshold:         1,
		RecoverySuccessThreshold: 1,
		OpenInterval:             30 * time.Second,
		Now:                      func() time.Time { return now },
	})

	rt.RecordProbeFailure("ghost-upstream", now, "timeout")
	rt.RecordProbeSuccess("ghost-upstream", now)
	rt.RecordRequestFailure("ghost-upstream", now, true, false, "timeout")
	rt.RecordRequestSuccess("ghost-upstream", now)

	snap := rt.Snapshot()
	if len(snap.Upstreams) != 1 {
		t.Fatalf("len(snapshot.upstreams) = %d, want 1 (configured upstreams only)", len(snap.Upstreams))
	}
	if snap.Upstreams[0].Name != "openai-main" {
		t.Fatalf("snapshot.upstreams[0].name = %q, want %q", snap.Upstreams[0].Name, "openai-main")
	}
}

func TestRuntimeWritePathRefreshesOpenToHalfOpenWithoutSnapshotRead(t *testing.T) {
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

	// This write should refresh open -> half_open internally first, then apply the half-open success.
	rt.RecordProbeSuccess("openai-main", current)

	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	if up.State != StateHalfOpen {
		t.Fatalf("state after post-cooldown probe success write = %q, want %q", up.State, StateHalfOpen)
	}
	if up.BreakerState != BreakerStateHalfOpen {
		t.Fatalf("breaker_state after post-cooldown probe success write = %q, want %q", up.BreakerState, BreakerStateHalfOpen)
	}
	if up.Eligible {
		t.Fatal("eligible after first half-open success = true, want false")
	}
}

func TestRuntimeIgnoresOlderRequestSuccessAfterNewerRequestFailure(t *testing.T) {
	base := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	now := base.Add(5 * time.Second)
	rt := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
		},
		FailureThreshold:         1,
		RecoverySuccessThreshold: 1,
		OpenInterval:             30 * time.Second,
		Now:                      func() time.Time { return now },
	})

	rt.RecordProbeSuccess("openai-main", base)
	rt.RecordRequestFailure("openai-main", now, true, false, "newer-timeout")
	rt.RecordRequestSuccess("openai-main", base.Add(1*time.Second))

	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	if up.State != StateOpen {
		t.Fatalf("state after older request success = %q, want %q", up.State, StateOpen)
	}
	if up.Source != SourceRequest {
		t.Fatalf("source after older request success = %q, want %q", up.Source, SourceRequest)
	}
	if up.ConsecutiveFailures != 1 {
		t.Fatalf("consecutive_failures after older request success = %d, want 1", up.ConsecutiveFailures)
	}
	if up.LastError != "newer-timeout" {
		t.Fatalf("last_error after older request success = %q, want %q", up.LastError, "newer-timeout")
	}
}

func TestRuntimeIgnoresOlderRequestFailureAfterNewerRequestSuccess(t *testing.T) {
	base := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	now := base.Add(5 * time.Second)
	rt := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
		},
		FailureThreshold:         1,
		RecoverySuccessThreshold: 1,
		OpenInterval:             30 * time.Second,
		Now:                      func() time.Time { return now },
	})

	rt.RecordProbeSuccess("openai-main", base)
	rt.RecordRequestSuccess("openai-main", now)
	rt.RecordRequestFailure("openai-main", base.Add(1*time.Second), true, false, "older-timeout")

	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	if up.State != StateHealthy {
		t.Fatalf("state after older request failure = %q, want %q", up.State, StateHealthy)
	}
	if up.BreakerState != BreakerStateClosed {
		t.Fatalf("breaker_state after older request failure = %q, want %q", up.BreakerState, BreakerStateClosed)
	}
	if up.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive_failures after older request failure = %d, want 0", up.ConsecutiveFailures)
	}
	if up.LastError != "" {
		t.Fatalf("last_error after older request failure = %q, want empty string", up.LastError)
	}
}

func TestRuntimeIgnoresOlderRequestSuccessAfterNewerProbeFailure(t *testing.T) {
	base := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	rt := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
		},
		FailureThreshold:         1,
		RecoverySuccessThreshold: 1,
		OpenInterval:             30 * time.Second,
		Now:                      func() time.Time { return base },
	})

	rt.RecordProbeSuccess("openai-main", base.Add(1*time.Second))
	rt.RecordProbeFailure("openai-main", base.Add(3*time.Second), "probe-timeout")
	rt.RecordRequestSuccess("openai-main", base.Add(2*time.Second))

	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	if up.State != StateOpen {
		t.Fatalf("state after older request success following newer probe failure = %q, want %q", up.State, StateOpen)
	}
	if up.Source != SourceProbe {
		t.Fatalf("source after older request success following newer probe failure = %q, want %q", up.Source, SourceProbe)
	}
	if up.LastError != "probe-timeout" {
		t.Fatalf("last_error after older request success following newer probe failure = %q, want %q", up.LastError, "probe-timeout")
	}
}

func TestRuntimeIgnoresOlderRequestFailureAfterNewerProbeSuccess(t *testing.T) {
	base := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	rt := NewRuntime(RuntimeOptions{
		Upstreams: []RuntimeUpstream{
			{Name: "openai-main", Provider: "openai"},
		},
		FailureThreshold:         1,
		RecoverySuccessThreshold: 1,
		OpenInterval:             30 * time.Second,
		Now:                      func() time.Time { return base },
	})

	rt.RecordProbeSuccess("openai-main", base.Add(3*time.Second))
	rt.RecordRequestFailure("openai-main", base.Add(2*time.Second), true, false, "older-timeout")

	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	if up.State != StateHealthy {
		t.Fatalf("state after older request failure following newer probe success = %q, want %q", up.State, StateHealthy)
	}
	if up.Source != SourceProbe {
		t.Fatalf("source after older request failure following newer probe success = %q, want %q", up.Source, SourceProbe)
	}
	if up.LastError != "" {
		t.Fatalf("last_error after older request failure following newer probe success = %q, want empty string", up.LastError)
	}
}

func TestRuntimeStaleProbeSuccessAfterCooldownDoesNotActAsHalfOpen(t *testing.T) {
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

	openAt := current
	rt.RecordProbeFailure("openai-main", openAt, "timeout")

	// Result arrives after wall-clock cooldown, but its event time is still inside open window.
	current = openAt.Add(40 * time.Second)
	rt.RecordProbeSuccess("openai-main", openAt.Add(10*time.Second))

	// Freeze read time before ejected-until so Snapshot() doesn't do read-path refresh.
	current = openAt.Add(20 * time.Second)
	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	if up.State != StateOpen {
		t.Fatalf("state after stale probe success delivery = %q, want %q", up.State, StateOpen)
	}
	wantEjectedUntil := openAt.Add(30 * time.Second)
	if !up.EjectedUntil.Equal(wantEjectedUntil) {
		t.Fatalf("ejected_until after stale probe success delivery = %v, want unchanged %v", up.EjectedUntil, wantEjectedUntil)
	}
}

func TestRuntimeStaleProbeFailureAfterCooldownDoesNotActAsHalfOpen(t *testing.T) {
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

	openAt := current
	rt.RecordProbeFailure("openai-main", openAt, "timeout")

	// Result arrives after wall-clock cooldown, but its event time is still inside open window.
	current = openAt.Add(40 * time.Second)
	rt.RecordProbeFailure("openai-main", openAt.Add(10*time.Second), "late-timeout")

	// Freeze read time before ejected-until so Snapshot() doesn't do read-path refresh.
	current = openAt.Add(20 * time.Second)
	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	if up.State != StateOpen {
		t.Fatalf("state after stale probe failure delivery = %q, want %q", up.State, StateOpen)
	}
	wantEjectedUntil := openAt.Add(30 * time.Second)
	if !up.EjectedUntil.Equal(wantEjectedUntil) {
		t.Fatalf("ejected_until after stale probe failure delivery = %v, want unchanged %v", up.EjectedUntil, wantEjectedUntil)
	}
}

func TestRuntimeStaleProbeSuccessAfterReadHalfOpenTransitionIsIgnored(t *testing.T) {
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

	openAt := current
	rt.RecordProbeFailure("openai-main", openAt, "timeout")

	// Read-path transition to half_open by wall clock.
	current = openAt.Add(40 * time.Second)
	if rt.IsEligible("openai-main") {
		t.Fatal("IsEligible(openai-main) = true, want false in half_open")
	}

	// Late event is still inside original open window.
	rt.RecordProbeSuccess("openai-main", openAt.Add(10*time.Second))

	// Freeze read-time before original boundary so no further read refresh effects.
	current = openAt.Add(20 * time.Second)
	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	if up.State != StateHalfOpen {
		t.Fatalf("state after stale probe success post-read-transition = %q, want %q", up.State, StateHalfOpen)
	}
	if up.Eligible {
		t.Fatal("eligible after stale probe success post-read-transition = true, want false")
	}
}

func TestRuntimeStaleProbeFailureAfterReadHalfOpenTransitionIsIgnored(t *testing.T) {
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

	openAt := current
	rt.RecordProbeFailure("openai-main", openAt, "timeout")

	// Read-path transition to half_open by wall clock.
	current = openAt.Add(40 * time.Second)
	_ = rt.Snapshot()

	// Late event is still inside original open window.
	rt.RecordProbeFailure("openai-main", openAt.Add(10*time.Second), "late-timeout")

	// Freeze read-time before original boundary so no further read refresh effects.
	current = openAt.Add(20 * time.Second)
	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	if up.State != StateHalfOpen {
		t.Fatalf("state after stale probe failure post-read-transition = %q, want %q", up.State, StateHalfOpen)
	}
	if up.Source != SourceProbe {
		t.Fatalf("source after stale probe failure post-read-transition = %q, want %q", up.Source, SourceProbe)
	}
}

func TestRuntimeSameTimestampEventsAreDeterministicAndDoNotDoubleCount(t *testing.T) {
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
	// Same-timestamp conflicts should be ignored after first applied event.
	rt.RecordRequestFailure("openai-main", now, true, false, "same-time-failure")
	rt.RecordRequestSuccess("openai-main", now)
	rt.RecordRequestFailure("openai-main", now, true, false, "same-time-failure-2")

	up := upstreamByName(t, rt.Snapshot(), "openai-main")
	if up.State != StateHealthy {
		t.Fatalf("state after same-timestamp events = %q, want %q", up.State, StateHealthy)
	}
	if up.ConsecutiveFailures != 0 {
		t.Fatalf("consecutive_failures after same-timestamp events = %d, want 0", up.ConsecutiveFailures)
	}
	if up.Source != SourceProbe {
		t.Fatalf("source after same-timestamp events = %q, want %q", up.Source, SourceProbe)
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
