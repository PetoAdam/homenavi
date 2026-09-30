package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/PetoAdam/homenavi/shared/mockdemo"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type demoGroupSeed struct {
	Group
	DeviceIDs []uuid.UUID
}

func (r *Repository) SeedDemoHousehold(ctx context.Context) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		seedRepo := &Repository{db: tx}
		if err := seedRepo.pruneStaleDemoRooms(ctx); err != nil {
			return fmt.Errorf("prune stale demo rooms: %w", err)
		}
		for _, room := range demoRooms() {
			if err := seedRepo.upsertDemoRoom(ctx, room); err != nil {
				return fmt.Errorf("upsert room %s: %w", room.ID, err)
			}
		}
		for _, device := range demoDevices() {
			if err := seedRepo.upsertDemoDevice(ctx, device); err != nil {
				return fmt.Errorf("upsert device %s: %w", device.ID, err)
			}
			if err := seedRepo.ensureDemoDeviceBinding(ctx, device.ID); err != nil {
				return fmt.Errorf("bind device %s: %w", device.ID, err)
			}
		}
		for _, group := range demoGroups() {
			if err := seedRepo.upsertDemoGroup(ctx, group.Group); err != nil {
				return fmt.Errorf("upsert group %s: %w", group.ID, err)
			}
			if err := seedRepo.SetGroupMembers(ctx, group.ID, group.DeviceIDs); err != nil {
				return fmt.Errorf("set group members %s: %w", group.ID, err)
			}
		}
		return nil
	})
}

func (r *Repository) pruneStaleDemoRooms(ctx context.Context) error {
	rooms, err := r.ListRooms(ctx)
	if err != nil {
		return err
	}
	allowed := make(map[uuid.UUID]struct{}, len(demoRooms()))
	for _, room := range demoRooms() {
		allowed[room.ID] = struct{}{}
	}
	for _, room := range rooms {
		if _, ok := allowed[room.ID]; ok {
			continue
		}
		if err := r.DeleteRoom(ctx, room.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) upsertDemoRoom(ctx context.Context, room Room) error {
	_, err := r.GetRoom(ctx, room.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.CreateRoom(ctx, &room)
	}
	if err != nil {
		return err
	}
	_, err = r.UpdateRoom(ctx, room.ID, map[string]any{
		"slug":       room.Slug,
		"name":       room.Name,
		"sort_order": room.SortOrder,
		"meta":       room.Meta,
	})
	return err
}

func (r *Repository) upsertDemoDevice(ctx context.Context, device Device) error {
	_, err := r.GetDevice(ctx, device.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.CreateDevice(ctx, &device)
	}
	if err != nil {
		return err
	}
	_, err = r.UpdateDevice(ctx, device.ID, map[string]any{
		"name":        device.Name,
		"description": device.Description,
		"room_id":     device.RoomID,
		"meta":        device.Meta,
	})
	return err
}

func (r *Repository) upsertDemoGroup(ctx context.Context, group Group) error {
	_, err := r.GetGroup(ctx, group.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.CreateGroup(ctx, &group)
	}
	if err != nil {
		return err
	}
	_, err = r.UpdateGroup(ctx, group.ID, map[string]any{
		"slug":        group.Slug,
		"name":        group.Name,
		"description": group.Description,
		"meta":        group.Meta,
	})
	return err
}

func (r *Repository) ensureDemoDeviceBinding(ctx context.Context, deviceID uuid.UUID) error {
	hdpDeviceID, ok := mockdemo.HDPDeviceIDForSeedDevice(deviceID)
	if !ok || hdpDeviceID == "" {
		return nil
	}
	view, err := r.GetDeviceView(ctx, deviceID)
	if err != nil {
		return err
	}
	for _, existing := range view.HDPExternalIDs {
		if existing == hdpDeviceID {
			return nil
		}
	}
	next := append(append([]string(nil), view.HDPExternalIDs...), hdpDeviceID)
	return r.SetDeviceHDPBindings(ctx, deviceID, next)
}

func demoRooms() []Room {
	return []Room{
		{ID: mustUUID("5ccf4ba6-35b8-4e96-a338-2b37d6f36d01"), Slug: "living-room", Name: "Living Room", SortOrder: 10, Meta: demoJSON(map[string]any{"map": map[string]any{"points": []map[string]float64{{"x": 0, "y": 0}, {"x": 392, "y": 0}, {"x": 392, "y": 301}, {"x": 0, "y": 301}}, "wall_lengths": []float64{5.6, 4.3, 5.6, 4.3}}})},
		{ID: mustUUID("b54f8586-1307-4b21-89f0-49f87d00c412"), Slug: "kitchen", Name: "Kitchen", SortOrder: 20, Meta: demoJSON(map[string]any{"map": map[string]any{"points": []map[string]float64{{"x": 392, "y": 0}, {"x": 637, "y": 0}, {"x": 637, "y": 238}, {"x": 392, "y": 238}}, "wall_lengths": []float64{3.5, 3.4, 3.5, 3.4}}})},
		{ID: mustUUID("3394fe86-a7c4-4b14-a7c7-0870f21f58d5"), Slug: "main-bedroom", Name: "Main Bedroom", SortOrder: 30, Meta: demoJSON(map[string]any{"map": map[string]any{"points": []map[string]float64{{"x": 0, "y": 301}, {"x": 336, "y": 301}, {"x": 336, "y": 567}, {"x": 0, "y": 567}}, "wall_lengths": []float64{4.8, 3.8, 4.8, 3.8}}})},
		{ID: mustUUID("ca4fce41-c8d2-4cb3-aef5-5bb5661ea3f8"), Slug: "home-office", Name: "Home Office", SortOrder: 40, Meta: demoJSON(map[string]any{"map": map[string]any{"points": []map[string]float64{{"x": 336, "y": 301}, {"x": 539, "y": 301}, {"x": 539, "y": 518}, {"x": 336, "y": 518}}, "wall_lengths": []float64{2.9, 3.1, 2.9, 3.1}}})},
		{ID: mustUUID("6fdde08c-f75c-4107-9fa8-04736f8c11de"), Slug: "entry-hall", Name: "Entry Hall", SortOrder: 50, Meta: demoJSON(map[string]any{"map": map[string]any{"points": []map[string]float64{{"x": 539, "y": 238}, {"x": 679, "y": 238}, {"x": 679, "y": 518}, {"x": 539, "y": 518}}, "wall_lengths": []float64{2, 4, 2, 4}}})},
	}
}

func demoDevices() []Device {
	livingRoomID := mustUUID("5ccf4ba6-35b8-4e96-a338-2b37d6f36d01")
	kitchenID := mustUUID("b54f8586-1307-4b21-89f0-49f87d00c412")
	bedroomID := mustUUID("3394fe86-a7c4-4b14-a7c7-0870f21f58d5")
	officeID := mustUUID("ca4fce41-c8d2-4cb3-aef5-5bb5661ea3f8")
	hallID := mustUUID("6fdde08c-f75c-4107-9fa8-04736f8c11de")
	return []Device{
		{ID: mustUUID("0d47ed4e-6bb2-45b5-bbfc-c53ff2377701"), Name: "Sofa Lamp", Description: "Warm standing lamp by the sofa", RoomID: &livingRoomID, Meta: demoJSON(map[string]any{"map": map[string]any{"x": 84, "y": 98, "favorite_fields": []string{"on", "brightness"}}})},
		{ID: mustUUID("4d7a0d13-927f-46c0-88bc-eb5ff4988688"), Name: "TV Backlight", Description: "Ambient strip behind the TV", RoomID: &livingRoomID, Meta: demoJSON(map[string]any{"map": map[string]any{"x": 329, "y": 133, "favorite_fields": []string{"on", "brightness"}}})},
		{ID: mustUUID("56b56529-3223-4214-ba31-0f6e7e4d59fa"), Name: "Kitchen Pendant", Description: "Main light over the island", RoomID: &kitchenID, Meta: demoJSON(map[string]any{"map": map[string]any{"x": 511, "y": 112, "favorite_fields": []string{"on", "brightness"}}})},
		{ID: mustUUID("0f03eb64-6301-46ff-8532-96ee39c3e215"), Name: "Coffee Maker", Description: "Countertop coffee machine plug", RoomID: &kitchenID, Meta: demoJSON(map[string]any{"map": map[string]any{"x": 595, "y": 189, "favorite_fields": []string{"power", "power_draw"}}})},
		{ID: mustUUID("b6ffdce7-10b6-4eb7-ac0f-f5ec8110e6f7"), Name: "Bedroom Blind", Description: "Motorized blackout blind", RoomID: &bedroomID, Meta: demoJSON(map[string]any{"map": map[string]any{"x": 168, "y": 343, "favorite_fields": []string{"position"}}})},
		{ID: mustUUID("f837b2f3-f01a-4aef-940b-d2f5cc785f95"), Name: "Desk Lamp", Description: "Task light on the office desk", RoomID: &officeID, Meta: demoJSON(map[string]any{"map": map[string]any{"x": 434, "y": 406, "favorite_fields": []string{"on", "brightness"}}})},
		{ID: mustUUID("0f22caa8-a112-4312-84c0-ec4896e8b6f3"), Name: "Entry Sensor", Description: "Door contact sensor at the main entrance", RoomID: &hallID, Meta: demoJSON(map[string]any{"map": map[string]any{"x": 644, "y": 476, "favorite_fields": []string{"contact", "battery"}}})},
		{ID: mustUUID("6ee50240-4763-49ba-b0f9-b0f9e29be6bf"), Name: "Air Purifier", Description: "Living room air purifier", RoomID: &livingRoomID, Meta: demoJSON(map[string]any{"map": map[string]any{"x": 203, "y": 231, "favorite_fields": []string{"on", "air_quality"}}})},
	}
}

func demoGroups() []demoGroupSeed {
	return []demoGroupSeed{
		{Group: Group{ID: mustUUID("f6127f16-9546-44a2-90a1-0fa0d7c26c33"), Slug: "evening-lights", Name: "Evening Lights", Description: "Ambient lights used after sunset", Meta: demoJSON(map[string]any{"kind": "lighting"})}, DeviceIDs: []uuid.UUID{mustUUID("0d47ed4e-6bb2-45b5-bbfc-c53ff2377701"), mustUUID("4d7a0d13-927f-46c0-88bc-eb5ff4988688"), mustUUID("56b56529-3223-4214-ba31-0f6e7e4d59fa"), mustUUID("f837b2f3-f01a-4aef-940b-d2f5cc785f95")}},
		{Group: Group{ID: mustUUID("2db1fb2f-2e9f-484c-b9df-9de45cdfee3d"), Slug: "comfort-zone", Name: "Comfort Zone", Description: "Soft lighting used during the day", Meta: demoJSON(map[string]any{"kind": "comfort"})}, DeviceIDs: []uuid.UUID{mustUUID("0d47ed4e-6bb2-45b5-bbfc-c53ff2377701"), mustUUID("4d7a0d13-927f-46c0-88bc-eb5ff4988688"), mustUUID("f837b2f3-f01a-4aef-940b-d2f5cc785f95")}},
		{Group: Group{ID: mustUUID("b7ab34d2-65ea-44ae-8b0f-3ad41201805c"), Slug: "arrival-lights", Name: "Arrival Lights", Description: "Quick entry lighting presets", Meta: demoJSON(map[string]any{"kind": "entry"})}, DeviceIDs: []uuid.UUID{mustUUID("56b56529-3223-4214-ba31-0f6e7e4d59fa"), mustUUID("f837b2f3-f01a-4aef-940b-d2f5cc785f95")}},
	}
}

func demoJSON(v any) datatypes.JSON {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return datatypes.JSON(b)
}

func mustUUID(raw string) uuid.UUID {
	id, err := uuid.Parse(raw)
	if err != nil {
		panic(err)
	}
	return id
}
