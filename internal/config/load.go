package config

import (
	"io"

	"gopkg.in/yaml.v3"
)

func Load(r io.Reader) (Config, error) {
	var cfg Config

	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, err
	}

	if err := Validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
