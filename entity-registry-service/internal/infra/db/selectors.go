package db

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

func (r *Repository) ResolveSelectorToHDP(ctx context.Context, selector string) ([]string, []uuid.UUID, error) {
	sel := strings.TrimSpace(selector)
	if sel == "" {
		return nil, nil, errors.New("selector is required")
	}
	parts := strings.SplitN(sel, ":", 2)
	if len(parts) != 2 {
		return nil, nil, errors.New("invalid selector format")
	}
	kind := strings.ToLower(strings.TrimSpace(parts[0]))
	arg := strings.TrimSpace(parts[1])
	if arg == "" {
		return nil, nil, errors.New("invalid selector")
	}

	var deviceIDs []uuid.UUID
	switch kind {
	case "tag":
		var tag Tag
		if err := r.db.WithContext(ctx).First(&tag, "slug = ?", strings.ToLower(arg)).Error; err != nil {
			return []string{}, []uuid.UUID{}, nil
		}
		var rows []DeviceTag
		if err := r.db.WithContext(ctx).Where("tag_id = ?", tag.ID).Find(&rows).Error; err != nil {
			return nil, nil, err
		}
		for _, row := range rows {
			deviceIDs = append(deviceIDs, row.DeviceID)
		}
	case "room":
		if id, err := uuid.Parse(arg); err == nil {
			var devs []Device
			if err := r.db.WithContext(ctx).Where("room_id = ?", id).Find(&devs).Error; err != nil {
				return nil, nil, err
			}
			for _, d := range devs {
				deviceIDs = append(deviceIDs, d.ID)
			}
		} else {
			var room Room
			if err := r.db.WithContext(ctx).First(&room, "slug = ?", strings.ToLower(arg)).Error; err != nil {
				return []string{}, []uuid.UUID{}, nil
			}
			var devs []Device
			if err := r.db.WithContext(ctx).Where("room_id = ?", room.ID).Find(&devs).Error; err != nil {
				return nil, nil, err
			}
			for _, d := range devs {
				deviceIDs = append(deviceIDs, d.ID)
			}
		}
	default:
		return nil, nil, errors.New("unsupported selector kind")
	}

	if len(deviceIDs) == 0 {
		return []string{}, []uuid.UUID{}, nil
	}

	var bindings []DeviceBinding
	if err := r.db.WithContext(ctx).Where("kind = ? AND device_id IN ?", "hdp", deviceIDs).Find(&bindings).Error; err != nil {
		return nil, nil, err
	}

	out := make([]string, 0, len(bindings))
	seen := map[string]struct{}{}
	for _, b := range bindings {
		id := strings.TrimSpace(b.ExternalID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, deviceIDs, nil
}
