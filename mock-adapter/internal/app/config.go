package app

import (
	"github.com/PetoAdam/homenavi/shared/envx"
	"github.com/PetoAdam/homenavi/shared/mqttx"
)

// Config holds the bootstrap configuration for mock-adapter.
type Config struct {
	Port      string
	MQTT      mqttx.Config
	AdapterID string
	Version   string
}

func LoadConfig() Config {
	return Config{
		Port:      envx.String("MOCK_ADAPTER_PORT", "8092"),
		MQTT:      mqttx.LoadConfig("mqtt://emqx:1883"),
		AdapterID: envx.String("MOCK_ADAPTER_ID", "mock-adapter-1"),
		Version:   envx.String("MOCK_ADAPTER_VERSION", "dev"),
	}
}
