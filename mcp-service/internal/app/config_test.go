package app

import "testing"

func TestLoadConfigIgnoresKubernetesServicePort(t *testing.T) {
	t.Setenv("MCP_SERVICE_PORT", "tcp://10.96.0.10:8096")
	t.Setenv("MCP_PORT", "")

	if config := LoadConfig(); config.Port != "8096" {
		t.Fatalf("port = %q, want default listener port", config.Port)
	}
}

func TestLoadConfigUsesMCPPort(t *testing.T) {
	t.Setenv("MCP_PORT", "9010")

	if config := LoadConfig(); config.Port != "9010" {
		t.Fatalf("port = %q, want configured listener port", config.Port)
	}
}
