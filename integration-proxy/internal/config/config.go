package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Integrations []IntegrationConfig `yaml:"integrations"`
}

type IntegrationConfig struct {
	ID               string `yaml:"id"`
	Upstream         string `yaml:"upstream"`
	Version          string `yaml:"version,omitempty"`
	AutoUpdate       bool   `yaml:"auto_update,omitempty"`
	DevLatestVersion string `yaml:"dev_latest_version,omitempty"`
	DevComposeFile   string `yaml:"dev_compose_file,omitempty"`
	DevHelmChartRef  string `yaml:"dev_helm_chart_ref,omitempty"`
	DevHelmVersion   string `yaml:"dev_helm_version,omitempty"`
	DevHelmNamespace string `yaml:"dev_helm_namespace,omitempty"`
	ComposeFile      string `yaml:"compose_file,omitempty"`
	HelmReleaseName  string `yaml:"helm_release_name,omitempty"`
	HelmNamespace    string `yaml:"helm_namespace,omitempty"`
	HelmChartRef     string `yaml:"helm_chart_ref,omitempty"`
	HelmChartVersion string `yaml:"helm_chart_version,omitempty"`
	HelmValuesFile   string `yaml:"helm_values_file,omitempty"`
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

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return os.WriteFile(path, b, mode)
	}
	return nil
}
