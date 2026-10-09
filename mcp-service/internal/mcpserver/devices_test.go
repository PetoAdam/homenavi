package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPDeviceClientListsBoundedSafeFields(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/mcp/hdp/devices" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer delegated-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`[{"id":"internal-id","device_id":"zigbee/1","type":"light","online":true,"state":{"on":true},"firmware":"private"}]`))
	}))
	defer upstream.Close()

	devices, err := (&httpDeviceClient{readBaseURL: upstream.URL + "/api/mcp/hdp", client: upstream.Client()}).List(context.Background(), "delegated-token")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].DeviceID != "zigbee/1" || !devices[0].Online || devices[0].State["on"] != true {
		t.Fatalf("unexpected devices: %#v", devices)
	}
}

func TestReadClientsLoadInventoryMetadataAndEnrichDevices(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer inventory-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/mcp/ers/devices/":
			_, _ = w.Write([]byte(`[{"name":"Bedroom Wardrobe","description":"TRADFRI LED driver","room_id":"room-1","tags":[{"id":"tag-1","name":"lights","slug":"lights"}],"hdp_external_ids":["zigbee/wardrobe"]}]`))
		case "/api/mcp/ers/rooms/":
			_, _ = w.Write([]byte(`[{"id":"room-1","name":"Bedroom","slug":"bedroom"}]`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	client := &readClients{gatewayURL: upstream.URL, client: upstream.Client()}
	entities, err := client.ListInventoryDevices(context.Background(), "inventory-token")
	if err != nil {
		t.Fatal(err)
	}
	rooms, err := client.ListInventoryRooms(context.Background(), "inventory-token")
	if err != nil {
		t.Fatal(err)
	}
	devices := []device{{ID: "device-1", DeviceID: "zigbee/wardrobe"}, {ID: "device-2", DeviceID: "zigbee/unbound"}}
	enrichDevices(devices, entities, rooms)

	if devices[0].Name != "Bedroom Wardrobe" || devices[0].Description != "TRADFRI LED driver" || devices[0].Room == nil || devices[0].Room.Name != "Bedroom" || len(devices[0].Tags) != 1 || devices[0].Tags[0].Name != "lights" {
		t.Fatalf("unexpected enriched device: %#v", devices[0])
	}
	if devices[1].Name != "" || devices[1].Room != nil || len(devices[1].Tags) != 0 {
		t.Fatalf("unbound device was enriched: %#v", devices[1])
	}
}

func TestEnrichDevicesLeavesUnknownRoomsUnset(t *testing.T) {
	roomID := "unknown-room"
	devices := []device{{DeviceID: "zigbee/wardrobe"}}
	enrichDevices(devices, []inventoryDevice{{Name: "Bedroom Wardrobe", RoomID: &roomID, HDPExternalIDs: []string{"zigbee/wardrobe"}}}, nil)
	if devices[0].Name != "Bedroom Wardrobe" || devices[0].Room != nil {
		t.Fatalf("unexpected enrichment: %#v", devices[0])
	}
}

func TestHTTPDeviceClientCommandsThroughGatewayWithDelegatedToken(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/mcp/hdp/devices/zigbee/1/commands" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer delegated-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content type = %q", r.Header.Get("Content-Type"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["correlation_id"] != "command-1" {
			t.Fatalf("correlation ID = %#v", body["correlation_id"])
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"queued"}`))
	}))
	defer upstream.Close()

	client := &httpDeviceClient{commandBaseURL: upstream.URL + "/api/mcp/hdp/devices", client: upstream.Client()}
	if _, err := client.Command(context.Background(), "zigbee/1", map[string]any{"on": true}, nil, "command-1", "delegated-token"); err != nil {
		t.Fatalf("command: %v", err)
	}
}

func TestReadClientsUseGatewayForHistoryAndGroupCreation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer delegated-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/mcp/history/state":
			if r.Method != http.MethodGet || r.URL.Query().Get("device_id") != "zigbee/1" {
				t.Fatalf("history request = %s %s", r.Method, r.URL.String())
			}
			_, _ = w.Write([]byte(`[{"state":{"on":true}}]`))
		case "/api/mcp/ers/group-creates":
			if r.Method != http.MethodPost || r.Header.Get("Idempotency-Key") != "group-1" {
				t.Fatalf("group create request = %s idempotency=%q", r.Method, r.Header.Get("Idempotency-Key"))
			}
			_, _ = w.Write([]byte(`{"id":"group-1"}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	client := &readClients{gatewayURL: upstream.URL, client: upstream.Client()}
	if _, err := client.QueryStateHistory(context.Background(), historyInput{DeviceID: "zigbee/1"}, "delegated-token"); err != nil {
		t.Fatalf("query history: %v", err)
	}
	if _, err := client.CreateGroup(context.Background(), groupCreateInput{Name: "Kitchen"}, "group-1", "delegated-token"); err != nil {
		t.Fatalf("create group: %v", err)
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
