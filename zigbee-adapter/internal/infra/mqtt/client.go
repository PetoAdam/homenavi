package mqtt

import (
	"fmt"

	"github.com/PetoAdam/homenavi/shared/mqttx"
)

type Client = mqttx.Client

type Message = mqttx.Message

type Handler = mqttx.Handler

func Connect(brokerURL, clientIDPrefix string) (*Client, error) {
	cli, err := mqttx.Connect(mqttx.Options{
		BrokerURL:             brokerURL,
		ClientIDPrefix:        clientIDPrefix,
		AutoReconnect:         true,
		ConnectRetry:          true,
		CleanSession:          false,
		ResumeSubs:            true,
		InsecureSkipVerifyTLS: true,
	})
	if err != nil {
		return nil, fmt.Errorf("connect mqtt: %w", err)
	}
	return cli, nil
}
