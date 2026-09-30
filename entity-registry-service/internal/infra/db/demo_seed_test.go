package db

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/PetoAdam/homenavi/shared/mockdemo"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSeedDemoHousehold_IsIdempotent(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:ers-demo-seed?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repo, err := New(database)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	if err := repo.SeedDemoHousehold(ctx); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if err := repo.SeedDemoHousehold(ctx); err != nil {
		t.Fatalf("second seed: %v", err)
	}

	rooms, err := repo.ListRooms(ctx)
	if err != nil {
		t.Fatalf("list rooms: %v", err)
	}
	if len(rooms) != 5 {
		t.Fatalf("expected 5 rooms, got %d", len(rooms))
	}
	if !hasRoomMapPoints(t, rooms[0].Meta) {
		t.Fatal("expected seeded room meta.map.points")
	}
	maxX, maxY := roomExtent(t, rooms)
	if maxX < 650 || maxX > 700 || maxY < 500 || maxY > 580 {
		t.Fatalf("expected demo room extent around 679x567, got %.0fx%.0f", maxX, maxY)
	}

	devices, err := repo.ListDevices(ctx)
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices) != 8 {
		t.Fatalf("expected 8 devices, got %d", len(devices))
	}
	if !hasDevicePlacement(t, devices[0].Meta) {
		t.Fatal("expected seeded device meta.map.x/y")
	}
	assertFavoriteFields(t, devices, "Sofa Lamp", []string{"on", "brightness"})
	assertFavoriteFields(t, devices, "Desk Lamp", []string{"on", "brightness"})
	assertFavoriteFields(t, devices, "Coffee Maker", []string{"power", "power_draw"})
	assertFavoriteFields(t, devices, "Entry Sensor", []string{"contact", "battery"})
	assertFavoriteFields(t, devices, "Air Purifier", []string{"on", "air_quality"})
	for _, binding := range mockdemo.Bindings() {
		assertHasBinding(t, devices, binding.SeedDeviceID, binding.HDPDeviceID)
	}

	extraBinding := "mock/custom-extra-binding"
	seededDeviceID := uuid.MustParse("0d47ed4e-6bb2-45b5-bbfc-c53ff2377701")
	baseBinding, ok := mockdemo.HDPDeviceIDForSeedDevice(seededDeviceID)
	if !ok {
		t.Fatal("expected seeded base binding")
	}
	if err := repo.SetDeviceHDPBindings(ctx, seededDeviceID, []string{baseBinding, extraBinding}); err != nil {
		t.Fatalf("set extra binding: %v", err)
	}
	if err := repo.SeedDemoHousehold(ctx); err != nil {
		t.Fatalf("third seed: %v", err)
	}
	view, err := repo.GetDeviceView(ctx, seededDeviceID)
	if err != nil {
		t.Fatalf("get device view: %v", err)
	}
	bindings := append([]string(nil), view.HDPExternalIDs...)
	sort.Strings(bindings)
	if len(bindings) != 2 || bindings[0] != extraBinding || bindings[1] != baseBinding {
		t.Fatalf("expected additive bindings [%s %s], got %v", extraBinding, baseBinding, bindings)
	}

	groups, err := repo.ListGroups(ctx)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	for _, group := range groups {
		if len(group.DeviceIDs) == 0 {
			t.Fatalf("expected group %s to have members", group.Name)
		}
	}
}

func TestSeedDemoHousehold_PrunesStaleRooms(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:ers-demo-prune?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	repo, err := New(database)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	staleRoom := Room{
		ID:        uuid.MustParse("0ba61d68-1138-494b-b909-8baec69549ea"),
		Slug:      "legacy-room",
		Name:      "Room",
		SortOrder: 5,
		Meta:      demoJSON(map[string]any{"map": map[string]any{"points": []map[string]float64{{"x": 420, "y": 224}, {"x": 728, "y": 224}, {"x": 728, "y": 420}, {"x": 420, "y": 420}}}}),
	}
	if err := repo.CreateRoom(ctx, &staleRoom); err != nil {
		t.Fatalf("create stale room: %v", err)
	}

	if err := repo.SeedDemoHousehold(ctx); err != nil {
		t.Fatalf("seed demo household: %v", err)
	}

	rooms, err := repo.ListRooms(ctx)
	if err != nil {
		t.Fatalf("list rooms: %v", err)
	}
	if len(rooms) != 5 {
		t.Fatalf("expected 5 rooms after pruning stale room, got %d", len(rooms))
	}
	for _, room := range rooms {
		if room.ID == staleRoom.ID {
			t.Fatalf("expected stale room %s to be pruned", staleRoom.ID)
		}
	}
}

func hasRoomMapPoints(t *testing.T, raw []byte) bool {
	t.Helper()
	var meta struct {
		Map struct {
			Points []map[string]float64 `json:"points"`
		} `json:"map"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("unmarshal room meta: %v", err)
	}
	return len(meta.Map.Points) >= 4
}

func hasDevicePlacement(t *testing.T, raw []byte) bool {
	t.Helper()
	var meta struct {
		Map struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"map"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("unmarshal device meta: %v", err)
	}
	return meta.Map.X > 0 && meta.Map.Y > 0
}

func assertFavoriteFields(t *testing.T, devices []DeviceView, name string, expected []string) {
	t.Helper()
	for _, device := range devices {
		if device.Name != name {
			continue
		}
		var meta struct {
			Map struct {
				FavoriteFields []string `json:"favorite_fields"`
			} `json:"map"`
		}
		if err := json.Unmarshal(device.Meta, &meta); err != nil {
			t.Fatalf("unmarshal %s meta: %v", name, err)
		}
		if len(meta.Map.FavoriteFields) != len(expected) {
			t.Fatalf("expected %s favorite fields %v, got %v", name, expected, meta.Map.FavoriteFields)
		}
		for index, field := range expected {
			if meta.Map.FavoriteFields[index] != field {
				t.Fatalf("expected %s favorite fields %v, got %v", name, expected, meta.Map.FavoriteFields)
			}
		}
		return
	}
	t.Fatalf("expected device %s in seed output", name)
}

func assertHasBinding(t *testing.T, devices []DeviceView, deviceID uuid.UUID, expected string) {
	t.Helper()
	for _, device := range devices {
		if device.ID != deviceID {
			continue
		}
		for _, binding := range device.HDPExternalIDs {
			if binding == expected {
				return
			}
		}
		t.Fatalf("expected device %s to have binding %s, got %v", deviceID, expected, device.HDPExternalIDs)
	}
	t.Fatalf("expected device %s in seed output", deviceID)
}

func roomExtent(t *testing.T, rooms []Room) (float64, float64) {
	t.Helper()
	maxX := 0.0
	maxY := 0.0
	for _, room := range rooms {
		var meta struct {
			Map struct {
				Points []struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
				} `json:"points"`
			} `json:"map"`
		}
		if err := json.Unmarshal(room.Meta, &meta); err != nil {
			t.Fatalf("unmarshal room meta: %v", err)
		}
		for _, point := range meta.Map.Points {
			if point.X > maxX {
				maxX = point.X
			}
			if point.Y > maxY {
				maxY = point.Y
			}
		}
	}
	return maxX, maxY
}
