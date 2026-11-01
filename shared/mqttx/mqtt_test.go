package mqttx

import (
	"fmt"
	"os"
	"testing"
	"time"
)

type recordedSubscription struct {
	topic string
	qos   byte
}

type fakeTransport struct {
	connected     bool
	subscriptions []recordedSubscription
	published     []recordedSubscription
	unsubscribed  []string
}

func (t *fakeTransport) Subscribe(topic string, qos byte, _ Handler) error {
	t.subscriptions = append(t.subscriptions, recordedSubscription{topic: topic, qos: qos})
	return nil
}

func (t *fakeTransport) Publish(topic string, _ []byte, qos byte, _ bool) error {
	t.published = append(t.published, recordedSubscription{topic: topic, qos: qos})
	return nil
}

func (t *fakeTransport) Unsubscribe(topic string) error {
	t.unsubscribed = append(t.unsubscribed, topic)
	return nil
}

func (t *fakeTransport) Disconnect(uint) {}

func (t *fakeTransport) IsConnected() bool { return t.connected }

func TestNormalizeBrokerURL(t *testing.T) {
	cases := map[string]string{
		"mqtt://emqx:1883":         "tcp://emqx:1883",
		"tcp://broker:1883":        "tcp://broker:1883",
		"ssl://broker:8883":        "ssl://broker:8883",
		"wss://broker.example/ws":  "wss://broker.example/ws",
		"ws://broker.example/mqtt": "ws://broker.example/mqtt",
	}
	for in, want := range cases {
		got, err := normalizeBrokerURL(in)
		if err != nil {
			t.Fatalf("normalizeBrokerURL(%q) error: %v", in, err)
		}
		if got != want {
			t.Fatalf("normalizeBrokerURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeBrokerURLRejectsUnsupportedScheme(t *testing.T) {
	if _, err := normalizeBrokerURL("amqp://rabbitmq:5672"); err == nil {
		t.Fatal("expected unsupported scheme error")
	}
}

func TestTransportBrokerURL(t *testing.T) {
	cases := map[string]string{
		"mqtt://broker:1883": "mqtt://broker:1883",
		"tcp://broker:1883":  "mqtt://broker:1883",
		"ssl://broker:8883":  "tls://broker:8883",
		"wss://broker/ws":    "wss://broker/ws",
	}
	for input, want := range cases {
		got, err := transportBrokerURL(input)
		if err != nil {
			t.Fatalf("transportBrokerURL(%q) error: %v", input, err)
		}
		if got != want {
			t.Fatalf("transportBrokerURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMatchesMQTTTopic(t *testing.T) {
	cases := []struct {
		filter string
		topic  string
		want   bool
	}{
		{filter: "homenavi/hdp/state/#", topic: "homenavi/hdp/state/kitchen", want: true},
		{filter: "homenavi/hdp/+/kitchen", topic: "homenavi/hdp/state/kitchen", want: true},
		{filter: "$share/history/homenavi/hdp/state/#", topic: "homenavi/hdp/state/kitchen", want: true},
		{filter: "homenavi/hdp/state/+/extra", topic: "homenavi/hdp/state/kitchen", want: false},
	}
	for _, testCase := range cases {
		if got := matchesMQTTTopic(testCase.filter, testCase.topic); got != testCase.want {
			t.Fatalf("matchesMQTTTopic(%q, %q) = %t, want %t", testCase.filter, testCase.topic, got, testCase.want)
		}
	}
}

func TestSessionOptionsDefaultsToPahoBehavior(t *testing.T) {
	cleanSession, setCleanSession, resumeSubs := sessionOptions(Options{})
	if setCleanSession {
		t.Fatal("expected zero-value options to preserve paho clean-session defaults")
	}
	if cleanSession {
		t.Fatal("expected zero-value cleanSession result to be false when unset")
	}
	if resumeSubs {
		t.Fatal("expected zero-value options to keep resume subscriptions disabled")
	}
}

func TestSessionOptionsPersistentSession(t *testing.T) {
	cleanSession, setCleanSession, resumeSubs := sessionOptions(Options{ResumeSubs: true})
	if !setCleanSession {
		t.Fatal("expected resume subscriptions to force explicit session configuration")
	}
	if cleanSession {
		t.Fatal("expected resume subscriptions to require clean session false")
	}
	if !resumeSubs {
		t.Fatal("expected resume subscriptions to be enabled")
	}
}

func TestReconnectIntervalsDefaultToFastRecovery(t *testing.T) {
	connectRetryInterval, maxReconnectInterval := reconnectIntervals(Options{AutoReconnect: true, ConnectRetry: true})
	if connectRetryInterval != 2*time.Second {
		t.Fatalf("expected default connect retry interval 2s, got %s", connectRetryInterval)
	}
	if maxReconnectInterval != 2*time.Second {
		t.Fatalf("expected default max reconnect interval 2s, got %s", maxReconnectInterval)
	}
}

func TestResolvedWriteTimeoutDefaultsToFiveSeconds(t *testing.T) {
	if got := resolvedWriteTimeout(Options{}); got != 5*time.Second {
		t.Fatalf("expected default write timeout 5s, got %s", got)
	}
}

func TestNormalizeBrokerKindDefaultsToMosquitto(t *testing.T) {
	if got := normalizeBrokerKind(""); got != BrokerKindMosquitto {
		t.Fatalf("expected default broker kind %q, got %q", BrokerKindMosquitto, got)
	}
}

func TestSharedTopic(t *testing.T) {
	if got := SharedTopic("history-ingest", "homenavi/hdp/state/#"); got != "$share/history-ingest/homenavi/hdp/state/#" {
		t.Fatalf("unexpected shared topic %q", got)
	}
}

func TestResolveSubscriptionDefaultsToExclusive(t *testing.T) {
	opts, resolved, err := resolveSubscription(BrokerKindEMQX, SubscriptionOptions{Topic: "homenavi/hdp/state/#", QoS: 1})
	if err != nil {
		t.Fatalf("resolveSubscription() error = %v", err)
	}
	if resolved != "homenavi/hdp/state/#" {
		t.Fatalf("unexpected resolved topic %q", resolved)
	}
	if opts.Mode != SubscriptionModeExclusive {
		t.Fatalf("expected exclusive mode, got %q", opts.Mode)
	}
}

func TestResolveSubscriptionSharedEMQX(t *testing.T) {
	opts, resolved, err := resolveSubscription(BrokerKindEMQX, SubscriptionOptions{Topic: "homenavi/hdp/state/#", QoS: 1, Mode: SubscriptionModeShared, Group: "history-ingest"})
	if err != nil {
		t.Fatalf("resolveSubscription() error = %v", err)
	}
	if resolved != "$share/history-ingest/homenavi/hdp/state/#" {
		t.Fatalf("unexpected resolved topic %q", resolved)
	}
	if opts.Group != "history-ingest" {
		t.Fatalf("unexpected group %q", opts.Group)
	}
}

func TestResolveSubscriptionSharedSupportedBrokers(t *testing.T) {
	for _, brokerKind := range []BrokerKind{BrokerKindMosquitto, BrokerKindVerneMQ} {
		t.Run(string(brokerKind), func(t *testing.T) {
			_, resolved, err := resolveSubscription(brokerKind, SubscriptionOptions{Topic: "homenavi/hdp/state/#", QoS: 1, Mode: SubscriptionModeShared, Group: "history-ingest"})
			if err != nil {
				t.Fatalf("resolveSubscription() error = %v", err)
			}
			if resolved != "$share/history-ingest/homenavi/hdp/state/#" {
				t.Fatalf("unexpected resolved topic %q", resolved)
			}
		})
	}
}

func TestResolveSubscriptionSharedRequiresGroup(t *testing.T) {
	_, _, err := resolveSubscription(BrokerKindEMQX, SubscriptionOptions{Topic: "homenavi/hdp/state/#", Mode: SubscriptionModeShared})
	if err == nil {
		t.Fatal("expected group error")
	}
}

func TestResolveSubscriptionSharedRejectsGenericBroker(t *testing.T) {
	_, _, err := resolveSubscription(BrokerKindGeneric, SubscriptionOptions{Topic: "homenavi/hdp/state/#", Mode: SubscriptionModeShared, Group: "history-ingest"})
	if err == nil {
		t.Fatal("expected unsupported broker error")
	}
}

func TestTopicStrategiesExposeExpectedCapabilities(t *testing.T) {
	emqx, err := topicStrategyFor(BrokerKindEMQX)
	if err != nil {
		t.Fatalf("topicStrategyFor(emqx) error = %v", err)
	}
	if !emqx.Capabilities().SharedSubscriptions {
		t.Fatal("expected EMQX shared-subscription capability")
	}
	for _, brokerKind := range []BrokerKind{BrokerKindMosquitto, BrokerKindVerneMQ} {
		strategy, err := topicStrategyFor(brokerKind)
		if err != nil {
			t.Fatalf("topicStrategyFor(%s) error = %v", brokerKind, err)
		}
		if !strategy.Capabilities().SharedSubscriptions {
			t.Fatalf("expected %s shared-subscription capability", brokerKind)
		}
	}

	generic, err := topicStrategyFor(BrokerKindGeneric)
	if err != nil {
		t.Fatalf("topicStrategyFor(generic) error = %v", err)
	}
	if generic.Capabilities().SharedSubscriptions {
		t.Fatal("did not expect generic shared-subscription capability")
	}
}

func TestTopicStrategyRejectsUnknownBroker(t *testing.T) {
	if _, err := topicStrategyFor("unknown"); err == nil {
		t.Fatal("expected unknown broker error")
	}
}

func TestSubscribeWithOptionsRejectsUnavailableClient(t *testing.T) {
	var client *Client
	err := client.SubscribeWithOptions(SubscriptionOptions{Topic: "homenavi/hdp/state/#"}, func(Message) {})
	if err == nil {
		t.Fatal("expected unavailable client error")
	}
}

func TestSubscribeWithOptionsRejectsNilHandler(t *testing.T) {
	client := &Client{}
	err := client.SubscribeWithOptions(SubscriptionOptions{Topic: "homenavi/hdp/state/#"}, nil)
	if err == nil {
		t.Fatal("expected missing handler error")
	}
}

func TestSubscribeFuncWithOptionsRejectsNilFunction(t *testing.T) {
	client := &Client{}
	err := client.SubscribeFuncWithOptions(SubscriptionOptions{Topic: "homenavi/hdp/state/#"}, nil)
	if err == nil {
		t.Fatal("expected missing subscribe function error")
	}
}

func TestUnsubscribeWithOptionsRejectsUnavailableClient(t *testing.T) {
	var client *Client
	err := client.UnsubscribeWithOptions(SubscriptionOptions{Topic: "homenavi/hdp/state/#"})
	if err == nil {
		t.Fatal("expected unavailable client error")
	}
}

func TestClientDelegatesSharedSubscriptionToTransport(t *testing.T) {
	transport := &fakeTransport{connected: true}
	client := &Client{brokerKind: BrokerKindEMQX, transport: transport, subs: make(map[string]subscription)}

	err := client.SubscribeWithOptions(SubscriptionOptions{
		Topic: "homenavi/hdp/state/#",
		QoS:   1,
		Mode:  SubscriptionModeShared,
		Group: "history-ingest",
	}, func(Message) {})
	if err != nil {
		t.Fatalf("SubscribeWithOptions() error = %v", err)
	}
	if len(transport.subscriptions) != 1 {
		t.Fatalf("expected one transport subscription, got %d", len(transport.subscriptions))
	}
	if got := transport.subscriptions[0]; got.topic != "$share/history-ingest/homenavi/hdp/state/#" || got.qos != 1 {
		t.Fatalf("unexpected transport subscription: %#v", got)
	}
}

func TestClientReplaysOriginalSubscriptionIntentThroughTransport(t *testing.T) {
	transport := &fakeTransport{connected: true}
	client := &Client{brokerKind: BrokerKindEMQX, transport: transport, subs: make(map[string]subscription)}
	if err := client.SubscribeWithOptions(SubscriptionOptions{Topic: "homenavi/hdp/state/#", QoS: 1, Mode: SubscriptionModeShared, Group: "history-ingest"}, func(Message) {}); err != nil {
		t.Fatalf("SubscribeWithOptions() error = %v", err)
	}

	client.resubscribeAll()
	if len(transport.subscriptions) != 2 {
		t.Fatalf("expected initial subscription plus replay, got %d", len(transport.subscriptions))
	}
	if got := transport.subscriptions[1]; got.topic != "$share/history-ingest/homenavi/hdp/state/#" || got.qos != 1 {
		t.Fatalf("unexpected replay subscription: %#v", got)
	}
}

func TestClientDelegatesPublishAndUnsubscribeToTransport(t *testing.T) {
	transport := &fakeTransport{connected: true}
	client := &Client{brokerKind: BrokerKindEMQX, transport: transport, subs: make(map[string]subscription)}

	if err := client.PublishWithOptions("homenavi/hdp/device/command/test", []byte("{}"), 1, false); err != nil {
		t.Fatalf("PublishWithOptions() error = %v", err)
	}
	if err := client.UnsubscribeWithOptions(SubscriptionOptions{Topic: "homenavi/hdp/state/#", Mode: SubscriptionModeShared, Group: "history-ingest"}); err != nil {
		t.Fatalf("UnsubscribeWithOptions() error = %v", err)
	}
	if len(transport.published) != 1 || transport.published[0].topic != "homenavi/hdp/device/command/test" {
		t.Fatalf("unexpected published records: %#v", transport.published)
	}
	if len(transport.unsubscribed) != 1 || transport.unsubscribed[0] != "$share/history-ingest/homenavi/hdp/state/#" {
		t.Fatalf("unexpected unsubscribe records: %#v", transport.unsubscribed)
	}
}

func TestMQTT5LivePublishSubscribe(t *testing.T) {
	brokerURL := os.Getenv("MQTTX_LIVE_BROKER_URL")
	if brokerURL == "" {
		t.Skip("set MQTTX_LIVE_BROKER_URL to run the live MQTT 5 transport test")
	}

	topic := fmt.Sprintf("homenavi/test/mqttx/%d", time.Now().UnixNano())
	received := make(chan []byte, 1)
	subscriber, err := Connect(Options{BrokerURL: brokerURL, BrokerKind: BrokerKindMosquitto, ClientID: "mqttx-live-subscriber"})
	if err != nil {
		t.Fatalf("connect subscriber: %v", err)
	}
	defer subscriber.Close()
	if err := subscriber.SubscribeWithQoS(topic, 1, func(message Message) {
		received <- message.Payload()
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	publisher, err := Connect(Options{BrokerURL: brokerURL, BrokerKind: BrokerKindMosquitto, ClientID: "mqttx-live-publisher"})
	if err != nil {
		t.Fatalf("connect publisher: %v", err)
	}
	defer publisher.Close()
	if err := publisher.PublishWithOptions(topic, []byte("mqtt5-live"), 1, false); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case payload := <-received:
		if string(payload) != "mqtt5-live" {
			t.Fatalf("received payload %q", payload)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for MQTT 5 delivery")
	}
}
