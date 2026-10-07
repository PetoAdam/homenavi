package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPDeviceClientListsBoundedSafeFields(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/hdp/devices" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":"internal-id","device_id":"zigbee/1","type":"light","online":true,"state":{"on":true},"firmware":"private"}]`))
	}))
	defer upstream.Close()

	devices, err := (&httpDeviceClient{baseURL: upstream.URL, client: upstream.Client()}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].DeviceID != "zigbee/1" || !devices[0].Online || devices[0].State["on"] != true {
		t.Fatalf("unexpected devices: %#v", devices)
	}
}

func TestRedactRemovesCredentialFieldsAndBoundsLists(t *testing.T) {
	value := redact(map[string]any{"token": "secret", "safe": map[string]any{"password": "hidden", "name": "Kitchen"}, "items": make([]any, 101)}).(map[string]any)
	if _, present := value["token"]; present {
		t.Fatal("token was not redacted")
	}
	safe := value["safe"].(map[string]any)
	if _, present := safe["password"]; present || safe["name"] != "Kitchen" {
		t.Fatalf("unexpected redaction result: %#v", safe)
	}
	if len(value["items"].([]any)) != 100 {
		t.Fatalf("items were not bounded: %d", len(value["items"].([]any)))
	}
}
