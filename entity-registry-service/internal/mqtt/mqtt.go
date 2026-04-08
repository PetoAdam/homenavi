package mqtt

import (
	"time"

	"github.com/PetoAdam/homenavi/shared/mqttx"
)

type Client struct {
	cli *mqttx.Client
}

type Message = mqttx.Message

type Handler = mqttx.Handler

func New(brokerURL string, clientIDPrefix string) *Client {
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
		panic(err)
	}
	return &Client{cli: cli}
}

func (c *Client) Subscribe(topic string, cb Handler) error {
	return c.cli.Subscribe(topic, cb)
}

func (c *Client) Disconnect(quiesceMs uint) {
	if c != nil && c.cli != nil {
		c.cli.Disconnect(quiesceMs)
	}
}
