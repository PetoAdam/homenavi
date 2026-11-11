package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestApplyDiscoverySnapshot(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := New(database)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	items := []DiscoveryMetadata{{DeviceID: "thread/one", DeviceType: " Thermostat ", Capabilities: json.RawMessage(`[{"id":"Temperature","property":"temperature"},{"property":"Humidity"}]`)}}
	if err := repo.ApplyDiscoverySnapshot(ctx, 2, items); err != nil {
		t.Fatal(err)
	}
	var device DiscoveryDevice
	if err := database.First(&device).Error; err != nil || device.Type != "thermostat" {
		t.Fatalf("device=%+v err=%v", device, err)
	}
	var capabilities []DiscoveryCapability
	if err := database.Order("capability").Find(&capabilities).Error; err != nil || len(capabilities) != 2 || capabilities[0].Capability != "humidity" || capabilities[1].Capability != "temperature" {
		t.Fatalf("capabilities=%+v err=%v", capabilities, err)
	}
	if err := repo.ApplyDiscoverySnapshot(ctx, 1, nil); err == nil {
		t.Fatal("older snapshot accepted")
	}
	if err := repo.ApplyDiscoverySnapshot(ctx, 3, []DiscoveryMetadata{{DeviceID: "thread/one", Capabilities: json.RawMessage(`{}`)}}); err == nil {
		t.Fatal("invalid capabilities accepted")
	}
	if err := repo.ApplyDiscoverySnapshot(ctx, 3, nil); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := database.Model(&DiscoveryDevice{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestSearchDiscovery(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := New(database)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := repo.SearchDiscovery(ctx, DiscoveryFilter{Limit: 1}); err != ErrDiscoveryUnavailable {
		t.Fatalf("expected unavailable, got %v", err)
	}
	items := []DiscoveryMetadata{
		{DeviceID: "thread/one", DeviceType: "thermostat", Capabilities: json.RawMessage(`[{"id":"temperature"}]`)},
		{DeviceID: "thread/two", DeviceType: "thermostat", Capabilities: json.RawMessage(`[{"id":"temperature"}]`)},
	}
	if err := repo.ApplyDiscoverySnapshot(ctx, 1, items); err != nil {
		t.Fatal(err)
	}
	room := Room{Slug: "bedroom", Name: "Bedroom"}
	if err := repo.CreateRoom(ctx, &room); err != nil {
		t.Fatal(err)
	}
	device := Device{Name: "Sensor 100%", RoomID: &room.ID}
	if err := repo.CreateDevice(ctx, &device); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetDeviceHDPBindings(ctx, device.ID, []string{"thread/one", "thread/two"}); err != nil {
		t.Fatal(err)
	}
	filter := DiscoveryFilter{Room: "bedroom", Name: "100%", DeviceType: "THERMOSTAT", Capability: "temperature", Limit: 1}
	first, err := repo.SearchDiscovery(ctx, filter)
	if err != nil || len(first.Devices) != 1 || first.Devices[0].DeviceID != "thread/one" || first.NextCursor == "" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	filter.Cursor = first.NextCursor
	second, err := repo.SearchDiscovery(ctx, filter)
	if err != nil || len(second.Devices) != 1 || second.Devices[0].DeviceID != "thread/two" || second.NextCursor != "" {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if _, err := repo.UpdateDevice(ctx, device.ID, map[string]any{"name": "Renamed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SearchDiscovery(ctx, filter); err != ErrDiscoveryCursor {
		t.Fatalf("expected expired cursor, got %v", err)
	}
}

func TestSearchDiscoveryPaginatesLargeUnboundInventory(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := New(database)
	if err != nil {
		t.Fatal(err)
	}
	items := make([]DiscoveryMetadata, 151)
	for index := range items {
		items[index] = DiscoveryMetadata{DeviceID: fmt.Sprintf("thread/device-%03d", index), DeviceType: "sensor", Capabilities: json.RawMessage(`[]`)}
	}
	if err := repo.ApplyDiscoverySnapshot(context.Background(), 1, items); err != nil {
		t.Fatal(err)
	}
	var queryCount atomic.Int64
	if err := database.Callback().Query().Before("gorm:query").Register("count_discovery_queries", func(*gorm.DB) { queryCount.Add(1) }); err != nil {
		t.Fatal(err)
	}
	if err := database.Callback().Row().Before("gorm:row").Register("count_discovery_rows", func(*gorm.DB) { queryCount.Add(1) }); err != nil {
		t.Fatal(err)
	}
	filter := DiscoveryFilter{Limit: 25}
	var ids []string
	for {
		queryCount.Store(0)
		page, err := repo.SearchDiscovery(context.Background(), filter)
		if err != nil {
			t.Fatal(err)
		}
		if count := queryCount.Load(); count > 2 {
			t.Fatalf("unfiltered page used %d queries", count)
		}
		for _, device := range page.Devices {
			ids = append(ids, device.DeviceID)
		}
		if page.NextCursor == "" {
			break
		}
		filter.Cursor = page.NextCursor
	}
	if len(ids) != len(items) {
		t.Fatalf("got %d devices, want %d", len(ids), len(items))
	}
	for index, id := range ids {
		if id != items[index].DeviceID {
			t.Fatalf("device %d = %s", index, id)
		}
	}
	if err := database.Model(&DiscoveryStatus{}).Where("id = 1").Update("last_reconciled_at", time.Now().Add(-time.Minute-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SearchDiscovery(context.Background(), DiscoveryFilter{Limit: 25}); err != ErrDiscoveryUnavailable {
		t.Fatalf("expected stale projection error, got %v", err)
	}
}

func TestSearchDiscoveryRoomErrorsAndCursorFilters(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := New(database)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	items := []DiscoveryMetadata{{DeviceID: "thread/one", Capabilities: json.RawMessage(`[]`)}, {DeviceID: "thread/two", Capabilities: json.RawMessage(`[]`)}}
	if err := repo.ApplyDiscoverySnapshot(ctx, 1, items); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SearchDiscovery(ctx, DiscoveryFilter{Room: "missing", Limit: 1}); !errors.Is(err, ErrDiscoveryRoomNotFound) {
		t.Fatalf("expected room not found, got %v", err)
	}
	for index := 0; index < 2; index++ {
		room := Room{Slug: fmt.Sprintf("room-%d", index), Name: "Bedroom"}
		if err := repo.CreateRoom(ctx, &room); err != nil {
			t.Fatal(err)
		}
	}
	_, err = repo.SearchDiscovery(ctx, DiscoveryFilter{Room: " BEDROOM ", Limit: 1})
	var ambiguity *DiscoveryRoomAmbiguity
	if !errors.As(err, &ambiguity) || len(ambiguity.Candidates) != 2 {
		t.Fatalf("expected room ambiguity, got %v", err)
	}
	page, err := repo.SearchDiscovery(ctx, DiscoveryFilter{Limit: 1})
	if err != nil || page.NextCursor == "" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if _, err := repo.SearchDiscovery(ctx, DiscoveryFilter{Name: "one", Limit: 1, Cursor: page.NextCursor}); !errors.Is(err, ErrDiscoveryCursor) {
		t.Fatalf("expected filter-bound cursor rejection, got %v", err)
	}
}

func TestSearchDiscoveryResolvesSourceIDAndRejectsConflicts(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&hdpDeviceRecord{}); err != nil {
		t.Fatal(err)
	}
	repo, err := New(database)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	source := hdpDeviceRecord{ID: uuid.New(), Protocol: "thread", ExternalID: "actual"}
	if err := database.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.ApplyDiscoverySnapshot(ctx, 1, []DiscoveryMetadata{{DeviceID: "thread/actual", SourceID: &source.ID, Capabilities: json.RawMessage(`[]`)}}); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		device := Device{Name: fmt.Sprintf("Entity %d", index)}
		if err := repo.CreateDevice(ctx, &device); err != nil {
			t.Fatal(err)
		}
		binding := DeviceBinding{ID: uuid.New(), DeviceID: device.ID, HDPDeviceID: &source.ID, Kind: "hdp", ExternalID: fmt.Sprintf("thread/alias-%d", index)}
		if err := database.Create(&binding).Error; err != nil {
			t.Fatal(err)
		}
		page, err := repo.SearchDiscovery(ctx, DiscoveryFilter{Limit: 25})
		if index == 0 && (err != nil || len(page.Devices) != 1 || page.Devices[0].Name != "Entity 0") {
			t.Fatalf("source ID binding page=%+v err=%v", page, err)
		}
		if index == 1 && !errors.Is(err, ErrDiscoveryBindingConflict) {
			t.Fatalf("expected conflicting bindings, got %v", err)
		}
	}
}
