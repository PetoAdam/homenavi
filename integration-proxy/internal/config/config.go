package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Integrations []IntegrationConfig `yaml:"integrations"`
}

type IntegrationConfig struct {
	ID       string `yaml:"id"`
	Upstream string `yaml:"upstream"`
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}