package mqtt

import (
	"crypto/tls"
	"log/slog"
	"net/url"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Client struct {
	cli mqtt.Client
}

// ClientAPI is the minimal surface area device-hub needs.
// It enables unit testing HTTP handlers without requiring a live broker.
type ClientAPI interface {
	Subscribe(topic string, cb Handler) error
	Unsubscribe(topic string) error
	Publish(topic string, payload []byte) error
	PublishWith(topic string, payload []byte, retain bool) error
}

// Message is re-exported type for handlers
type Message = mqtt.Message

// Handler is handler signature
type Handler = mqtt.MessageHandler

func New(brokerURL string) *Client {
	u, err := url.Parse(brokerURL)
	if err != nil {
		panic(err)
	}
	opts := mqtt.NewClientOptions()
	server := u.Host
	if u.Scheme == "mqtt" || u.Scheme == "tcp" {
		server = "tcp://" + server
	} else if u.Scheme == "ssl" || u.Scheme == "tls" {
		server = "ssl://" + server
	} else if u.Scheme == "ws" || u.Scheme == "wss" {
		server = u.Scheme + "://" + server + u.Path
	}
	opts.AddBroker(server)
	opts.SetClientID("device-hub-" + time.Now().Format("150405.000"))
	opts.OnConnect = func(c mqtt.Client) { slog.Info("mqtt connected", "broker", brokerURL) }
	opts.OnConnectionLost = func(c mqtt.Client, err error) { slog.Error("mqtt connection lost", "error", err) }
	if u.User != nil {
		pw, _ := u.User.Password()
		opts.SetUsername(u.User.Username())
		opts.SetPassword(pw)
	}
	if u.Scheme == "ssl" || u.Scheme == "tls" || u.Scheme == "wss" {
		opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true}) // TODO: tighten
	}
	cli := mqtt.NewClient(opts)
	if t := cli.Connect(); t.Wait() && t.Error() != nil {
		panic(t.Error())
	}
	return &Client{cli: cli}
}

func (c *Client) Subscribe(topic string, cb Handler) error {
	t := c.cli.Subscribe(topic, 0, cb)
	if t.Wait() && t.Error() != nil {
		return t.Error()
	}
	slog.Info("mqtt subscribed", "topic", topic)
	return nil
}

func (c *Client) Publish(topic string, payload []byte) error {
	return c.PublishWith(topic, payload, false)
}

func (c *Client) PublishWith(topic string, payload []byte, retain bool) error {
	t := c.cli.Publish(topic, 0, retain, payload)
	if t.Wait() && t.Error() != nil {
		return t.Error()
	}
	return nil
}

func (c *Client) Unsubscribe(topic string) error {
	t := c.cli.Unsubscribe(topic)
	if t.Wait() && t.Error() != nil {
		return t.Error()
	}
	slog.Info("mqtt unsubscribed", "topic", topic)
	return nil
}
