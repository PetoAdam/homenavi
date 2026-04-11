package mqttx

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Message = mqtt.Message

type Handler = mqtt.MessageHandler

type Client struct {
	cli mqtt.Client
}

type Options struct {
	BrokerURL             string
	ClientID              string
	ClientIDPrefix        string
	AutoReconnect         bool
	ConnectRetry          bool
	ConnectRetryInterval  time.Duration
	KeepAlive             time.Duration
	PingTimeout           time.Duration
	CleanSession          bool
	ResumeSubs            bool
	InsecureSkipVerifyTLS bool
	OnConnect             func()
	OnConnectionLost      func(error)
}

func sessionOptions(opts Options) (cleanSession bool, setCleanSession bool, resumeSubs bool) {
	if opts.ResumeSubs {
		return false, true, true
	}
	if opts.CleanSession {
		return true, true, false
	}
	return false, false, false
}

func normalizeBrokerURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	server := u.Host
	switch u.Scheme {
	case "mqtt", "tcp", "":
		if server == "" {
			server = u.Path
		}
		return "tcp://" + server, nil
	case "ssl", "tls":
		return "ssl://" + server, nil
	case "ws", "wss":
		return u.Scheme + "://" + server + u.Path, nil
	default:
		return "", fmt.Errorf("unsupported mqtt scheme %q", u.Scheme)
	}
}

func Connect(opts Options) (*Client, error) {
	brokerURL := strings.TrimSpace(opts.BrokerURL)
	if brokerURL == "" {
		brokerURL = "mqtt://mosquitto:1883"
	}
	server, err := normalizeBrokerURL(brokerURL)
	if err != nil {
		return nil, err
	}
	clientID := strings.TrimSpace(opts.ClientID)
	if clientID == "" {
		prefix := strings.TrimSpace(opts.ClientIDPrefix)
		if prefix == "" {
			prefix = "homenavi"
		}
		clientID = prefix + "-" + time.Now().Format("150405.000")
	}
	mopts := mqtt.NewClientOptions()
	mopts.AddBroker(server)
	mopts.SetClientID(clientID)
	mopts.SetAutoReconnect(opts.AutoReconnect)
	mopts.SetConnectRetry(opts.ConnectRetry)
	if opts.ConnectRetryInterval > 0 {
		mopts.SetConnectRetryInterval(opts.ConnectRetryInterval)
	}
	if opts.KeepAlive > 0 {
		mopts.SetKeepAlive(opts.KeepAlive)
	}
	if opts.PingTimeout > 0 {
		mopts.SetPingTimeout(opts.PingTimeout)
	}
	if cleanSession, setCleanSession, resumeSubs := sessionOptions(opts); setCleanSession {
		mopts.SetCleanSession(cleanSession)
		mopts.SetResumeSubs(resumeSubs)
	}
	if strings.HasPrefix(server, "ssl://") || strings.HasPrefix(server, "wss://") {
		mopts.SetTLSConfig(&tls.Config{InsecureSkipVerify: opts.InsecureSkipVerifyTLS})
	}
	parsed, _ := url.Parse(strings.TrimSpace(brokerURL))
	if parsed != nil && parsed.User != nil {
		pw, _ := parsed.User.Password()
		mopts.SetUsername(parsed.User.Username())
		mopts.SetPassword(pw)
	}
	mopts.OnConnect = func(_ mqtt.Client) {
		slog.Info("mqtt connected", "broker", brokerURL)
		if opts.OnConnect != nil {
			opts.OnConnect()
		}
	}
	mopts.OnConnectionLost = func(_ mqtt.Client, err error) {
		slog.Error("mqtt connection lost", "broker", brokerURL, "error", err)
		if opts.OnConnectionLost != nil {
			opts.OnConnectionLost(err)
		}
	}
	cli := mqtt.NewClient(mopts)
	tok := cli.Connect()
	if ok := tok.WaitTimeout(15 * time.Second); !ok {
		return nil, fmt.Errorf("mqtt connect timeout")
	}
	if err := tok.Error(); err != nil {
		return nil, err
	}
	return &Client{cli: cli}, nil
}

func (c *Client) Subscribe(topic string, cb Handler) error {
	return c.SubscribeWithQoS(topic, 0, cb)
}

func (c *Client) SubscribeWithQoS(topic string, qos byte, cb Handler) error {
	t := c.cli.Subscribe(topic, qos, cb)
	t.Wait()
	return t.Error()
}

func (c *Client) SubscribeFunc(topic string, cb func(Message)) error {
	return c.SubscribeFuncWithQoS(topic, 0, cb)
}

func (c *Client) SubscribeFuncWithQoS(topic string, qos byte, cb func(Message)) error {
	return c.SubscribeWithQoS(topic, qos, func(_ mqtt.Client, msg mqtt.Message) {
		cb(msg)
	})
}

func (c *Client) Publish(topic string, payload []byte) error {
	return c.PublishWith(topic, payload, false)
}

func (c *Client) PublishWith(topic string, payload []byte, retain bool) error {
	return c.PublishWithOptions(topic, payload, 0, retain)
}

func (c *Client) PublishWithOptions(topic string, payload []byte, qos byte, retain bool) error {
	t := c.cli.Publish(topic, qos, retain, payload)
	t.Wait()
	return t.Error()
}

func (c *Client) Unsubscribe(topic string) error {
	t := c.cli.Unsubscribe(topic)
	t.Wait()
	return t.Error()
}

func (c *Client) Disconnect(quiesceMs uint) {
	if c == nil || c.cli == nil {
		return
	}
	c.cli.Disconnect(quiesceMs)
}

func (c *Client) Close() {
	c.Disconnect(1000)
}

func (c *Client) IsConnected() bool {
	return c != nil && c.cli != nil && c.cli.IsConnected()
}
