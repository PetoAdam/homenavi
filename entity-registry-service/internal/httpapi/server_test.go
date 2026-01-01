package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entity-registry-service/internal/realtime"
	"entity-registry-service/internal/store"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWebSocket_EmitsOnRoomCreate(t *testing.T) {
	repo := newTestRepo(t)
	hub := realtime.NewHub()
	srv := NewServer(repo, hub)
	mux := http.NewServeMux()
	srv.Register(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/ers"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	// Trigger mutation that should emit.
	res, payload := doJSON(t, ts.Client(), http.MethodPost, ts.URL+"/api/ers/rooms/", map[string]any{"name": "Kitchen"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create room status=%d payload=%v", res.StatusCode, payload)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ws: %v", err)
	}
	var ev realtime.Event
	if err := json.Unmarshal(msg, &ev); err != nil {
		t.Fatalf("unmarshal event: %v msg=%s", err, string(msg))
	}
	if ev.Type != "ers.room.created" {
		t.Fatalf("unexpected event type: %q", ev.Type)
	}
}

func newTestRepo(t *testing.T) *store.Repo {
	t.Helper()
	// Use a unique in-memory DB per test to avoid cross-test contamination.
	dsn := "file:memdb_" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repo, err := store.New(db)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	return repo
}

func doJSON(t *testing.T, client *http.Client, method, url string, body any) (*http.Response, any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode json: %v", err)
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	var payload any
	_ = json.NewDecoder(res.Body).Decode(&payload)
	return res, payload
}

func TestSelectorResolve_Tag(t *testing.T) {
	repo := newTestRepo(t)
	srv := NewServer(repo, nil)
	mux := http.NewServeMux()
	srv.Register(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c := ts.Client()

	// Create tag kitchen
	res, payload := doJSON(t, c, http.MethodPost, ts.URL+"/api/ers/tags/", map[string]any{"name": "Kitchen"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create tag status=%d payload=%v", res.StatusCode, payload)
	}

	// Create device
	res, payload = doJSON(t, c, http.MethodPost, ts.URL+"/api/ers/devices/", map[string]any{"name": "Ceiling Light"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create device status=%d payload=%v", res.StatusCode, payload)
	}
	deviceID := uuid.MustParse(payload.(map[string]any)["id"].(string))

	// Bind HDP
	res, payload = doJSON(t, c, http.MethodPut, ts.URL+"/api/ers/devices/"+deviceID.String()+"/bindings/hdp", map[string]any{"hdp_external_id": "zigbee/0x001"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("bind hdp status=%d payload=%v", res.StatusCode, payload)
	}

	// Fetch tag id
	res, payload = doJSON(t, c, http.MethodGet, ts.URL+"/api/ers/tags/", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list tags status=%d payload=%v", res.StatusCode, payload)
	}
	tags := payload.([]any)
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	tagID := uuid.MustParse(tags[0].(map[string]any)["id"].(string))

	// Set membership
	res, payload = doJSON(t, c, http.MethodPut, ts.URL+"/api/ers/tags/"+tagID.String()+"/members", map[string]any{"device_ids": []string{deviceID.String()}})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set members status=%d payload=%v", res.StatusCode, payload)
	}

	// Resolve selector
	res, payload = doJSON(t, c, http.MethodPost, ts.URL+"/api/ers/selectors/resolve", map[string]any{"selector": "tag:kitchen"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("resolve status=%d payload=%v", res.StatusCode, payload)
	}
	resp := payload.(map[string]any)
	ids := resp["hdp_external_ids"].([]any)
	if len(ids) != 1 || ids[0].(string) != "zigbee/0x001" {
		t.Fatalf("unexpected resolved ids: %v", ids)
	}
}

func TestSelectorResolve_Room(t *testing.T) {
	repo := newTestRepo(t)
	srv := NewServer(repo, nil)
	mux := http.NewServeMux()
	srv.Register(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c := ts.Client()

	res, payload := doJSON(t, c, http.MethodPost, ts.URL+"/api/ers/rooms/", map[string]any{"name": "Living Room"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create room status=%d payload=%v", res.StatusCode, payload)
	}
	roomID := uuid.MustParse(payload.(map[string]any)["id"].(string))

	res, payload = doJSON(t, c, http.MethodPost, ts.URL+"/api/ers/devices/", map[string]any{"name": "Lamp", "room_id": roomID.String()})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create device status=%d payload=%v", res.StatusCode, payload)
	}
	devID := uuid.MustParse(payload.(map[string]any)["id"].(string))

	res, payload = doJSON(t, c, http.MethodPut, ts.URL+"/api/ers/devices/"+devID.String()+"/bindings/hdp", map[string]any{"hdp_external_id": "zigbee/0x002"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("bind hdp status=%d payload=%v", res.StatusCode, payload)
	}

	// Resolve by id
	res, payload = doJSON(t, c, http.MethodPost, ts.URL+"/api/ers/selectors/resolve", map[string]any{"selector": "room:" + roomID.String()})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("resolve room id status=%d payload=%v", res.StatusCode, payload)
	}
	ids := payload.(map[string]any)["hdp_external_ids"].([]any)
	if len(ids) != 1 || ids[0].(string) != "zigbee/0x002" {
		t.Fatalf("unexpected ids: %v", ids)
	}

	// Resolve by slug
	res, payload = doJSON(t, c, http.MethodPost, ts.URL+"/api/ers/selectors/resolve", map[string]any{"selector": "room:living-room"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("resolve room slug status=%d payload=%v", res.StatusCode, payload)
	}
	ids = payload.(map[string]any)["hdp_external_ids"].([]any)
	if len(ids) != 1 || ids[0].(string) != "zigbee/0x002" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

func TestDevicesSetTags(t *testing.T) {
	repo := newTestRepo(t)
	srv := NewServer(repo, nil)
	mux := http.NewServeMux()
	srv.Register(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c := ts.Client()

	// Create two tags
	res, payload := doJSON(t, c, http.MethodPost, ts.URL+"/api/ers/tags/", map[string]any{"name": "Kitchen"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create tag status=%d payload=%v", res.StatusCode, payload)
	}
	res, payload = doJSON(t, c, http.MethodPost, ts.URL+"/api/ers/tags/", map[string]any{"name": "Lights"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create tag status=%d payload=%v", res.StatusCode, payload)
	}

	// Fetch tag ids
	res, payload = doJSON(t, c, http.MethodGet, ts.URL+"/api/ers/tags/", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list tags status=%d payload=%v", res.StatusCode, payload)
	}
	tags := payload.([]any)
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
	tag1 := uuid.MustParse(tags[0].(map[string]any)["id"].(string))
	tag2 := uuid.MustParse(tags[1].(map[string]any)["id"].(string))

	// Create device
	res, payload = doJSON(t, c, http.MethodPost, ts.URL+"/api/ers/devices/", map[string]any{"name": "Ceiling Light"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create device status=%d payload=%v", res.StatusCode, payload)
	}
	deviceID := uuid.MustParse(payload.(map[string]any)["id"].(string))

	// Set tags
	res, payload = doJSON(t, c, http.MethodPut, ts.URL+"/api/ers/devices/"+deviceID.String()+"/tags", map[string]any{"tag_ids": []string{tag1.String(), tag2.String()}})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set tags status=%d payload=%v", res.StatusCode, payload)
	}

	dev := payload.(map[string]any)
	devTags, ok := dev["tags"].([]any)
	if !ok {
		t.Fatalf("expected tags array, got %T", dev["tags"])
	}
	if len(devTags) != 2 {
		t.Fatalf("expected 2 tags on device, got %d", len(devTags))
	}

	// Clear tags
	res, payload = doJSON(t, c, http.MethodPut, ts.URL+"/api/ers/devices/"+deviceID.String()+"/tags", map[string]any{"tag_ids": []string{}})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("clear tags status=%d payload=%v", res.StatusCode, payload)
	}
	dev = payload.(map[string]any)
	devTags, ok = dev["tags"].([]any)
	if !ok {
		t.Fatalf("expected tags array, got %T", dev["tags"])
	}
	if len(devTags) != 0 {
		t.Fatalf("expected 0 tags after clear, got %d", len(devTags))
	}
}

func TestDevicesPatch_ReturnsDeviceView(t *testing.T) {
	repo := newTestRepo(t)
	srv := NewServer(repo, nil)
	mux := http.NewServeMux()
	srv.Register(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c := ts.Client()

	// Create a tag.
	res, payload := doJSON(t, c, http.MethodPost, ts.URL+"/api/ers/tags/", map[string]any{"name": "Kitchen"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create tag status=%d payload=%v", res.StatusCode, payload)
	}
	res, payload = doJSON(t, c, http.MethodGet, ts.URL+"/api/ers/tags/", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list tags status=%d payload=%v", res.StatusCode, payload)
	}
	tags := payload.([]any)
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	tagID := uuid.MustParse(tags[0].(map[string]any)["id"].(string))

	// Create device.
	res, payload = doJSON(t, c, http.MethodPost, ts.URL+"/api/ers/devices/", map[string]any{"name": "Lamp"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create device status=%d payload=%v", res.StatusCode, payload)
	}
	deviceID := uuid.MustParse(payload.(map[string]any)["id"].(string))

	// Set tags.
	res, payload = doJSON(t, c, http.MethodPut, ts.URL+"/api/ers/devices/"+deviceID.String()+"/tags", map[string]any{"tag_ids": []string{tagID.String()}})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set tags status=%d payload=%v", res.StatusCode, payload)
	}

	// Set multi-bindings.
	res, payload = doJSON(t, c, http.MethodPut, ts.URL+"/api/ers/devices/"+deviceID.String()+"/bindings/hdp", map[string]any{"hdp_external_ids": []string{"zigbee/0x001", "thread/abcd"}})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set bindings status=%d payload=%v", res.StatusCode, payload)
	}

	// PATCH should return a DeviceView (tags + hdp_external_ids included).
	res, payload = doJSON(t, c, http.MethodPatch, ts.URL+"/api/ers/devices/"+deviceID.String(), map[string]any{"name": "Lamp v2"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("patch device status=%d payload=%v", res.StatusCode, payload)
	}
	dev := payload.(map[string]any)
	if dev["name"].(string) != "Lamp v2" {
		t.Fatalf("unexpected name: %v", dev["name"])
	}
	if _, ok := dev["tags"].([]any); !ok {
		t.Fatalf("expected tags array, got %T", dev["tags"])
	}
	ids, ok := dev["hdp_external_ids"].([]any)
	if !ok {
		t.Fatalf("expected hdp_external_ids array, got %T", dev["hdp_external_ids"])
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 bindings, got %v", ids)
	}
}
