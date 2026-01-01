package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureDeviceForHDP_Idempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repo, err := New(db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	hdpID := "zigbee/0x1234"

	id1, created1, err := repo.EnsureDeviceForHDP(ctx, hdpID, "Kitchen Light", "")
	if err != nil {
		t.Fatalf("ensure 1: %v", err)
	}
	if id1 == uuid.Nil {
		t.Fatalf("ensure 1 returned nil id")
	}
	if !created1 {
		t.Fatalf("expected first ensure to create")
	}

	id2, created2, err := repo.EnsureDeviceForHDP(ctx, hdpID, "Kitchen Light", "")
	if err != nil {
		t.Fatalf("ensure 2: %v", err)
	}
	if id2 != id1 {
		t.Fatalf("expected same id, got %s vs %s", id1, id2)
	}
	if created2 {
		t.Fatalf("expected second ensure to be no-op")
	}

	// Verify only one device exists.
	var count int64
	if err := db.Model(&Device{}).Count(&count).Error; err != nil {
		t.Fatalf("count devices: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 device, got %d", count)
	}
}
