package mqttx

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	mqtt "github.com/eclipse/paho.golang/paho"
)

type mqttTransport interface {
	Subscribe(topic string, qos byte, handler Handler) error
	Publish(topic string, payload []byte, qos byte, retain bool) error
	Unsubscribe(topic string) error
	Disconnect(quiesceMs uint)
	IsConnected() bool
}

type pahoTransport struct {
	client       *autopaho.ConnectionManager
	cancel       context.CancelFunc
	mu           sync.RWMutex
	subscribers  map[string]Handler
	connected    bool
}

func connectPahoTransport(opts Options, brokerURL string, onConnect func(), onConnectionLost func(error)) (*pahoTransport, error) {
	server, err := transportBrokerURL(brokerURL)
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

	ctx, cancel := context.WithCancel(context.Background())
	transport := &pahoTransport{cancel: cancel, subscribers: make(map[string]Handler)}
	connectRetryInterval, maxReconnectInterval := reconnectIntervals(opts)
	if connectRetryInterval <= 0 {
		connectRetryInterval = 2 * time.Second
	}
	if maxReconnectInterval <= 0 {
		maxReconnectInterval = connectRetryInterval
	}

	serverURL, err := url.Parse(server)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("parse mqtt broker URL: %w", err)
	}
	parsed, _ := url.Parse(strings.TrimSpace(brokerURL))
	config := autopaho.ClientConfig{
		ServerUrls:               []*url.URL{serverURL},
		KeepAlive:                durationSeconds(opts.KeepAlive),
		ConnectTimeout:           15 * time.Second,
		ReconnectBackoff:         reconnectBackoff(connectRetryInterval, maxReconnectInterval),
		CleanStartOnInitialConnection: opts.CleanSession,
		OnConnectionUp: func(_ *autopaho.ConnectionManager, _ *mqtt.Connack) {
			transport.mu.Lock()
			transport.connected = true
			transport.mu.Unlock()
			if onConnect != nil {
				onConnect()
			}
		},
		OnConnectionDown: func() bool {
			transport.mu.Lock()
			transport.connected = false
			transport.mu.Unlock()
			if onConnectionLost != nil {
				onConnectionLost(fmt.Errorf("mqtt connection lost"))
			}
			return true
		},
		ClientConfig: mqtt.ClientConfig{
			ClientID:          clientID,
			OnPublishReceived: []func(mqtt.PublishReceived) (bool, error){transport.onPublishReceived},
			PacketTimeout:     resolvedWriteTimeout(opts),
		},
	}
	if opts.ResumeSubs {
		sessionExpiryInterval := uint32(24 * 60 * 60)
		config.SessionExpiryInterval = sessionExpiryInterval
	}
	if strings.HasPrefix(server, "tls://") || strings.HasPrefix(server, "wss://") {
		config.TlsCfg = &tls.Config{InsecureSkipVerify: opts.InsecureSkipVerifyTLS}
	}
	if parsed != nil && parsed.User != nil {
		password, _ := parsed.User.Password()
		config.ConnectUsername = parsed.User.Username()
		config.ConnectPassword = []byte(password)
	}

	client, err := autopaho.NewConnection(ctx, config)
	if err != nil {
		cancel()
		return nil, err
	}
	transport.client = client
	connectCtx, connectCancel := context.WithTimeout(ctx, 15*time.Second)
	defer connectCancel()
	if err := client.AwaitConnection(connectCtx); err != nil {
		cancel()
		return nil, fmt.Errorf("mqtt connect: %w", err)
	}
	return transport, nil
}

func transportBrokerURL(raw string) (string, error) {
	server, err := normalizeBrokerURL(raw)
	if err != nil {
		return "", err
	}
	switch {
	case strings.HasPrefix(server, "tcp://"):
		return "mqtt://" + strings.TrimPrefix(server, "tcp://"), nil
	case strings.HasPrefix(server, "ssl://"):
		return "tls://" + strings.TrimPrefix(server, "ssl://"), nil
	default:
		return server, nil
	}
}

func durationSeconds(value time.Duration) uint16 {
	if value <= 0 {
		return 0
	}
	seconds := uint64(value / time.Second)
	if seconds == 0 {
		return 1
	}
	if seconds > 65535 {
		return 65535
	}
	return uint16(seconds)
}

func reconnectBackoff(initial, maximum time.Duration) func(int) time.Duration {
	return func(attempt int) time.Duration {
		if attempt <= 0 || initial >= maximum {
			return initial
		}
		delay := initial
		for retry := 0; retry < attempt && delay < maximum; retry++ {
			delay *= 2
		}
		if delay > maximum {
			return maximum
		}
		return delay
	}
}

func (t *pahoTransport) onPublishReceived(received mqtt.PublishReceived) (bool, error) {
	if received.Packet == nil {
		return false, nil
	}
	message := pahoMessage{publish: received.Packet}
	t.mu.RLock()
	handlers := make([]Handler, 0, len(t.subscribers))
	for filter, handler := range t.subscribers {
		if matchesMQTTTopic(filter, received.Packet.Topic) {
			handlers = append(handlers, handler)
		}
	}
	t.mu.RUnlock()
	for _, handler := range handlers {
		handler(message)
	}
	return len(handlers) > 0, nil
}

func matchesMQTTTopic(filter, topic string) bool {
	filter = strings.TrimSpace(filter)
	if strings.HasPrefix(filter, "$share/") {
		parts := strings.SplitN(filter, "/", 3)
		if len(parts) != 3 || strings.TrimSpace(parts[1]) == "" {
			return false
		}
		filter = parts[2]
	}
	filterParts := strings.Split(filter, "/")
	topicParts := strings.Split(strings.TrimSpace(topic), "/")
	for index, part := range filterParts {
		switch part {
		case "#":
			return index == len(filterParts)-1
		case "+":
			if index >= len(topicParts) {
				return false
			}
		default:
			if index >= len(topicParts) || topicParts[index] != part {
				return false
			}
		}
	}
	return len(filterParts) == len(topicParts)
}

type pahoMessage struct {
	publish *mqtt.Publish
}

func (m pahoMessage) Topic() string     { return m.publish.Topic }
func (m pahoMessage) Payload() []byte   { return m.publish.Payload }
func (m pahoMessage) Qos() byte         { return m.publish.QoS }
func (m pahoMessage) Retained() bool    { return m.publish.Retain }
func (m pahoMessage) Duplicate() bool   { return m.publish.Duplicate() }

func (t *pahoTransport) Subscribe(topic string, qos byte, handler Handler) error {
	if t == nil || t.client == nil {
		return fmt.Errorf("mqtt client unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := t.client.Subscribe(ctx, &mqtt.Subscribe{Subscriptions: []mqtt.SubscribeOptions{{Topic: topic, QoS: qos}}}); err != nil {
		return err
	}
	t.mu.Lock()
	t.subscribers[topic] = handler
	t.mu.Unlock()
	return nil
}

func (t *pahoTransport) Publish(topic string, payload []byte, qos byte, retain bool) error {
	if t == nil || t.client == nil || !t.IsConnected() {
		return fmt.Errorf("mqtt client unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := t.client.Publish(ctx, &mqtt.Publish{Topic: topic, Payload: payload, QoS: qos, Retain: retain})
	return err
}

func (t *pahoTransport) Unsubscribe(topic string) error {
	if t == nil || t.client == nil {
		return fmt.Errorf("mqtt client unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := t.client.Unsubscribe(ctx, &mqtt.Unsubscribe{Topics: []string{topic}}); err != nil {
		return err
	}
	t.mu.Lock()
	delete(t.subscribers, topic)
	t.mu.Unlock()
	return nil
}

func (t *pahoTransport) Disconnect(quiesceMs uint) {
	if t == nil || t.client == nil {
		return
	}
	timeout := time.Duration(quiesceMs) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_ = t.client.Disconnect(ctx)
	if t.cancel != nil {
		t.cancel()
	}
}

func (t *pahoTransport) IsConnected() bool {
	if t == nil || t.client == nil {
		return false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.connected
}
