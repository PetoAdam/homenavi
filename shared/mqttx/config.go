package mqttx

import (
	"fmt"
	"strings"

	"github.com/PetoAdam/homenavi/shared/envx"
)

type Config struct {
	BrokerURL string
	ClientID  string
}

func LoadConfig(defaultBrokerURL string, clientIDEnvKeys ...string) Config {
	clientID := ""
	for _, key := range clientIDEnvKeys {
		if value := envx.String(key, ""); value != "" {
			clientID = value
			break
		}
	}
	return Config{
		BrokerURL: envx.String("MQTT_BROKER_URL", defaultBrokerURL),
		ClientID:  clientID,
	}
}

func (cfg Config) Validate() error {
	if strings.TrimSpace(cfg.BrokerURL) == "" {
		return fmt.Errorf("MQTT_BROKER_URL is required")
	}
	return nil
}
