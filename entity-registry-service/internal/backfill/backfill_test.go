package backfill

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"entity-registry-service/internal/store"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestRepo(t *testing.T) *store.Repo {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repo, err := store.New(db)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	return repo
}

func TestRunOnce_CreatesAndIsIdempotent(t *testing.T) {
	repo := newTestRepo(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/hdp/devices" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"device_id": "zigbee/0x001", "name": "Lamp", "description": ""},
			{"device_id": "zigbee/0x002", "name": "", "manufacturer": "Acme", "model": "Sensor"},
		})
	}))
	defer ts.Close()

	ctx := context.Background()
	created, err := RunOnce(ctx, repo, ts.URL, ts.Client())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if created != 2 {
		t.Fatalf("expected created=2, got %d", created)
	}

	created2, err := RunOnce(ctx, repo, ts.URL, ts.Client())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if created2 != 0 {
		t.Fatalf("expected created=0 on second run, got %d", created2)
	}

	devs, err := repo.ListDevices(ctx)
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devs) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devs))
	}

	seen := map[string]bool{}
	for _, d := range devs {
		if len(d.HDPExternalIDs) == 0 {
			t.Fatalf("expected hdp binding")
		}
		seen[d.HDPExternalIDs[0]] = true
	}
	if !seen["zigbee/0x001"] || !seen["zigbee/0x002"] {
		t.Fatalf("unexpected bindings: %#v", seen)
	}
}
