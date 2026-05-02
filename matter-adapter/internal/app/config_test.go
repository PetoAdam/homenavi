package app

import "testing"

func TestLoadConfig(t *testing.T) {
	t.Setenv("MATTER_ADAPTER_PORT", "9999")
	t.Setenv("MQTT_BROKER_URL", "mqtt://broker:1883")
	t.Setenv("MATTER_ADAPTER_ID", "adapter-x")
	t.Setenv("MATTER_ADAPTER_VERSION", "1.2.3")
	t.Setenv("MATTER_ADAPTER_ENABLED", "true")
	t.Setenv("MATTER_DEFAULT_PAIRING_TIMEOUT_SEC", "321")
	t.Setenv("MATTER_DEFAULT_NETWORK_PATH", "thread")
	t.Setenv("MATTER_COMMISSIONING_INTERFACE", "eth0")
	t.Setenv("MATTER_ENABLE_BLE", "false")
	t.Setenv("MATTER_ENABLE_THREAD", "true")
	t.Setenv("MATTER_ENABLE_ON_NETWORK", "true")
	t.Setenv("MATTER_OTBR_BASE_URL", "http://192.168.64.144")
	t.Setenv("MATTER_OTBR_EXPECTED_STATE", "leader")
	t.Setenv("MATTER_THREAD_BORDER_ROUTER_HOST", "192.168.64.144")
	t.Setenv("MATTER_THREAD_BORDER_ROUTER_PORT", "8080")
	t.Setenv("MATTER_THREAD_DATASET_SOURCE", "env")
	t.Setenv("MATTER_THREAD_OPERATIONAL_DATASET_HEX", "hex:abcd")
	t.Setenv("MATTER_COMMISSIONER_ENABLED", "true")
	t.Setenv("MATTER_COMMISSIONER_COMMAND", "/usr/local/bin/matter-commissioner")
	t.Setenv("MATTER_COMMISSIONER_ARGS", "--driver chip-tool --log-level info")
	t.Setenv("MATTER_COMMISSIONER_TIMEOUT", "45s")

	cfg := LoadConfig()
	if cfg.Port != "9999" || cfg.MQTT.BrokerURL != "mqtt://broker:1883" || cfg.AdapterID != "adapter-x" || cfg.Version != "1.2.3" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if !cfg.Enabled || cfg.DefaultTimeoutSec != 321 || cfg.DefaultNetworkPath != "thread" || cfg.CommissioningInterface != "eth0" {
		t.Fatalf("unexpected matter runtime config: %#v", cfg)
	}
	if cfg.EnableBLE || !cfg.EnableThread || !cfg.EnableOnNetwork {
		t.Fatalf("unexpected capability flags: %#v", cfg)
	}
	if cfg.OTBRBaseURL != "http://192.168.64.144" || cfg.ThreadBorderRouterHost != "192.168.64.144" || cfg.ThreadBorderRouterPort != 8080 {
		t.Fatalf("unexpected OTBR host config: %#v", cfg)
	}
	if cfg.ThreadDatasetSource != "env" || cfg.ThreadOperationalDataset != "hex:abcd" {
		t.Fatalf("unexpected dataset config: %#v", cfg)
	}
	if !cfg.CommissionerEnabled || cfg.CommissionerCommand != "/usr/local/bin/matter-commissioner" || cfg.CommissionerTimeout.Seconds() != 45 {
		t.Fatalf("unexpected commissioner config: %#v", cfg)
	}
	if len(cfg.CommissionerArgs) != 4 || cfg.CommissionerArgs[0] != "--driver" || cfg.CommissionerArgs[1] != "chip-tool" {
		t.Fatalf("unexpected commissioner args: %#v", cfg.CommissionerArgs)
	}
}
