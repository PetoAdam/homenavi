package mqtt

import (
	"crypto/tls"
	"log/slog"
	"net/url"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

type Client struct {
	cli paho.Client
}

type Message = paho.Message

type Handler = paho.MessageHandler

func New(brokerURL string, clientIDPrefix string) *Client {
	u, err := url.Parse(brokerURL)
	if err != nil {
		panic(err)
	}
	opts := paho.NewClientOptions()
	server := u.Host
	if u.Scheme == "mqtt" || u.Scheme == "tcp" {
		server = "tcp://" + server
	} else if u.Scheme == "ssl" || u.Scheme == "tls" {
		server = "ssl://" + server
	} else if u.Scheme == "ws" || u.Scheme == "wss" {
		server = u.Scheme + "://" + server + u.Path
	}
	opts.AddBroker(server)
	prefix := clientIDPrefix
	if prefix == "" {
		prefix = "entity-registry"
	}
	opts.SetClientID(prefix + "-" + time.Now().Format("150405.000"))
	opts.OnConnect = func(c paho.Client) { slog.Info("mqtt connected", "broker", brokerURL) }
	opts.OnConnectionLost = func(c paho.Client, err error) { slog.Error("mqtt connection lost", "error", err) }
	if u.User != nil {
		pw, _ := u.User.Password()
		opts.SetUsername(u.User.Username())
		opts.SetPassword(pw)
	}
	if u.Scheme == "ssl" || u.Scheme == "tls" || u.Scheme == "wss" {
		opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	}

	cli := paho.NewClient(opts)
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

func (c *Client) Disconnect(quiesceMs uint) {
	if c == nil || c.cli == nil {
		return
	}
	c.cli.Disconnect(quiesceMs)
}
