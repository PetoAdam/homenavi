package mqtt

import (
	"fmt"
	"time"

	"github.com/PetoAdam/homenavi/shared/mqttx"
)

type Client struct {
	cli *mqttx.Client
}

type Message = mqttx.Message

func Connect(brokerURL string, clientIDPrefix string) (*Client, error) {
	cli, err := mqttx.Connect(mqttx.Options{
		BrokerURL:             brokerURL,
		ClientIDPrefix:        clientIDPrefix,
		AutoReconnect:         true,
		ConnectRetry:          true,
		ConnectRetryInterval:  2 * time.Second,
		KeepAlive:             30 * time.Second,
		PingTimeout:           10 * time.Second,
		InsecureSkipVerifyTLS: true,
	})
	if err != nil {
		return nil, fmt.Errorf("connect mqtt: %w", err)
	}
	return &Client{cli: cli}, nil
}

func (c *Client) Subscribe(topic string, cb func(Message)) error {
	return c.cli.SubscribeFunc(topic, cb)
}

func (c *Client) Close() {
	if c != nil && c.cli != nil {
		c.cli.Disconnect(250)
	}
}
