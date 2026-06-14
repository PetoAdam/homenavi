package db

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
)

func (r *Repository) ResolveSelectorToHDP(ctx context.Context, selector string) ([]string, []uuid.UUID, error) {
	targets, deviceIDs, err := r.ResolveSelectorToHDPTargets(ctx, selector)
	if err != nil {
		return nil, nil, err
	}
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		if strings.TrimSpace(target.ExternalID) == "" {
			continue
		}
		out = append(out, target.ExternalID)
	}
	return out, deviceIDs, nil
}

func (r *Repository) ResolveSelectorToHDPTargets(ctx context.Context, selector string) ([]HDPTarget, []uuid.UUID, error) {
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
			return []HDPTarget{}, []uuid.UUID{}, nil
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
				return []HDPTarget{}, []uuid.UUID{}, nil
			}
			var devs []Device
			if err := r.db.WithContext(ctx).Where("room_id = ?", room.ID).Find(&devs).Error; err != nil {
				return nil, nil, err
			}
			for _, d := range devs {
				deviceIDs = append(deviceIDs, d.ID)
			}
		}
	case "group":
		var group Group
		if id, err := uuid.Parse(arg); err == nil {
			if err := r.db.WithContext(ctx).First(&group, "id = ?", id).Error; err != nil {
				return []HDPTarget{}, []uuid.UUID{}, nil
			}
		} else {
			if err := r.db.WithContext(ctx).First(&group, "slug = ?", strings.ToLower(arg)).Error; err != nil {
				return []HDPTarget{}, []uuid.UUID{}, nil
			}
		}
		var rows []GroupMember
		if err := r.db.WithContext(ctx).Where("group_id = ?", group.ID).Find(&rows).Error; err != nil {
			return nil, nil, err
		}
		for _, row := range rows {
			deviceIDs = append(deviceIDs, row.DeviceID)
		}
	default:
		return nil, nil, errors.New("unsupported selector kind")
	}

	if len(deviceIDs) == 0 {
		return []HDPTarget{}, []uuid.UUID{}, nil
	}

	targets, err := r.resolveHDPTargetsForDevices(ctx, deviceIDs)
	if err != nil {
		return nil, nil, err
	}
	return targets, deviceIDs, nil
}

func (r *Repository) resolveHDPTargetsForDevices(ctx context.Context, deviceIDs []uuid.UUID) ([]HDPTarget, error) {
	var bindings []DeviceBinding
	if err := r.db.WithContext(ctx).Where("kind = ? AND device_id IN ?", "hdp", deviceIDs).Find(&bindings).Error; err != nil {
		return nil, err
	}
	targets := make([]HDPTarget, 0, len(bindings))
	seen := map[string]struct{}{}
	for _, binding := range bindings {
		externalID, err := r.resolveBindingExternalID(ctx, binding)
		if err != nil {
			return nil, err
		}
		if externalID == "" {
			continue
		}
		key := externalID
		if binding.HDPDeviceID != nil {
			key = binding.HDPDeviceID.String() + "|" + externalID
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, HDPTarget{ExternalID: externalID, HDPDeviceID: binding.HDPDeviceID})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].ExternalID < targets[j].ExternalID })
	return targets, nil
}

func (r *Repository) resolveBindingExternalID(ctx context.Context, binding DeviceBinding) (string, error) {
	externalID := strings.TrimSpace(binding.ExternalID)
	if binding.HDPDeviceID == nil {
		return externalID, nil
	}
	device, err := loadHDPDeviceByID(r.db.WithContext(ctx), *binding.HDPDeviceID)
	if err != nil {
		return "", err
	}
	if device == nil {
		return externalID, nil
	}
	resolved := splitJoinHDPRef(device.Protocol, device.ExternalID)
	if strings.TrimSpace(resolved) == "" {
		return externalID, nil
	}
	return resolved, nil
}

func splitJoinHDPRef(protocol, externalID string) string {
	protocol = strings.TrimSpace(protocol)
	externalID = strings.TrimSpace(externalID)
	if protocol == "" {
		return externalID
	}
	if externalID == "" {
		return ""
	}
	return protocol + "/" + externalID
}
