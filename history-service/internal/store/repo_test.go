package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openTestRepo(t *testing.T) *Repo {
	t.Helper()
	// Use a unique in-memory DB per test to avoid cross-test contamination.
	dsn := "file:store_" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repo, err := New(db)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return repo
}

func TestListStatePointsCursorAsc(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Insert 3 points, with two sharing the same timestamp to exercise (ts,id) ordering.
	p1 := &DeviceStatePoint{ID: uuid.New(), DeviceID: "zigbee/0x1", TS: base.Add(1 * time.Second), Payload: []byte(`{"v":1}`)}
	p2 := &DeviceStatePoint{ID: uuid.New(), DeviceID: "zigbee/0x1", TS: base.Add(2 * time.Second), Payload: []byte(`{"v":2}`)}
	p3 := &DeviceStatePoint{ID: uuid.New(), DeviceID: "zigbee/0x1", TS: base.Add(2 * time.Second), Payload: []byte(`{"v":3}`)}

	// Ensure deterministic order for p2/p3
	if p3.ID.String() < p2.ID.String() {
		p2, p3 = p3, p2
	}

	for _, p := range []*DeviceStatePoint{p1, p2, p3} {
		if err := repo.InsertStatePoint(ctx, p); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	page1, err := repo.ListStatePoints(ctx, "zigbee/0x1", time.Time{}, time.Time{}, 2, nil, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page1.Points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(page1.Points))
	}
	if page1.NextCursor == "" {
		t.Fatalf("expected next_cursor")
	}

	cur, err := DecodeCursor(page1.NextCursor)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	page2, err := repo.ListStatePoints(ctx, "zigbee/0x1", time.Time{}, time.Time{}, 2, cur, false)
	if err != nil {
		t.Fatalf("list page2: %v", err)
	}
	if len(page2.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(page2.Points))
	}
	if page2.NextCursor != "" {
		t.Fatalf("did not expect next_cursor")
	}
}
