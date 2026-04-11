package app

import (
	"github.com/PetoAdam/homenavi/shared/envx"
)

// Config holds the bootstrap configuration for thread-adapter.
type Config struct {
	Port          string
	MQTTBrokerURL string
	AdapterID     string
	Version       string
}

func LoadConfig() Config {
	return Config{
		Port:          envx.String("THREAD_ADAPTER_PORT", "8092"),
		MQTTBrokerURL: envx.String("MQTT_BROKER_URL", "mqtt://mosquitto:1883"),
		AdapterID:     envx.String("THREAD_ADAPTER_ID", "thread-adapter-1"),
		Version:       envx.String("THREAD_ADAPTER_VERSION", "dev"),
	}
}
