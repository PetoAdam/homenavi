package autoimport

import (
	"context"
	"encoding/json"
	"testing"

	"entity-registry-service/internal/store"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAutoImport_DeviceRemovedDeletesBoundDevice(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repo, err := store.New(db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	r := &Runner{repo: repo}

	hdpID := "zigbee/0xdeadbeef"
	// Ensure autoimport created it.
	if _, _, err := repo.EnsureDeviceForHDP(ctx, hdpID, "Test Device", ""); err != nil {
		t.Fatalf("ensure device: %v", err)
	}

	env := map[string]any{
		"schema":    "hdp.v1",
		"type":      "event",
		"device_id": hdpID,
		"event":     "device_removed",
		"ts":        123,
		"data": map[string]any{
			"reason": "test",
		},
	}
	payload, _ := json.Marshal(env)
	r.handleMessage(ctx, "homenavi/hdp/device/event/"+hdpID, payload)

	// Should be gone.
	_, ok, err := repo.FindDeviceIDByHDPExternalID(ctx, hdpID)
	if err != nil {
		t.Fatalf("find device id: %v", err)
	}
	if ok {
		t.Fatalf("expected device to be deleted")
	}
}

func TestAutoImport_DeviceRemovedUnbindsWhenMultipleBindings(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repo, err := store.New(db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	r := &Runner{repo: repo}

	id, _, err := repo.EnsureDeviceForHDP(ctx, "zigbee/0x001", "Lamp", "")
	if err != nil {
		t.Fatalf("ensure device: %v", err)
	}
	// Add a second binding.
	if err := repo.SetDeviceHDPBindings(ctx, id, []string{"zigbee/0x001", "thread/abcd"}); err != nil {
		t.Fatalf("set bindings: %v", err)
	}

	env := map[string]any{
		"schema":    "hdp.v1",
		"type":      "event",
		"device_id": "zigbee/0x001",
		"event":     "device_removed",
		"ts":        123,
	}
	payload, _ := json.Marshal(env)
	r.handleMessage(ctx, "homenavi/hdp/device/event/zigbee/0x001", payload)

	view, err := repo.GetDeviceView(ctx, id)
	if err != nil {
		t.Fatalf("get view: %v", err)
	}
	if len(view.HDPExternalIDs) != 1 || view.HDPExternalIDs[0] != "thread/abcd" {
		t.Fatalf("expected remaining binding [thread/abcd], got %v", view.HDPExternalIDs)
	}
}
