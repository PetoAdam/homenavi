package mqtt

import (
	"time"

	"github.com/PetoAdam/homenavi/shared/mqttx"
)

// Client wraps the shared MQTT client for automation-service.
type Client struct {
	client *mqttx.Client
}

type Message struct {
	mqttx.Message
}

func (m Message) Retained() bool { return m.Message.Retained() }

func Connect(brokerURL, clientID string) (*Client, error) {
	cli, err := mqttx.Connect(mqttx.Options{
		BrokerURL:             brokerURL,
		ClientID:              clientID,
		ClientIDPrefix:        "automation-service",
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
	return c.client.SubscribeFuncWithQoS(topic, 1, func(msg mqttx.Message) {
		handler(Message{Message: msg})
	})
}

func (c *Client) Publish(topic string, payload []byte) error {
	return c.client.PublishWithOptions(topic, payload, 1, false)
}

func (c *Client) Close() {
	if c != nil && c.client != nil {
		c.client.Close()
	}
}
