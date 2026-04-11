package mqtt

import (
	"fmt"
	"time"

	"github.com/PetoAdam/homenavi/shared/mqttx"
)

type Client struct {
	cli *mqttx.Client
}

// ClientAPI is the minimal surface area device-hub needs.
type ClientAPI interface {
	Subscribe(topic string, cb Handler) error
	Unsubscribe(topic string) error
	Publish(topic string, payload []byte) error
	PublishWith(topic string, payload []byte, retain bool) error
}

type Message = mqttx.Message

type Handler = mqttx.Handler

func Connect(brokerURL string) (*Client, error) {
	cli, err := mqttx.Connect(mqttx.Options{BrokerURL: brokerURL, ClientIDPrefix: "device-hub", AutoReconnect: true, ConnectRetry: true, ConnectRetryInterval: 2 * time.Second, KeepAlive: 30 * time.Second, PingTimeout: 10 * time.Second, InsecureSkipVerifyTLS: true})
	if err != nil {
		return nil, fmt.Errorf("connect mqtt: %w", err)
	}
	return &Client{cli: cli}, nil
}

func (c *Client) Subscribe(topic string, cb Handler) error {
	return c.cli.Subscribe(topic, cb)
}

func (c *Client) Publish(topic string, payload []byte) error {
	return c.PublishWith(topic, payload, false)
}

func (c *Client) PublishWith(topic string, payload []byte, retain bool) error {
	return c.cli.PublishWithOptions(topic, payload, 0, retain)
}

func (c *Client) Unsubscribe(topic string) error {
	return c.cli.Unsubscribe(topic)
}

func (c *Client) Close() {
	if c != nil && c.cli != nil {
		c.cli.Close()
	}
}
