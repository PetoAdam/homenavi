package mqtt

import (
	"crypto/tls"
	"log/slog"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Client struct {
	client mqtt.Client
}

type Message struct {
	mqtt.Message
}

func (m Message) Retained() bool { return m.Message.Retained() }

func Connect(brokerURL, clientID string) (*Client, error) {
	opts := mqtt.NewClientOptions()
	url := strings.TrimSpace(brokerURL)
	if url == "" {
		url = "mqtt://mosquitto:1883"
	}
	if strings.HasPrefix(url, "mqtt://") {
		url = strings.TrimPrefix(url, "mqtt://")
		url = "tcp://" + url
	}
	opts.AddBroker(url)
	if strings.TrimSpace(clientID) == "" {
		clientID = "history-service-" + time.Now().Format("150405.000")
	}
	opts.SetClientID(clientID)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(2 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	// If a TLS broker is used in the future, tighten this.
	opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})

	opts.OnConnectionLost = func(_ mqtt.Client, err error) {
		slog.Warn("mqtt connection lost", "error", err)
	}
	opts.OnConnect = func(_ mqtt.Client) {
		slog.Info("mqtt connected")
	}

	c := mqtt.NewClient(opts)
	tok := c.Connect()
	if ok := tok.WaitTimeout(15 * time.Second); !ok {
		return nil, tok.Error()
	}
	if err := tok.Error(); err != nil {
		return nil, err
	}
	return &Client{client: c}, nil
}

func (c *Client) Subscribe(topic string, handler func(Message)) error {
	tok := c.client.Subscribe(topic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		handler(Message{Message: msg})
	})
	tok.Wait()
	return tok.Error()
}

func (c *Client) Close() {
	if c == nil || c.client == nil {
		return
	}
	c.client.Disconnect(1000)
}
