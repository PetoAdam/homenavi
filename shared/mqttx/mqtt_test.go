package mqttx

import "testing"

func TestNormalizeBrokerURL(t *testing.T) {
	cases := map[string]string{
		"mqtt://mosquitto:1883":    "tcp://mosquitto:1883",
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
