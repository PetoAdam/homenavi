package mqttx

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Message = mqtt.Message

type Handler = mqtt.MessageHandler

type Client struct {
	cli  mqtt.Client
	mu   sync.RWMutex
	subs map[string]subscription
}

type subscription struct {
	qos byte
	cb  Handler
}

type Options struct {
	BrokerURL             string
	ClientID              string
	ClientIDPrefix        string
	AutoReconnect         bool
	ConnectRetry          bool
	ConnectRetryInterval  time.Duration
	MaxReconnectInterval  time.Duration
	KeepAlive             time.Duration
	PingTimeout           time.Duration
	WriteTimeout          time.Duration
	CleanSession          bool
	ResumeSubs            bool
	InsecureSkipVerifyTLS bool
	OnConnect             func()
	OnConnectionLost      func(error)
}

func reconnectIntervals(opts Options) (time.Duration, time.Duration) {
	connectRetryInterval := opts.ConnectRetryInterval
	if connectRetryInterval <= 0 && opts.ConnectRetry {
		connectRetryInterval = 2 * time.Second
	}

	maxReconnectInterval := opts.MaxReconnectInterval
	if maxReconnectInterval <= 0 {
		switch {
		case connectRetryInterval > 0:
			maxReconnectInterval = connectRetryInterval
		case opts.AutoReconnect || opts.ConnectRetry:
			maxReconnectInterval = 5 * time.Second
		}
	}

	return connectRetryInterval, maxReconnectInterval
}

func resolvedWriteTimeout(opts Options) time.Duration {
	if opts.WriteTimeout > 0 {
		return opts.WriteTimeout
	}
	return 5 * time.Second
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
		brokerURL = "mqtt://emqx:1883"
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
	client := &Client{subs: make(map[string]subscription)}
	mopts.SetAutoReconnect(opts.AutoReconnect)
	mopts.SetConnectRetry(opts.ConnectRetry)
	connectRetryInterval, maxReconnectInterval := reconnectIntervals(opts)
	if connectRetryInterval > 0 {
		mopts.SetConnectRetryInterval(connectRetryInterval)
	}
	if maxReconnectInterval > 0 {
		mopts.SetMaxReconnectInterval(maxReconnectInterval)
	}
	if opts.KeepAlive > 0 {
		mopts.SetKeepAlive(opts.KeepAlive)
	}
	if opts.PingTimeout > 0 {
		mopts.SetPingTimeout(opts.PingTimeout)
	}
	mopts.SetWriteTimeout(resolvedWriteTimeout(opts))
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
		client.resubscribeAll()
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
	client.cli = cli
	return client, nil
}

func (c *Client) Subscribe(topic string, cb Handler) error {
	return c.SubscribeWithQoS(topic, 0, cb)
}

func (c *Client) SubscribeWithQoS(topic string, qos byte, cb Handler) error {
	c.rememberSubscription(topic, qos, cb)
	t := c.cli.Subscribe(topic, qos, cb)
	t.Wait()
	if err := t.Error(); err != nil {
		c.forgetSubscription(topic)
		return err
	}
	return nil
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
	if c == nil || c.cli == nil || !c.cli.IsConnected() {
		return fmt.Errorf("mqtt client unavailable")
	}
	t := c.cli.Publish(topic, qos, retain, payload)
	if ok := t.WaitTimeout(5 * time.Second); !ok {
		return fmt.Errorf("mqtt publish timeout")
	}
	return t.Error()
}

func (c *Client) Unsubscribe(topic string) error {
	t := c.cli.Unsubscribe(topic)
	t.Wait()
	if err := t.Error(); err != nil {
		return err
	}
	c.forgetSubscription(topic)
	return nil
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

func (c *Client) rememberSubscription(topic string, qos byte, cb Handler) {
	if c == nil || strings.TrimSpace(topic) == "" || cb == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subs[topic] = subscription{qos: qos, cb: cb}
}

func (c *Client) forgetSubscription(topic string) {
	if c == nil || strings.TrimSpace(topic) == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.subs, topic)
}

func (c *Client) resubscribeAll() {
	if c == nil || c.cli == nil {
		return
	}

	c.mu.RLock()
	subs := make(map[string]subscription, len(c.subs))
	for topic, sub := range c.subs {
		subs[topic] = sub
	}
	c.mu.RUnlock()

	for topic, sub := range subs {
		if strings.TrimSpace(topic) == "" || sub.cb == nil {
			continue
		}
		tok := c.cli.Subscribe(topic, sub.qos, sub.cb)
		tok.Wait()
		if err := tok.Error(); err != nil {
			slog.Warn("mqtt resubscribe failed", "topic", topic, "error", err)
		} else {
			slog.Info("mqtt resubscribed", "topic", topic, "qos", sub.qos)
		}
	}
}
