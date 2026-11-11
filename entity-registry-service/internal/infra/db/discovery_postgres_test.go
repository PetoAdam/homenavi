package db

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDiscoveryPostgres(t *testing.T) {
	dsn := os.Getenv("DISCOVERY_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("DISCOVERY_TEST_POSTGRES_DSN not set")
	}
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := New(database)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	source := hdpDeviceRecord{ID: uuid.New(), Protocol: "thread", ExternalID: "postgres-one"}
	if err := database.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	items := []DiscoveryMetadata{{DeviceID: "thread/postgres-one", SourceID: &source.ID, DeviceType: "sensor", Capabilities: json.RawMessage(`[{"id":"temperature"}]`)}, {DeviceID: "thread/postgres-two", DeviceType: "sensor", Capabilities: json.RawMessage(`[]`)}}
	if err := repo.ApplyDiscoverySnapshot(ctx, 10, items); err != nil {
		t.Fatal(err)
	}
	filter := DiscoveryFilter{Limit: 1}
	page, err := repo.SearchDiscovery(ctx, filter)
	if err != nil || len(page.Devices) != 1 || page.NextCursor == "" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	filter.Cursor = page.NextCursor
	room := Room{Slug: "postgres-room", Name: "Room"}
	if err := repo.CreateRoom(ctx, &room); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SearchDiscovery(ctx, filter); err != ErrDiscoveryCursor {
		t.Fatalf("expected cursor invalidation, got %v", err)
	}
	page, err = repo.SearchDiscovery(ctx, DiscoveryFilter{Capability: "temperature", Limit: 25})
	if err != nil || len(page.Devices) != 1 || page.Devices[0].DeviceID != "thread/postgres-one" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	device := Device{Name: "Sensor 100%", RoomID: &room.ID}
	if err := repo.CreateDevice(ctx, &device); err != nil {
		t.Fatal(err)
	}
	binding := DeviceBinding{ID: uuid.New(), DeviceID: device.ID, HDPDeviceID: &source.ID, Kind: "hdp", ExternalID: "thread/legacy-alias"}
	if err := database.Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	page, err = repo.SearchDiscovery(ctx, DiscoveryFilter{Room: "room", Name: "100%", Capability: "temperature", Limit: 25})
	if err != nil || len(page.Devices) != 1 || page.Devices[0].DeviceID != "thread/postgres-one" {
		t.Fatalf("filtered page=%+v err=%v", page, err)
	}
}
