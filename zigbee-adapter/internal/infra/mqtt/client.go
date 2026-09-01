package mqtt

import (
	"fmt"

	"github.com/PetoAdam/homenavi/shared/mqttx"
)

type Client struct {
	client *mqttx.Client
}

type Message interface {
	mqttx.Message
}

type Handler func(Message)

func Connect(cfg mqttx.Config, clientIDPrefix string) (*Client, error) {
	cli, err := mqttx.Connect(mqttx.Options{
		BrokerURL:             cfg.BrokerURL,
		BrokerKind:            cfg.BrokerKind,
		ClientID:              cfg.ClientID,
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
	return &Client{client: cli}, nil
}

func (c *Client) Subscribe(topic string, handler Handler) error {
	return c.client.SubscribeWithOptions(mqttx.SubscriptionOptions{Topic: topic, Mode: mqttx.SubscriptionModeExclusive}, func(message mqttx.Message) {
		handler(message)
	})
}

func (c *Client) Publish(topic string, payload []byte) error {
	return c.client.Publish(topic, payload)
}

func (c *Client) PublishWith(topic string, payload []byte, retain bool) error {
	return c.client.PublishWith(topic, payload, retain)
}

func (c *Client) Unsubscribe(topic string) error {
	return c.client.Unsubscribe(topic)
}

func (c *Client) Close() {
	if c != nil && c.client != nil {
		c.client.Close()
	}
}
