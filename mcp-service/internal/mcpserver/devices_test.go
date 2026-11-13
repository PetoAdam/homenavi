package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestDeviceClientContinuesMCPTrace(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	var traceparent, authorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		traceparent = request.Header.Get("traceparent")
		authorization = request.Header.Get("Authorization")
		_, _ = writer.Write([]byte(`[]`))
	}))
	defer upstream.Close()
	ctx, parent := provider.Tracer("test").Start(t.Context(), "mcp")
	client := &httpDeviceClient{readBaseURL: upstream.URL, client: newDownstreamHTTPClient()}
	if _, err := client.List(ctx, "delegated-token"); err != nil {
		t.Fatal(err)
	}
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 2 || spans[0].Parent().SpanID() != parent.SpanContext().SpanID() || traceparent != "00-"+parent.SpanContext().TraceID().String()+"-"+spans[0].SpanContext().SpanID().String()+"-01" || authorization != "Bearer delegated-token" {
		t.Fatal("MCP request did not preserve trace and delegated authorization")
	}
}

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

func TestDeviceStatesInputValidation(t *testing.T) {
	for _, test := range []struct {
		name  string
		input deviceStatesInput
		valid bool
	}{
		{"valid", deviceStatesInput{DeviceIDs: []string{"thread/one"}, Fields: []string{"state"}}, true},
		{"empty", deviceStatesInput{}, false},
		{"duplicate ID", deviceStatesInput{DeviceIDs: []string{"thread/one", "thread/one"}}, false},
		{"unknown field", deviceStatesInput{DeviceIDs: []string{"thread/one"}, Fields: []string{"secret"}}, false},
		{"duplicate field", deviceStatesInput{DeviceIDs: []string{"thread/one"}, Fields: []string{"state", "state"}}, false},
		{"too many", deviceStatesInput{DeviceIDs: make([]string, 101)}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if valid := validateDeviceStatesInput(test.input) == nil; valid != test.valid {
				t.Fatalf("valid=%v, want %v", valid, test.valid)
			}
		})
	}
}

func TestDiscoveryInputValidation(t *testing.T) {
	for _, test := range []struct {
		name         string
		input        discoveryInput
		roomRequired bool
		valid        bool
	}{
		{"empty search", discoveryInput{}, false, true},
		{"missing room", discoveryInput{}, true, false},
		{"room state", discoveryInput{Room: "Bedroom"}, true, true},
		{"conflicting rooms", discoveryInput{Room: "Bedroom", RoomID: "id"}, false, false},
		{"unknown field", discoveryInput{Fields: []string{"password"}}, false, false},
		{"duplicate field", discoveryInput{Fields: []string{"name", "name"}}, false, false},
		{"conflicting detail", discoveryInput{Detail: "verbose", Fields: []string{"name"}}, false, false},
		{"invalid limit", discoveryInput{Limit: 101}, false, false},
		{"properties without state", discoveryInput{Fields: []string{"online"}, StateProperties: []string{"on"}}, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := validateDiscoveryInput(test.input, test.roomRequired) == nil; got != test.valid {
				t.Fatalf("valid=%v, want %v", got, test.valid)
			}
		})
	}
}

func TestDiscoverDevicesRoomStateBatchesAndProjects(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get("Authorization") != "Bearer delegated-token" {
			t.Fatal("missing delegated token")
		}
		switch request.URL.Path {
		case "/api/mcp/ers/discovery:room-state":
			_, _ = writer.Write([]byte(`{"room":{"id":"room-id","name":"Bedroom"},"devices":[{"device_id":"thread/one","name":"Thermostat","type":"thermostat","room":"Bedroom"},{"device_id":"thread/missing","name":"Offline","type":"sensor"}],"next_cursor":"next"}`))
		case "/api/mcp/hdp/device-states:batch":
			_, _ = writer.Write([]byte(`{"read_at":"now","results":[{"device_id":"thread/one","status":"ok","online":true,"state":{"temperature":0,"on":false,"password":"secret"},"state_updated_at":"then"},{"device_id":"thread/missing","status":"not_found"}]}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer upstream.Close()
	client := &readClients{gatewayURL: upstream.URL, client: upstream.Client()}
	result, err := client.DiscoverDevices(context.Background(), discoveryInput{Room: "Bedroom", StateProperties: []string{"temperature", "on"}}, true, "delegated-token")
	if err != nil {
		t.Fatal(err)
	}
	page := result.(map[string]any)
	devices := page["devices"].([]map[string]any)
	state := devices[0]["state"].(map[string]any)
	if requests != 2 || page["next_cursor"] != "next" || page["read_at"] != "now" || state["temperature"] != float64(0) || state["on"] != false || len(state) != 2 || devices[1]["status"] != "not_found" {
		t.Fatalf("unexpected response: %#v", page)
	}
}

func TestDiscoverDevicesRetriesOversizedBatch(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/mcp/ers/discovery:room-state":
			var input struct {
				Limit int `json:"limit"`
			}
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input.Limit == 1 {
				_, _ = writer.Write([]byte(`{"devices":[{"device_id":"thread/one"}],"next_cursor":"next"}`))
			} else {
				_, _ = writer.Write([]byte(`{"devices":[{"device_id":"thread/one"},{"device_id":"thread/two"}]}`))
			}
		case "/api/mcp/hdp/device-states:batch":
			var input struct {
				DeviceIDs []string `json:"device_ids"`
			}
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if len(input.DeviceIDs) > 1 {
				writer.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			_, _ = writer.Write([]byte(`{"results":[{"device_id":"thread/one","status":"ok","state":{"on":false}}]}`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer upstream.Close()
	client := &readClients{gatewayURL: upstream.URL, client: upstream.Client()}
	result, err := client.DiscoverDevices(context.Background(), discoveryInput{Room: "Bedroom"}, true, "token")
	if err != nil {
		t.Fatal(err)
	}
	page := result.(map[string]any)
	if len(page["devices"].([]map[string]any)) != 1 || page["next_cursor"] != "next" {
		t.Fatalf("unexpected page: %#v", page)
	}
}

func TestGetDeviceStatesProjectsFields(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/mcp/hdp/device-states:batch" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer delegated-token" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"read_at":"2026-11-10T00:00:00Z","results":[{"device_id":"thread/one","status":"ok","online":true,"state":{"on":false},"last_seen":"old","secret":"hidden"}]}`))
	}))
	defer upstream.Close()
	client := &readClients{gatewayURL: upstream.URL, client: upstream.Client()}
	result, err := client.GetDeviceStates(context.Background(), deviceStatesInput{DeviceIDs: []string{"thread/one"}, Fields: []string{"state"}}, "delegated-token")
	if err != nil {
		t.Fatal(err)
	}
	row := result.(map[string]any)["results"].([]any)[0].(map[string]any)
	if len(row) != 3 || row["device_id"] != "thread/one" || row["status"] != "ok" || row["state"].(map[string]any)["on"] != false {
		t.Fatalf("unexpected projected result: %#v", row)
	}
}

func TestGetDeviceStatesDoesNotTruncateStateArrays(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"device_id": "thread/one", "status": "ok", "state": map[string]any{"values": make([]int, 101)}}}})
	}))
	defer upstream.Close()
	client := &readClients{gatewayURL: upstream.URL, client: upstream.Client()}
	result, err := client.GetDeviceStates(context.Background(), deviceStatesInput{DeviceIDs: []string{"thread/one"}}, "delegated-token")
	if err != nil {
		t.Fatal(err)
	}
	row := result.(map[string]any)["results"].([]any)[0].(map[string]any)
	values := row["state"].(map[string]any)["values"].([]any)
	if len(values) != 101 {
		t.Fatalf("state array length = %d, want 101", len(values))
	}
}

func TestReadClientsLoadInventoryMetadataAndEnrichDevices(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer inventory-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/mcp/ers/devices":
			_, _ = w.Write([]byte(`[{"name":"Bedroom Wardrobe","description":"TRADFRI LED driver","room_id":"room-1","tags":[{"id":"tag-1","name":"lights","slug":"lights"}],"hdp_external_ids":["zigbee/wardrobe"]}]`))
		case "/api/mcp/ers/rooms":
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
			if r.URL.Query().Get("limit") != "10000" {
				t.Fatalf("history limit = %q", r.URL.Query().Get("limit"))
			}
			if r.URL.Query().Get("cursor") != "cursor-1" {
				t.Fatalf("history cursor = %q", r.URL.Query().Get("cursor"))
			}
			points := make([]map[string]any, 101)
			for index := range points {
				points[index] = map[string]any{"state": map[string]any{"on": true}}
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"points": points, "next_cursor": "cursor-2"}); err != nil {
				t.Fatalf("encode history response: %v", err)
			}
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
	history, err := client.QueryStateHistory(context.Background(), historyInput{DeviceID: "zigbee/1", Cursor: "cursor-1"}, "delegated-token")
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	points := history.(map[string]any)["points"].([]any)
	if len(points) != 101 {
		t.Fatalf("expected all history points, got %d", len(points))
	}
	if _, err := client.CreateGroup(context.Background(), groupCreateInput{Name: "Kitchen"}, "group-1", "delegated-token"); err != nil {
		t.Fatalf("create group: %v", err)
	}
}

func TestQueryStateHistoryRejectsLimitAboveMaximum(t *testing.T) {
	client := &readClients{}
	_, err := client.QueryStateHistory(context.Background(), historyInput{
		DeviceID: "zigbee/1",
		Limit:    maxHistoryQueryLimit + 1,
	}, "delegated-token")
	if err == nil || !strings.Contains(err.Error(), "10000") {
		t.Fatalf("expected maximum limit error, got %v", err)
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
