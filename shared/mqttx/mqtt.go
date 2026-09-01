package mqttx

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Message interface {
	Topic() string
	Payload() []byte
	Qos() byte
	Retained() bool
	Duplicate() bool
}

type Handler func(Message)

type BrokerKind string

const (
	BrokerKindEMQX    BrokerKind = "emqx"
	BrokerKindGeneric BrokerKind = "generic"
)

func (k BrokerKind) IsValid() bool {
	switch normalizeBrokerKind(k) {
	case BrokerKindEMQX, BrokerKindGeneric:
		return true
	default:
		return false
	}
}

type SubscriptionMode string

const (
	SubscriptionModeExclusive SubscriptionMode = "exclusive"
	SubscriptionModeShared    SubscriptionMode = "shared"
)

type SubscriptionOptions struct {
	Topic string
	QoS   byte
	Mode  SubscriptionMode
	Group string
}

type Capabilities struct {
	SharedSubscriptions bool
}

type TopicStrategy interface {
	ResolveSubscribeTopic(SubscriptionOptions) (string, error)
	Capabilities() Capabilities
}

type emqxTopicStrategy struct{}

func (emqxTopicStrategy) ResolveSubscribeTopic(opts SubscriptionOptions) (string, error) {
	if opts.Mode == SubscriptionModeShared {
		return SharedTopic(opts.Group, opts.Topic), nil
	}
	return opts.Topic, nil
}

func (emqxTopicStrategy) Capabilities() Capabilities {
	return Capabilities{SharedSubscriptions: true}
}

type genericTopicStrategy struct{}

func (genericTopicStrategy) ResolveSubscribeTopic(opts SubscriptionOptions) (string, error) {
	if opts.Mode == SubscriptionModeShared {
		return "", fmt.Errorf("mqtt broker kind %q does not support shared subscriptions", BrokerKindGeneric)
	}
	return opts.Topic, nil
}

func (genericTopicStrategy) Capabilities() Capabilities {
	return Capabilities{}
}

type Client struct {
	transport  mqttTransport
	brokerKind BrokerKind
	mu         sync.RWMutex
	subs       map[string]subscription
}

type subscription struct {
	opts          SubscriptionOptions
	resolvedTopic string
	cb            Handler
}

type Options struct {
	BrokerURL             string
	BrokerKind            BrokerKind
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

func normalizeBrokerKind(kind BrokerKind) BrokerKind {
	switch strings.ToLower(strings.TrimSpace(string(kind))) {
	case "", string(BrokerKindEMQX):
		return BrokerKindEMQX
	case string(BrokerKindGeneric):
		return BrokerKindGeneric
	default:
		return BrokerKind(strings.ToLower(strings.TrimSpace(string(kind))))
	}
}

func normalizeSubscriptionMode(mode SubscriptionMode) SubscriptionMode {
	switch strings.ToLower(strings.TrimSpace(string(mode))) {
	case "", string(SubscriptionModeExclusive):
		return SubscriptionModeExclusive
	case string(SubscriptionModeShared):
		return SubscriptionModeShared
	default:
		return SubscriptionMode(strings.ToLower(strings.TrimSpace(string(mode))))
	}
}

func SharedTopic(group, topic string) string {
	return "$share/" + strings.TrimSpace(group) + "/" + strings.TrimSpace(topic)
}

func topicStrategyFor(kind BrokerKind) (TopicStrategy, error) {
	switch normalizeBrokerKind(kind) {
	case BrokerKindEMQX:
		return emqxTopicStrategy{}, nil
	case BrokerKindGeneric:
		return genericTopicStrategy{}, nil
	default:
		return nil, fmt.Errorf("unsupported mqtt broker kind %q", kind)
	}
}

func resolveSubscription(kind BrokerKind, opts SubscriptionOptions) (SubscriptionOptions, string, error) {
	normalized := SubscriptionOptions{
		Topic: strings.TrimSpace(opts.Topic),
		QoS:   opts.QoS,
		Mode:  normalizeSubscriptionMode(opts.Mode),
		Group: strings.TrimSpace(opts.Group),
	}
	if normalized.Topic == "" {
		return SubscriptionOptions{}, "", fmt.Errorf("mqtt subscribe topic is required")
	}
	switch normalized.Mode {
	case SubscriptionModeExclusive:
		return normalized, normalized.Topic, nil
	case SubscriptionModeShared:
		if normalized.Group == "" {
			return SubscriptionOptions{}, "", fmt.Errorf("mqtt shared subscription group is required")
		}
	default:
		return SubscriptionOptions{}, "", fmt.Errorf("unsupported mqtt subscription mode %q", opts.Mode)
	}
	strategy, err := topicStrategyFor(kind)
	if err != nil {
		return SubscriptionOptions{}, "", err
	}
	resolvedTopic, err := strategy.ResolveSubscribeTopic(normalized)
	if err != nil {
		return SubscriptionOptions{}, "", err
	}
	return normalized, resolvedTopic, nil
}

func Connect(opts Options) (*Client, error) {
	brokerURL := strings.TrimSpace(opts.BrokerURL)
	if brokerURL == "" {
		brokerURL = "mqtt://emqx:1883"
	}
	brokerKind := normalizeBrokerKind(opts.BrokerKind)
	if !brokerKind.IsValid() {
		return nil, fmt.Errorf("unsupported mqtt broker kind %q", opts.BrokerKind)
	}
	client := &Client{brokerKind: brokerKind, subs: make(map[string]subscription)}
	transport, err := connectPahoTransport(opts, brokerURL, func() {
		slog.Info("mqtt connected", "broker", brokerURL)
		client.resubscribeAll()
		if opts.OnConnect != nil {
			opts.OnConnect()
		}
	}, func(err error) {
		slog.Error("mqtt connection lost", "broker", brokerURL, "error", err)
		if opts.OnConnectionLost != nil {
			opts.OnConnectionLost(err)
		}
	})
	if err != nil {
		return nil, err
	}
	client.transport = transport
	return client, nil
}

func (c *Client) Subscribe(topic string, cb Handler) error {
	return c.SubscribeWithQoS(topic, 0, cb)
}

func (c *Client) SubscribeWithQoS(topic string, qos byte, cb Handler) error {
	return c.SubscribeWithOptions(SubscriptionOptions{Topic: topic, QoS: qos}, cb)
}

func (c *Client) SubscribeWithOptions(opts SubscriptionOptions, cb Handler) error {
	if c == nil || c.transport == nil {
		return fmt.Errorf("mqtt client unavailable")
	}
	if cb == nil {
		return fmt.Errorf("mqtt subscribe handler is required")
	}
	normalized, resolvedTopic, err := resolveSubscription(c.brokerKind, opts)
	if err != nil {
		return err
	}
	c.rememberSubscription(normalized, resolvedTopic, cb)
	if err := c.transport.Subscribe(resolvedTopic, normalized.QoS, cb); err != nil {
		c.forgetSubscription(resolvedTopic)
		return err
	}
	return nil
}

func (c *Client) SubscribeFunc(topic string, cb func(Message)) error {
	return c.SubscribeFuncWithQoS(topic, 0, cb)
}

func (c *Client) SubscribeFuncWithQoS(topic string, qos byte, cb func(Message)) error {
	return c.SubscribeFuncWithOptions(SubscriptionOptions{Topic: topic, QoS: qos}, cb)
}

func (c *Client) SubscribeFuncWithOptions(opts SubscriptionOptions, cb func(Message)) error {
	if cb == nil {
		return fmt.Errorf("mqtt subscribe function is required")
	}
	return c.SubscribeWithOptions(opts, func(msg Message) {
		cb(msg)
	})
}

func (c *Client) SubscribeShared(group, topic string, qos byte, cb Handler) error {
	return c.SubscribeWithOptions(SubscriptionOptions{Topic: topic, QoS: qos, Mode: SubscriptionModeShared, Group: group}, cb)
}

func (c *Client) SubscribeSharedFunc(group, topic string, qos byte, cb func(Message)) error {
	return c.SubscribeShared(group, topic, qos, func(msg Message) {
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
	if c == nil || c.transport == nil || !c.transport.IsConnected() {
		return fmt.Errorf("mqtt client unavailable")
	}
	return c.transport.Publish(topic, payload, qos, retain)
}

func (c *Client) Unsubscribe(topic string) error {
	return c.UnsubscribeWithOptions(SubscriptionOptions{Topic: topic})
}

func (c *Client) UnsubscribeWithOptions(opts SubscriptionOptions) error {
	if c == nil || c.transport == nil {
		return fmt.Errorf("mqtt client unavailable")
	}
	_, resolvedTopic, err := resolveSubscription(c.brokerKind, opts)
	if err != nil {
		return err
	}
	if err := c.transport.Unsubscribe(resolvedTopic); err != nil {
		return err
	}
	c.forgetSubscription(resolvedTopic)
	return nil
}

func (c *Client) Disconnect(quiesceMs uint) {
	if c == nil || c.transport == nil {
		return
	}
	c.transport.Disconnect(quiesceMs)
}

func (c *Client) Close() {
	c.Disconnect(1000)
}

func (c *Client) IsConnected() bool {
	return c != nil && c.transport != nil && c.transport.IsConnected()
}

func (c *Client) rememberSubscription(opts SubscriptionOptions, resolvedTopic string, cb Handler) {
	if c == nil || strings.TrimSpace(resolvedTopic) == "" || cb == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subs[resolvedTopic] = subscription{opts: opts, resolvedTopic: resolvedTopic, cb: cb}
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
	if c == nil || c.transport == nil {
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
		if err := c.transport.Subscribe(topic, sub.opts.QoS, sub.cb); err != nil {
			slog.Warn("mqtt resubscribe failed", "topic", topic, "error", err)
		} else {
			slog.Info("mqtt resubscribed", "topic", topic, "qos", sub.opts.QoS, "mode", sub.opts.Mode, "group", sub.opts.Group)
		}
	}
}
