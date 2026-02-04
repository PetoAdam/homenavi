package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type MarketplaceConfig struct {
	Integrations []MarketplaceIntegration `yaml:"integrations"`
}

type MarketplaceIntegration struct {
	ID          string `yaml:"id"`
	DisplayName string `yaml:"display_name"`
	Description string `yaml:"description"`
	Icon        string `yaml:"icon"`
	Version     string `yaml:"version"`
	Publisher   string `yaml:"publisher"`
	Homepage    string `yaml:"homepage"`
	Upstream    string `yaml:"upstream"`
	ComposeFile string `yaml:"compose_file"`
	ComposeYAML string `yaml:"compose_yaml"`
}

func LoadMarketplace(path string) (MarketplaceConfig, error) {
	if path == "" {
		return MarketplaceConfig{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return MarketplaceConfig{}, nil
		}
		return MarketplaceConfig{}, err
	}
	var cfg MarketplaceConfig
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return MarketplaceConfig{}, err
	}
	return cfg, nil
}
