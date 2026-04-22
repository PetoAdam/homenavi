package mqttx

import (
	"testing"
	"time"
)

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
