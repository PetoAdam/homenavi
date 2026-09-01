package mqtt

import (
	"time"

	"github.com/PetoAdam/homenavi/shared/mqttx"
)

// Client wraps the shared MQTT client for history-service.
type Client struct {
	client *mqttx.Client
}

type Message interface {
	mqttx.Message
}

func Connect(cfg mqttx.Config) (*Client, error) {
	cli, err := mqttx.Connect(mqttx.Options{
		BrokerURL:             cfg.BrokerURL,
		BrokerKind:            cfg.BrokerKind,
		ClientID:              cfg.ClientID,
		ClientIDPrefix:        "history-service",
		AutoReconnect:         true,
		ConnectRetry:          true,
		ConnectRetryInterval:  2 * time.Second,
		KeepAlive:             30 * time.Second,
		PingTimeout:           10 * time.Second,
		InsecureSkipVerifyTLS: true,
	})
	if err != nil {
		return nil, err
	}
	return &Client{client: cli}, nil
}

func (c *Client) Subscribe(topic string, handler func(Message)) error {
	return c.client.SubscribeFuncWithOptions(mqttx.SubscriptionOptions{Topic: topic, QoS: 1}, func(msg mqttx.Message) {
		handler(msg)
	})
}

func (c *Client) SubscribeShared(group, topic string, handler func(Message)) error {
	return c.client.SubscribeFuncWithOptions(mqttx.SubscriptionOptions{Topic: topic, QoS: 1, Mode: mqttx.SubscriptionModeShared, Group: group}, func(msg mqttx.Message) {
		handler(msg)
	})
}

func (c *Client) Close() {
	if c != nil && c.client != nil {
		c.client.Close()
	}
}
