package config

import (
	"bytes"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

type runtimeHealthDefaultsPresence struct {
	Health struct {
		ProbeInterval       *string `yaml:"probe_interval"`
		ProbeTimeout        *string `yaml:"probe_timeout"`
		RequireInitialProbe *bool   `yaml:"require_initial_probe"`
	} `yaml:"health"`
	Breaker struct {
		FailureThreshold         *int    `yaml:"failure_threshold"`
		OpenInterval             *string `yaml:"open_interval"`
		RecoverySuccessThreshold *int    `yaml:"recovery_success_threshold"`
	} `yaml:"breaker"`
	Providers []struct {
		Probe struct {
			Interval *string `yaml:"interval"`
			Timeout  *string `yaml:"timeout"`
		} `yaml:"probe"`
	} `yaml:"providers"`
}

func Load(r io.Reader) (Config, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Config{}, err
	}
	if err := validateExplicitRuntimeHealthNullOverrides(data); err != nil {
		return Config{}, err
	}

	var cfg Config
	var presence runtimeHealthDefaultsPresence

	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&presence); err != nil {
		return Config{}, err
	}

	dec = yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, err
	}

	if err := validateExplicitRuntimeHealthDurationOverrides(presence); err != nil {
		return Config{}, err
	}

	applyRuntimeHealthDefaults(&cfg, presence)

	if err := Validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func applyRuntimeHealthDefaults(cfg *Config, presence runtimeHealthDefaultsPresence) {
	if presence.Health.ProbeInterval == nil {
		cfg.Health.ProbeInterval = "15s"
	}
	if presence.Health.ProbeTimeout == nil {
		cfg.Health.ProbeTimeout = "2s"
	}
	if presence.Health.RequireInitialProbe == nil {
		cfg.Health.RequireInitialProbe = true
	}

	if presence.Breaker.FailureThreshold == nil {
		cfg.Breaker.FailureThreshold = 3
	}
	if presence.Breaker.OpenInterval == nil {
		cfg.Breaker.OpenInterval = "30s"
	}
	if presence.Breaker.RecoverySuccessThreshold == nil {
		cfg.Breaker.RecoverySuccessThreshold = 1
	}
}

func validateExplicitRuntimeHealthDurationOverrides(presence runtimeHealthDefaultsPresence) error {
	if presence.Health.ProbeInterval != nil && *presence.Health.ProbeInterval == "" {
		return fmt.Errorf("health.probe_interval must not be empty when explicitly set")
	}
	if presence.Health.ProbeTimeout != nil && *presence.Health.ProbeTimeout == "" {
		return fmt.Errorf("health.probe_timeout must not be empty when explicitly set")
	}
	if presence.Breaker.OpenInterval != nil && *presence.Breaker.OpenInterval == "" {
		return fmt.Errorf("breaker.open_interval must not be empty when explicitly set")
	}
	for i, provider := range presence.Providers {
		if provider.Probe.Interval != nil && *provider.Probe.Interval == "" {
			return fmt.Errorf("providers[%d].probe.interval must not be empty when explicitly set", i)
		}
		if provider.Probe.Timeout != nil && *provider.Probe.Timeout == "" {
			return fmt.Errorf("providers[%d].probe.timeout must not be empty when explicitly set", i)
		}
	}
	return nil
}

func validateExplicitRuntimeHealthNullOverrides(data []byte) error {
	var root yaml.Node
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&root); err != nil {
		return err
	}
	if len(root.Content) == 0 {
		return nil
	}

	top := root.Content[0]
	health := mappingValue(top, "health")
	if isNullScalar(mappingValue(health, "probe_interval")) {
		return fmt.Errorf("health.probe_interval must not be null when explicitly set")
	}
	if isNullScalar(mappingValue(health, "probe_timeout")) {
		return fmt.Errorf("health.probe_timeout must not be null when explicitly set")
	}

	breaker := mappingValue(top, "breaker")
	if isNullScalar(mappingValue(breaker, "open_interval")) {
		return fmt.Errorf("breaker.open_interval must not be null when explicitly set")
	}

	providers := mappingValue(top, "providers")
	if providers != nil && providers.Kind == yaml.SequenceNode {
		for i, provider := range providers.Content {
			probe := mappingValue(provider, "probe")
			if isNullScalar(mappingValue(probe, "interval")) {
				return fmt.Errorf("providers[%d].probe.interval must not be null when explicitly set", i)
			}
			if isNullScalar(mappingValue(probe, "timeout")) {
				return fmt.Errorf("providers[%d].probe.timeout must not be null when explicitly set", i)
			}
		}
	}

	return nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func isNullScalar(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.Tag == "!!null"
}
