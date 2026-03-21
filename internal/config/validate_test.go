package config

import "testing"

func validConfig() Config {
	return Config{
		Auth: AuthConfig{
			ClientKeys: []ClientKeyConfig{
				{
					ID:                   "local",
					Secret:               "secret",
					Active:               true,
					AllowedModelPatterns: []string{"openai/gpt-*"},
				},
			},
		},
		Models: []ModelConfig{
			{Pattern: "openai/gpt-*", RouteGroup: "default"},
		},
		Providers: []ProviderConfig{
			{
				Name:      "openai-main",
				Provider:  "openai",
				BaseURL:   "https://api.openai.com",
				APIKeyEnv: "OPENAI_API_KEY",
			},
		},
		Routing: RoutingConfig{
			RouteGroups: []RouteGroupConfig{
				{Name: "default"},
			},
		},
		Health: HealthConfig{
			ProbeInterval:       "5s",
			ProbeTimeout:        "1s",
			RequireInitialProbe: true,
		},
		Breaker: BreakerConfig{
			FailureThreshold:         3,
			RecoverySuccessThreshold: 2,
			OpenInterval:             "30s",
		},
	}
}

func TestValidateAcceptsProviderProbeOverrides(t *testing.T) {
	cfg := validConfig()
	cfg.Providers[0].Probe = ProbeConfig{
		Method:           "GET",
		Path:             "/healthz",
		Headers:          map[string]string{"X-Probe": "1"},
		ExpectedStatuses: []int{200, 204},
		Interval:         "10s",
		Timeout:          "2s",
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsInvalidProbeMethod(t *testing.T) {
	cfg := validConfig()
	cfg.Providers[0].Probe.Method = "BREW"

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for probe method")
	}
}

func TestValidateRejectsProbePathWithoutLeadingSlash(t *testing.T) {
	cfg := validConfig()
	cfg.Providers[0].Probe.Path = "healthz"

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for probe path")
	}
}

func TestValidateRejectsInvalidExpectedStatuses(t *testing.T) {
	cfg := validConfig()
	cfg.Providers[0].Probe.ExpectedStatuses = []int{200, 999}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for expected statuses")
	}
}

func TestValidateRejectsInvalidHealthDurations(t *testing.T) {
	cfg := validConfig()
	cfg.Health.ProbeInterval = "bad-duration"

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for health.probe_interval")
	}
}

func TestValidateRejectsInvalidHealthProbeTimeout(t *testing.T) {
	cfg := validConfig()
	cfg.Health.ProbeTimeout = "bad-duration"

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for health.probe_timeout")
	}
}

func TestValidateRejectsInvalidProviderProbeDurations(t *testing.T) {
	cfg := validConfig()
	cfg.Providers[0].Probe.Interval = "bad-duration"

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for providers[].probe.interval")
	}
}

func TestValidateRejectsInvalidProviderProbeTimeout(t *testing.T) {
	cfg := validConfig()
	cfg.Providers[0].Probe.Timeout = "bad-duration"

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for providers[].probe.timeout")
	}
}

func TestValidateRejectsNonPositiveHealthProbeInterval(t *testing.T) {
	tests := []string{"0s", "-1s"}
	for _, value := range tests {
		cfg := validConfig()
		cfg.Health.ProbeInterval = value

		err := Validate(cfg)
		if err == nil {
			t.Fatalf("expected validation error for health.probe_interval=%q", value)
		}
	}
}

func TestValidateRejectsNonPositiveHealthProbeTimeout(t *testing.T) {
	tests := []string{"0s", "-1s"}
	for _, value := range tests {
		cfg := validConfig()
		cfg.Health.ProbeTimeout = value

		err := Validate(cfg)
		if err == nil {
			t.Fatalf("expected validation error for health.probe_timeout=%q", value)
		}
	}
}

func TestValidateRejectsNonPositiveProviderProbeInterval(t *testing.T) {
	tests := []string{"0s", "-1s"}
	for _, value := range tests {
		cfg := validConfig()
		cfg.Providers[0].Probe.Interval = value

		err := Validate(cfg)
		if err == nil {
			t.Fatalf("expected validation error for providers[].probe.interval=%q", value)
		}
	}
}

func TestValidateRejectsNonPositiveProviderProbeTimeout(t *testing.T) {
	tests := []string{"0s", "-1s"}
	for _, value := range tests {
		cfg := validConfig()
		cfg.Providers[0].Probe.Timeout = value

		err := Validate(cfg)
		if err == nil {
			t.Fatalf("expected validation error for providers[].probe.timeout=%q", value)
		}
	}
}

func TestValidateRejectsInvalidBreakerOpenInterval(t *testing.T) {
	cfg := validConfig()
	cfg.Breaker.OpenInterval = "bad-duration"

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for breaker.open_interval")
	}
}

func TestValidateRejectsNonPositiveBreakerOpenInterval(t *testing.T) {
	tests := []string{"0s", "-1s"}
	for _, value := range tests {
		cfg := validConfig()
		cfg.Breaker.OpenInterval = value

		err := Validate(cfg)
		if err == nil {
			t.Fatalf("expected validation error for breaker.open_interval=%q", value)
		}
	}
}

func TestValidateRejectsFailureThresholdZero(t *testing.T) {
	cfg := validConfig()
	cfg.Breaker.FailureThreshold = 0

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for failure_threshold")
	}
}

func TestValidateRejectsRecoverySuccessThresholdZero(t *testing.T) {
	cfg := validConfig()
	cfg.Breaker.RecoverySuccessThreshold = 0

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for recovery_success_threshold")
	}
}
