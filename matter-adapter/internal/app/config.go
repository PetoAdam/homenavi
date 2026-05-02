package app

import (
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/mqttx"
)

// Config holds the bootstrap configuration for matter-adapter.
type Config struct {
	Port                    string
	MQTT                    mqttx.Config
	AdapterID               string
	Version                 string
	Enabled                 bool
	DefaultTimeoutSec       int
	DefaultNetworkPath      string
	CommissioningInterface  string
	EnableBLE               bool
	EnableThread            bool
	EnableOnNetwork         bool
	OTBRBaseURL             string
	OTBRExpectedState       string
	ThreadBorderRouterHost  string
	ThreadBorderRouterPort  int
	ThreadDatasetSource     string
	ThreadOperationalDataset string
	CommissionerEnabled     bool
	CommissionerCommand     string
	CommissionerArgs        []string
	CommissionerTimeout     time.Duration
}

func LoadConfig() Config {
	return Config{
		Port:                     envx.String("MATTER_ADAPTER_PORT", "8096"),
		MQTT:                     mqttx.LoadConfig("mqtt://emqx:1883"),
		AdapterID:                envx.String("MATTER_ADAPTER_ID", "matter-adapter-1"),
		Version:                  envx.String("MATTER_ADAPTER_VERSION", "dev"),
		Enabled:                  envx.Bool("MATTER_ADAPTER_ENABLED", true),
		DefaultTimeoutSec:        envx.Int("MATTER_DEFAULT_PAIRING_TIMEOUT_SEC", 300),
		DefaultNetworkPath:       envx.String("MATTER_DEFAULT_NETWORK_PATH", "on_network"),
		CommissioningInterface:   envx.String("MATTER_COMMISSIONING_INTERFACE", "auto"),
		EnableBLE:                envx.Bool("MATTER_ENABLE_BLE", true),
		EnableThread:             envx.Bool("MATTER_ENABLE_THREAD", true),
		EnableOnNetwork:          envx.Bool("MATTER_ENABLE_ON_NETWORK", true),
		OTBRBaseURL:              envx.String("MATTER_OTBR_BASE_URL", "http://192.168.64.144"),
		OTBRExpectedState:        envx.String("MATTER_OTBR_EXPECTED_STATE", "leader"),
		ThreadBorderRouterHost:   envx.String("MATTER_THREAD_BORDER_ROUTER_HOST", "192.168.64.144"),
		ThreadBorderRouterPort:   envx.Int("MATTER_THREAD_BORDER_ROUTER_PORT", 8080),
		ThreadDatasetSource:      envx.String("MATTER_THREAD_DATASET_SOURCE", "env"),
		ThreadOperationalDataset: envx.String("MATTER_THREAD_OPERATIONAL_DATASET_HEX", "hex:0e080000000000010000000300000f4a0300001a35060004001fffe00208dead00beef00cafe0708fd000db800a00000051000112233445566778899aabbccddeeff030e4f70656e5468726561642d455350010212340410104810e2315100afd6bc9215a6bfac530c0402a0f7f8"),
		CommissionerEnabled:      envx.Bool("MATTER_COMMISSIONER_ENABLED", false),
		CommissionerCommand:      envx.String("MATTER_COMMISSIONER_COMMAND", ""),
		CommissionerArgs:         strings.Fields(envx.String("MATTER_COMMISSIONER_ARGS", "")),
		CommissionerTimeout:      envx.Duration("MATTER_COMMISSIONER_TIMEOUT", 2*time.Minute),
	}
}
