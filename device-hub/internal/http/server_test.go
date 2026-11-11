package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	model "github.com/PetoAdam/homenavi/device-hub/internal/devices"
	dbinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/db"
	mqttinfra "github.com/PetoAdam/homenavi/device-hub/internal/infra/mqtt"
	"github.com/PetoAdam/homenavi/shared/mqttx"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDeviceStateBatchValidation(t *testing.T) {
	for _, test := range []struct {
		name  string
		ids   []string
		valid bool
	}{
		{"valid", []string{"zigbee/0x0011223344556677", "thread/abc"}, true},
		{"empty", nil, false},
		{"duplicate", []string{"zigbee/0x0011223344556677", "zigbee/0x0011223344556677"}, false},
		{"missing protocol", []string{"abc"}, false},
		{"noncanonical", []string{" ZIGBEE/0x001 "}, false},
		{"oversized", make([]string, 101), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, valid := deviceStateKeys(test.ids)
			if valid != test.valid {
				t.Fatalf("valid=%v, want %v", valid, test.valid)
			}
		})
	}
}

func TestDeviceStateBatchResults(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	rows := []dbinfra.DeviceStateView{
		{Protocol: "zigbee", ExternalID: "one", Online: true, LastSeen: now, State: json.RawMessage(`{"value":0,"on":false}`), StateUpdatedAt: &now},
		{Protocol: "zigbee", ExternalID: "two", LastSeen: now},
	}
	results := batchDeviceStates([]string{"zigbee/two", "zigbee/missing", "zigbee/one"}, rows)
	if len(results) != 3 || results[0].Status != "state_unavailable" || results[1].Status != "not_found" || results[2].Status != "ok" || string(results[2].State) != `{"value":0,"on":false}` {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestDeviceStateBatchRejectsInvalidRequests(t *testing.T) {
	server := NewServer(nil, nil)
	for _, body := range []string{`{}`, `{"device_ids":[]}`, `{"device_ids":["zigbee/x"],"extra":true}`, `{"device_ids":["zigbee/x"]} {}`} {
		response := httptest.NewRecorder()
		server.handleDeviceStatesBatch(response, httptest.NewRequest(http.MethodPost, "/api/hdp/device-states:batch", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d", body, response.Code)
		}
	}
}

func TestDeviceStateBatchReadsPersistedStates(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.Device{}, &dbinfra.DeviceState{}); err != nil {
		t.Fatal(err)
	}
	deviceID := uuid.New()
	device := model.Device{ID: deviceID, Protocol: "thread", ExternalID: "one", Online: true}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	stateTime := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	if err := database.Create(&dbinfra.DeviceState{DeviceID: deviceID.String(), State: json.RawMessage(`{"temperature":0,"on":false}`), UpdatedAt: stateTime}).Error; err != nil {
		t.Fatal(err)
	}
	server := NewServer(dbinfra.New(database), nil)
	request := httptest.NewRequest(http.MethodPost, "/api/hdp/device-states:batch", strings.NewReader(`{"device_ids":["thread/missing","thread/one"]}`))
	response := httptest.NewRecorder()
	server.handleDeviceStatesBatch(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Results []batchDeviceState `json:"results"`
		ReadAt  time.Time          `json:"read_at"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Results) != 2 || payload.Results[0].Status != "not_found" || payload.Results[1].Status != "ok" || string(payload.Results[1].State) != `{"temperature":0,"on":false}` || payload.Results[1].StateUpdatedAt == nil || !payload.Results[1].StateUpdatedAt.Equal(stateTime) || payload.ReadAt.IsZero() {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	largeState, err := json.Marshal(map[string]string{"value": strings.Repeat("x", maxDeviceStateResponseBytes)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&dbinfra.DeviceState{}).Where("device_id = ?", deviceID.String()).Update("state", largeState).Error; err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	server.handleDeviceStatesBatch(response, httptest.NewRequest(http.MethodPost, "/api/hdp/device-states:batch", strings.NewReader(`{"device_ids":["thread/one"]}`)))
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized response status=%d", response.Code)
	}
}

func TestDeviceMetadataPage(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.Device{}); err != nil {
		t.Fatal(err)
	}
	if err := dbinfra.EnsureMetadataRevision(database); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"} {
		device := model.Device{ID: uuid.MustParse(id), Protocol: "thread", ExternalID: id, Type: "thermostat", Capabilities: datatypes.JSON(`[{"id":"temperature"}]`)}
		if err := database.Create(&device).Error; err != nil {
			t.Fatal(err)
		}
	}
	server := NewServer(dbinfra.New(database), nil)
	for _, test := range []struct {
		query    string
		status   int
		expected string
	}{
		{"?limit=1", http.StatusOK, `"next_cursor":"00000000-0000-0000-0000-000000000001"`},
		{"?limit=1&cursor=00000000-0000-0000-0000-000000000001&revision=2", http.StatusOK, `"next_cursor":""`},
		{"?cursor=00000000-0000-0000-0000-000000000001", http.StatusBadRequest, "revision"},
		{"?revision=1", http.StatusConflict, "changed"},
		{"?limit=101", http.StatusBadRequest, "limit"},
		{"?cursor=invalid", http.StatusBadRequest, "cursor"},
	} {
		response := httptest.NewRecorder()
		server.handleDeviceMetadataPage(response, httptest.NewRequest(http.MethodGet, "/api/hdp/device-metadata"+test.query, nil))
		if response.Code != test.status || !strings.Contains(response.Body.String(), test.expected) {
			t.Fatalf("query=%s status=%d body=%s", test.query, response.Code, response.Body.String())
		}
		if test.query == "?limit=1" && (!strings.Contains(response.Body.String(), `"source_id":"00000000-0000-0000-0000-000000000001"`) || strings.Contains(response.Body.String(), `"state"`)) {
			t.Fatalf("metadata page lacks source ID or includes state: %s", response.Body.String())
		}
	}
	if err := database.Model(&model.Device{}).Where("id = ?", "00000000-0000-0000-0000-000000000001").Update("type", "sensor").Error; err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.handleDeviceMetadataPage(response, httptest.NewRequest(http.MethodGet, "/api/hdp/device-metadata?limit=1&cursor=00000000-0000-0000-0000-000000000001&revision=2", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("changed metadata scan status=%d", response.Code)
	}
}

func TestDeviceMetadataPagesMoreThanOneHundredDevices(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.Device{}); err != nil {
		t.Fatal(err)
	}
	if err := dbinfra.EnsureMetadataRevision(database); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 151; index++ {
		device := model.Device{Protocol: "thread", ExternalID: fmt.Sprintf("device-%03d", index), Capabilities: datatypes.JSON(`[]`)}
		if err := database.Create(&device).Error; err != nil {
			t.Fatal(err)
		}
	}
	server := NewServer(dbinfra.New(database), nil)
	query := "?limit=100"
	seen := map[string]bool{}
	for pageIndex := 0; pageIndex < 2; pageIndex++ {
		response := httptest.NewRecorder()
		server.handleDeviceMetadataPage(response, httptest.NewRequest(http.MethodGet, "/api/hdp/device-metadata"+query, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		var page struct {
			Devices []struct {
				DeviceID string `json:"device_id"`
			} `json:"devices"`
			Revision   int64  `json:"revision"`
			NextCursor string `json:"next_cursor"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		for _, device := range page.Devices {
			if seen[device.DeviceID] {
				t.Fatalf("duplicate device %s", device.DeviceID)
			}
			seen[device.DeviceID] = true
		}
		if pageIndex == 0 && page.NextCursor == "" || pageIndex == 1 && page.NextCursor != "" {
			t.Fatalf("unexpected page cursor %q", page.NextCursor)
		}
		query = fmt.Sprintf("?limit=100&cursor=%s&revision=%d", page.NextCursor, page.Revision)
	}
	if len(seen) != 151 {
		t.Fatalf("got %d devices", len(seen))
	}
}

type subscriptionTestMQTTClient struct {
	subscriptions []mqttx.SubscriptionOptions
}

func (c *subscriptionTestMQTTClient) Subscribe(_ string, _ mqttinfra.Handler) error { return nil }

func (c *subscriptionTestMQTTClient) SubscribeWithOptions(opts mqttx.SubscriptionOptions, _ mqttinfra.Handler) error {
	c.subscriptions = append(c.subscriptions, opts)
	return nil
}

func (c *subscriptionTestMQTTClient) Publish(_ string, _ []byte) error { return nil }

func (c *subscriptionTestMQTTClient) PublishWith(_ string, _ []byte, _ bool) error { return nil }

func TestRegisterSharesDurableSubscriptionsButNotAdapterDiscovery(t *testing.T) {
	mqtt := &subscriptionTestMQTTClient{}
	srv := NewServer(nil, mqtt, WithMQTTSharedGroup("device-hub-ingest"))
	srv.Register(http.NewServeMux())

	subscriptions := map[string]mqttx.SubscriptionOptions{}
	for _, subscription := range mqtt.subscriptions {
		subscriptions[subscription.Topic] = subscription
		if subscription.Mode == mqttx.SubscriptionModeShared {
			if subscription.Group != "device-hub-ingest" {
				t.Fatalf("unexpected shared subscription group %q", subscription.Group)
			}
		}
	}
	for _, topic := range []string{hdpAdapterHelloTopic, hdpAdapterStatusPrefix + "#", hdpPairingProgressPrefix + "#"} {
		if subscription := subscriptions[topic]; subscription.Mode != mqttx.SubscriptionModeExclusive {
			t.Fatalf("expected %q to use an exclusive subscription, got %#v", topic, subscription)
		}
	}
	for _, topic := range []string{hdpMetadataPrefix + "#", hdpStatePrefix + "#", hdpEventPrefix + "#", hdpCommandResultPrefix + "#"} {
		if subscription := subscriptions[topic]; subscription.Mode != mqttx.SubscriptionModeShared {
			t.Fatalf("expected %q to use the shared group, got %#v", topic, subscription)
		}
	}
}

func TestHandlePairingConfigReturnsConfigs(t *testing.T) {
	srv := NewServer(nil, nil)
	srv.adapters.upsertFromHello([]byte(`{
		"schema":"hdp.v1",
		"type":"hello",
		"adapter_id":"zigbee",
		"protocol":"zigbee",
		"version":"test",
		"features": {"supports_pairing": true, "supports_interview": true},
		"pairing": {
                  "schema_version": "1.0",
		  "label":"Zigbee",
		  "supported": true,
		  "supports_interview": true,
		  "default_timeout_sec": 60,
		  "instructions": ["a","b"],
		  "cta_label": "Start Zigbee pairing",
                  "flow": {"entry_modes": ["default"]}
		},
		"ts": 1
	}`))
	srv.adapters.upsertFromHello([]byte(`{
		"schema":"hdp.v1",
		"type":"hello",
		"adapter_id":"mock",
		"protocol":"mock",
		"version":"test",
		"features": {"supports_pairing": true, "supports_interview": false},
		"pairing": {
                  "schema_version": "1.0",
		  "label":"Mock Adapter",
		  "supported": false,
		  "supports_interview": false,
		  "default_timeout_sec": 30,
		  "notes": "placeholder"
		},
		"ts": 1
	}`))

	req := httptest.NewRequest(http.MethodGet, "/api/hdp/pairing-config", nil)
	rr := httptest.NewRecorder()

	srv.handlePairingConfig(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", status, http.StatusOK)
	}

	var got []PairingConfig
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid json: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("unexpected config count: got %d want %d", len(got), 2)
	}

	byProto := map[string]PairingConfig{}
	for _, cfg := range got {
		byProto[cfg.Protocol] = cfg
	}
	if _, ok := byProto["zigbee"]; !ok {
		t.Fatalf("expected zigbee config present: %+v", got)
	}
	if _, ok := byProto["mock"]; !ok {
		t.Fatalf("expected mock config present: %+v", got)
	}
	if byProto["zigbee"].SchemaVersion != "1.0" {
		t.Fatalf("expected zigbee schema version 1.0, got %q", byProto["zigbee"].SchemaVersion)
	}
	if byProto["zigbee"].Flow == nil {
		t.Fatalf("expected zigbee flow in pairing config")
	}
}

func TestHandlePairingConfigEmpty(t *testing.T) {
	srv := NewServer(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/hdp/pairing-config", nil)
	rr := httptest.NewRecorder()

	srv.handlePairingConfig(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", status, http.StatusOK)
	}

	var got []any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid json: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected empty array, got %d items", len(got))
	}
}

func TestHandleDeviceCommandInvalidJSON(t *testing.T) {
	srv := NewServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/hdp/devices/abc/commands", strings.NewReader("{not-json"))
	rr := httptest.NewRecorder()

	srv.handleDeviceCommand(rr, req, "abc")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "invalid json") {
		t.Fatalf("expected invalid json message, got %q", rr.Body.String())
	}
}

func TestHandleDeviceCommandMissingState(t *testing.T) {
	srv := NewServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/hdp/devices/abc/commands", strings.NewReader(`{"state":{},"input":null}`))
	rr := httptest.NewRecorder()

	srv.handleDeviceCommand(rr, req, "abc")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "state is required") {
		t.Fatalf("expected missing state message, got %q", rr.Body.String())
	}
}

func TestValidateCapabilityStatePatch(t *testing.T) {
	capabilities := datatypes.JSON([]byte(`[
		{"id":"state","property":"state","value_type":"boolean","true_value":"ON","false_value":"OFF","access":{"write":true}},
		{"id":"brightness","property":"brightness","value_type":"number","access":{"write":true},"range":{"min":0,"max":254}},
		{"id":"temperature","property":"temperature","value_type":"number","access":{"write":false}}
	]`))
	if err := validateCapabilityStatePatch(capabilities, map[string]any{"state": true, "brightness": float64(120)}); err != nil {
		t.Fatalf("expected valid capability write: %v", err)
	}
	if err := validateCapabilityStatePatch(capabilities, map[string]any{"state": "ON"}); err != nil {
		t.Fatalf("expected declared binary value to be accepted: %v", err)
	}
	if err := validateCapabilityStatePatch(capabilities, map[string]any{"state": "INVALID"}); err == nil {
		t.Fatal("expected invalid binary value rejection")
	}
	if err := validateCapabilityStatePatch(capabilities, map[string]any{"temperature": float64(20)}); err == nil {
		t.Fatal("expected read-only property rejection")
	}
	if err := validateCapabilityStatePatch(capabilities, map[string]any{"brightness": float64(255)}); err == nil {
		t.Fatal("expected out-of-range rejection")
	}
}

func TestNormalizeLegacyOnStatePatch(t *testing.T) {
	capabilities := datatypes.JSON([]byte(`[
		{"id":"state","property":"state","value_type":"boolean","access":{"write":true}}
	]`))
	state := normalizeLegacyOnStatePatch(capabilities, map[string]any{"on": true})
	if got, ok := state["state"].(bool); !ok || !got {
		t.Fatalf("expected legacy on value to map to state, got %#v", state)
	}
	if _, found := state["on"]; found {
		t.Fatalf("expected legacy on property to be removed, got %#v", state)
	}
}

func TestNormalizeLegacyOnStatePatchPreservesNativeOnCapability(t *testing.T) {
	capabilities := datatypes.JSON([]byte(`[
		{"id":"on","property":"on","value_type":"boolean","access":{"write":true}},
		{"id":"state","property":"state","value_type":"boolean","access":{"write":true}}
	]`))
	state := normalizeLegacyOnStatePatch(capabilities, map[string]any{"on": true})
	if _, found := state["state"]; found {
		t.Fatalf("expected native on property to remain unchanged, got %#v", state)
	}
	if got, ok := state["on"].(bool); !ok || !got {
		t.Fatalf("expected native on value to remain, got %#v", state)
	}
}

func TestHandleDeviceCreateInvalidJSON(t *testing.T) {
	srv := NewServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/hdp/devices", strings.NewReader("{bad"))
	rr := httptest.NewRecorder()

	srv.handleDeviceCreate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "invalid json") {
		t.Fatalf("expected invalid json message, got %q", rr.Body.String())
	}
}

func TestHandleDeviceCreateMissingFields(t *testing.T) {
	srv := NewServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/hdp/devices", strings.NewReader(`{"protocol":"","external_id":""}`))
	rr := httptest.NewRecorder()

	srv.handleDeviceCreate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", rr.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rr.Body.String(), "protocol and external_id are required") {
		t.Fatalf("expected external id message, got %q", rr.Body.String())
	}
}

func TestParseHDPDeviceRequestPath_WithSlashDeviceIDCommands(t *testing.T) {
	deviceID, action, ok := parseHDPDeviceRequestPath("/api/hdp/devices/zigbee/0xa4c13867e32d96d4/commands")
	if !ok {
		t.Fatalf("expected ok")
	}
	if action != "commands" {
		t.Fatalf("expected action commands, got %q", action)
	}
	if deviceID != "zigbee/0xa4c13867e32d96d4" {
		t.Fatalf("expected device id %q got %q", "zigbee/0xa4c13867e32d96d4", deviceID)
	}
}

func TestHandleDeviceRequest_StateIsRequired(t *testing.T) {
	srv := NewServer(nil, nil)

	body := `{"correlation_id":"test-corr"}`
	req := httptest.NewRequest(http.MethodPost, "/api/hdp/devices/zigbee/0xa4c13867e32d96d4/commands", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.handleDeviceRequest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d body=%q", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "state is required") {
		t.Fatalf("expected state required message, got %q", rr.Body.String())
	}
}

func TestParseHDPDeviceRequestPath_WithSlashDeviceIDRefresh(t *testing.T) {
	deviceID, action, ok := parseHDPDeviceRequestPath("/api/hdp/devices/zigbee/0xa4c13867e32d96d4/refresh")
	if !ok {
		t.Fatalf("expected ok")
	}
	if action != "refresh" {
		t.Fatalf("expected action refresh, got %q", action)
	}
	if deviceID != "zigbee/0xa4c13867e32d96d4" {
		t.Fatalf("expected device id %q got %q", "zigbee/0xa4c13867e32d96d4", deviceID)
	}
}

func TestParseHDPDeviceRequestPath_WithSlashDeviceIDReconfigure(t *testing.T) {
	deviceID, action, ok := parseHDPDeviceRequestPath("/api/hdp/devices/zigbee/0xa4c13867e32d96d4/reconfigure")
	if !ok {
		t.Fatalf("expected ok")
	}
	if action != "reconfigure" {
		t.Fatalf("expected action reconfigure, got %q", action)
	}
	if deviceID != "zigbee/0xa4c13867e32d96d4" {
		t.Fatalf("expected device id %q got %q", "zigbee/0xa4c13867e32d96d4", deviceID)
	}
}

func TestHandleDeviceReconfigureInvalidJSON(t *testing.T) {
	srv := NewServer(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/hdp/devices/abc/reconfigure", strings.NewReader(`{bad`))
	rr := httptest.NewRecorder()

	srv.handleDeviceReconfigure(rr, req, "abc")

	if rr.Code != http.StatusInternalServerError && rr.Code != http.StatusBadRequest && rr.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: got %d", rr.Code)
	}
}

func TestManagementActionsForProtocol_UsesInterviewSupport(t *testing.T) {
	srv := NewServer(nil, nil)
	srv.adapters.upsertFromHello([]byte(`{"adapter_id":"zigbee","protocol":"zigbee","pairing":{"schema_version":"1.0","supported":true,"supports_interview":true}}`))

	actions := srv.managementActionsForProtocol("zigbee")
	if len(actions) != 2 {
		t.Fatalf("expected two management actions, got %#v", actions)
	}
	if actions[0].Command != "reconfigure" || actions[0].Mode != "interview" {
		t.Fatalf("expected reconfigure interview action, got %#v", actions[0])
	}
	if actions[1].Command != "reconfigure" || actions[1].Mode != "force_interview" {
		t.Fatalf("expected force reinterview action, got %#v", actions[1])
	}
	capability := srv.reconfigureCapabilityForProtocol("zigbee")
	if capability == nil || capability.Version != "v1" || len(capability.Modes) != 2 {
		t.Fatalf("expected v1 capability with two modes, got %#v", capability)
	}
	if !srv.supportsReconfigureMode("zigbee", "force_interview") {
		t.Fatal("expected force_interview to be supported")
	}
}

func TestConfigurationStatusForDevice_ReadyWhenCapabilitiesPresent(t *testing.T) {
	status := configurationStatusForDevice(&model.Device{
		Protocol:     "zigbee",
		Capabilities: datatypes.JSON([]byte(`[{"id":"state"}]`)),
	})
	if !status.Ready || status.Status != "configured" {
		t.Fatalf("expected configured status, got %#v", status)
	}
	if status.Message != "" {
		t.Fatalf("expected empty configured message, got %q", status.Message)
	}
}

func TestConfigurationStatusForDevice_IncompleteWhenMetadataMissing(t *testing.T) {
	status := configurationStatusForDevice(&model.Device{Protocol: "zigbee"})
	if status.Ready {
		t.Fatalf("expected incomplete status, got %#v", status)
	}
	if status.Status != "incomplete" {
		t.Fatalf("expected incomplete status string, got %#v", status)
	}
	if !strings.Contains(strings.ToLower(status.Message), "reinterview") {
		t.Fatalf("expected zigbee guidance message, got %q", status.Message)
	}
}

func TestParseHDPDeviceRequestPath_WithSlashDeviceIDGet(t *testing.T) {
	deviceID, action, ok := parseHDPDeviceRequestPath("/api/hdp/devices/zigbee/0xa4c13867e32d96d4")
	if !ok {
		t.Fatalf("expected ok")
	}
	if action != "" {
		t.Fatalf("expected empty action, got %q", action)
	}
	if deviceID != "zigbee/0xa4c13867e32d96d4" {
		t.Fatalf("expected device id %q got %q", "zigbee/0xa4c13867e32d96d4", deviceID)
	}
}
