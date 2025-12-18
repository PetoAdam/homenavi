package ingest

import (
	"context"
	"strings"
	"testing"
	"time"

	"history-service/internal/store"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeMsg struct {
	topic    string
	payload  []byte
	retained bool
}

func (m fakeMsg) Topic() string   { return m.topic }
func (m fakeMsg) Payload() []byte { return m.payload }
func (m fakeMsg) Retained() bool  { return m.retained }

func openRepo(t *testing.T) *store.Repo {
	t.Helper()
	// Use a unique in-memory DB per test (still shared cache within that DB)
	// to avoid cross-test contamination when tests run in parallel.
	dsn := "file:ingest_" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repo, err := store.New(db)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return repo
}

func TestParseDeviceID(t *testing.T) {
	id, err := ParseDeviceID("homenavi/hdp/device/state/", "homenavi/hdp/device/state/zigbee/0xabc")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if id != "zigbee/0xabc" {
		t.Fatalf("expected zigbee/0xabc, got %q", id)
	}
}

func TestHandleMessageStoresValidJSON(t *testing.T) {
	repo := openRepo(t)
	ing := &Ingestor{Repo: repo, StatePrefix: "homenavi/hdp/device/state/", AllowRetains: true}
	msg := fakeMsg{topic: "homenavi/hdp/device/state/zigbee/0x1", payload: []byte(`{"on":true}`), retained: false}
	ing.HandleMessage(context.Background(), msg, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))

	page, err := repo.ListStatePoints(context.Background(), "zigbee/0x1", time.Time{}, time.Time{}, 10, nil, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Points) != 1 {
		t.Fatalf("expected 1 row, got %d", len(page.Points))
	}
}

func TestHandleMessageRejectsInvalidJSON(t *testing.T) {
	repo := openRepo(t)
	ing := &Ingestor{Repo: repo, StatePrefix: "homenavi/hdp/device/state/", AllowRetains: true}
	msg := fakeMsg{topic: "homenavi/hdp/device/state/zigbee/0x1", payload: []byte(`{not-json}`), retained: false}
	ing.HandleMessage(context.Background(), msg, time.Now().UTC())

	page, err := repo.ListStatePoints(context.Background(), "zigbee/0x1", time.Time{}, time.Time{}, 10, nil, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Points) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(page.Points))
	}
}
