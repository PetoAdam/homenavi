package mqtt

import (
	"sync"
	"time"

	"github.com/PetoAdam/homenavi/shared/mqttx"
)

// Client wraps the shared MQTT client for automation-service.
type Client struct {
	client      *mqttx.Client
	onConnectMu sync.RWMutex
	onConnect   []func()
}

type Message interface {
	mqttx.Message
}

func Connect(cfg mqttx.Config) (*Client, error) {
	wrapped := &Client{}
	cli, err := mqttx.Connect(mqttx.Options{
		BrokerURL:             cfg.BrokerURL,
		BrokerKind:            cfg.BrokerKind,
		ClientID:              cfg.ClientID,
		ClientIDPrefix:        "automation-service",
		AutoReconnect:         true,
		ConnectRetry:          true,
		ConnectRetryInterval:  2 * time.Second,
		KeepAlive:             30 * time.Second,
		PingTimeout:           10 * time.Second,
		InsecureSkipVerifyTLS: true,
		OnConnect: func() {
			wrapped.notifyConnected()
		},
	})
	if err != nil {
		return nil, err
	}
	wrapped.client = cli
	return wrapped, nil
}

func (c *Client) AddOnConnectHandler(handler func()) {
	if c == nil || handler == nil {
		return
	}
	c.onConnectMu.Lock()
	c.onConnect = append(c.onConnect, handler)
	c.onConnectMu.Unlock()
}

func (c *Client) notifyConnected() {
	if c == nil {
		return
	}
	c.onConnectMu.RLock()
	handlers := append([]func(){}, c.onConnect...)
	c.onConnectMu.RUnlock()
	for _, handler := range handlers {
		if handler != nil {
			handler()
		}
	}
}

func (c *Client) Subscribe(topic string, handler func(Message)) error {
	return c.SubscribeWithOptions(mqttx.SubscriptionOptions{Topic: topic, QoS: 1}, handler)
}

func (c *Client) SubscribeWithOptions(opts mqttx.SubscriptionOptions, handler func(Message)) error {
	return c.client.SubscribeFuncWithOptions(opts, func(msg mqttx.Message) {
		handler(msg)
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
