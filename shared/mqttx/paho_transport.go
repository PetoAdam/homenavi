package mqttx

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type mqttTransport interface {
	Subscribe(topic string, qos byte, handler Handler) error
	Publish(topic string, payload []byte, qos byte, retain bool) error
	Unsubscribe(topic string) error
	Disconnect(quiesceMs uint)
	IsConnected() bool
}

type pahoTransport struct {
	client mqtt.Client
}

func connectPahoTransport(opts Options, brokerURL string, onConnect func(), onConnectionLost func(error)) (*pahoTransport, error) {
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

	pahoOptions := mqtt.NewClientOptions()
	pahoOptions.AddBroker(server)
	pahoOptions.SetClientID(clientID)
	pahoOptions.SetAutoReconnect(opts.AutoReconnect)
	pahoOptions.SetConnectRetry(opts.ConnectRetry)
	connectRetryInterval, maxReconnectInterval := reconnectIntervals(opts)
	if connectRetryInterval > 0 {
		pahoOptions.SetConnectRetryInterval(connectRetryInterval)
	}
	if maxReconnectInterval > 0 {
		pahoOptions.SetMaxReconnectInterval(maxReconnectInterval)
	}
	if opts.KeepAlive > 0 {
		pahoOptions.SetKeepAlive(opts.KeepAlive)
	}
	if opts.PingTimeout > 0 {
		pahoOptions.SetPingTimeout(opts.PingTimeout)
	}
	pahoOptions.SetWriteTimeout(resolvedWriteTimeout(opts))
	if cleanSession, setCleanSession, resumeSubs := sessionOptions(opts); setCleanSession {
		pahoOptions.SetCleanSession(cleanSession)
		pahoOptions.SetResumeSubs(resumeSubs)
	}
	if strings.HasPrefix(server, "ssl://") || strings.HasPrefix(server, "wss://") {
		pahoOptions.SetTLSConfig(&tls.Config{InsecureSkipVerify: opts.InsecureSkipVerifyTLS})
	}
	parsed, _ := url.Parse(strings.TrimSpace(brokerURL))
	if parsed != nil && parsed.User != nil {
		password, _ := parsed.User.Password()
		pahoOptions.SetUsername(parsed.User.Username())
		pahoOptions.SetPassword(password)
	}
	pahoOptions.OnConnect = func(_ mqtt.Client) {
		if onConnect != nil {
			onConnect()
		}
	}
	pahoOptions.OnConnectionLost = func(_ mqtt.Client, err error) {
		if onConnectionLost != nil {
			onConnectionLost(err)
		}
	}

	client := mqtt.NewClient(pahoOptions)
	token := client.Connect()
	if ok := token.WaitTimeout(15 * time.Second); !ok {
		return nil, fmt.Errorf("mqtt connect timeout")
	}
	if err := token.Error(); err != nil {
		return nil, err
	}
	return &pahoTransport{client: client}, nil
}

func (t *pahoTransport) Subscribe(topic string, qos byte, handler Handler) error {
	if t == nil || t.client == nil {
		return fmt.Errorf("mqtt client unavailable")
	}
	token := t.client.Subscribe(topic, qos, func(_ mqtt.Client, message mqtt.Message) {
		handler(message)
	})
	token.Wait()
	return token.Error()
}

func (t *pahoTransport) Publish(topic string, payload []byte, qos byte, retain bool) error {
	if t == nil || t.client == nil || !t.client.IsConnected() {
		return fmt.Errorf("mqtt client unavailable")
	}
	token := t.client.Publish(topic, qos, retain, payload)
	if ok := token.WaitTimeout(5 * time.Second); !ok {
		return fmt.Errorf("mqtt publish timeout")
	}
	return token.Error()
}

func (t *pahoTransport) Unsubscribe(topic string) error {
	if t == nil || t.client == nil {
		return fmt.Errorf("mqtt client unavailable")
	}
	token := t.client.Unsubscribe(topic)
	token.Wait()
	return token.Error()
}

func (t *pahoTransport) Disconnect(quiesceMs uint) {
	if t != nil && t.client != nil {
		t.client.Disconnect(quiesceMs)
	}
}

func (t *pahoTransport) IsConnected() bool {
	return t != nil && t.client != nil && t.client.IsConnected()
}
