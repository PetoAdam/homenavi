package db

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GroupView struct {
	Group
	Devices        []DeviceView `json:"devices"`
	DeviceIDs      []uuid.UUID  `json:"device_ids"`
	HDPExternalIDs []string     `json:"hdp_external_ids"`
}

func (r *Repository) ListGroups(ctx context.Context) ([]GroupView, error) {
	var groups []Group
	if err := r.db.WithContext(ctx).Order("name asc").Find(&groups).Error; err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return []GroupView{}, nil
	}

	groupIDs := make([]uuid.UUID, 0, len(groups))
	for _, group := range groups {
		groupIDs = append(groupIDs, group.ID)
	}

	var members []GroupMember
	if err := r.db.WithContext(ctx).Where("group_id IN ?", groupIDs).Find(&members).Error; err != nil {
		return nil, err
	}

	deviceViewsByID, err := r.deviceViewsByIDs(ctx, uniqueGroupDeviceIDs(members))
	if err != nil {
		return nil, err
	}

	return buildGroupViews(groups, members, deviceViewsByID), nil
}

func (r *Repository) CreateGroup(ctx context.Context, group *Group) error {
	if group.ID == uuid.Nil {
		group.ID = uuid.New()
	}
	group.Slug = strings.TrimSpace(strings.ToLower(group.Slug))
	group.Name = strings.TrimSpace(group.Name)
	group.Description = strings.TrimSpace(group.Description)
	if group.Slug == "" || group.Name == "" {
		return errors.New("group.slug and group.name are required")
	}
	return r.db.WithContext(ctx).Create(group).Error
}

func (r *Repository) GetGroup(ctx context.Context, id uuid.UUID) (*Group, error) {
	var row Group
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) GetGroupView(ctx context.Context, id uuid.UUID) (*GroupView, error) {
	group, err := r.GetGroup(ctx, id)
	if err != nil {
		return nil, err
	}
	var members []GroupMember
	if err := r.db.WithContext(ctx).Where("group_id = ?", id).Find(&members).Error; err != nil {
		return nil, err
	}
	deviceViewsByID, err := r.deviceViewsByIDs(ctx, uniqueGroupDeviceIDs(members))
	if err != nil {
		return nil, err
	}
	views := buildGroupViews([]Group{*group}, members, deviceViewsByID)
	if len(views) == 0 {
		return &GroupView{Group: *group, Devices: []DeviceView{}, DeviceIDs: []uuid.UUID{}, HDPExternalIDs: []string{}}, nil
	}
	return &views[0], nil
}

func (r *Repository) UpdateGroup(ctx context.Context, id uuid.UUID, patch map[string]any) (*Group, error) {
	if len(patch) == 0 {
		return r.GetGroup(ctx, id)
	}
	if v, ok := patch["name"].(string); ok {
		patch["name"] = strings.TrimSpace(v)
	}
	if v, ok := patch["slug"].(string); ok {
		patch["slug"] = strings.TrimSpace(strings.ToLower(v))
	}
	if v, ok := patch["description"].(string); ok {
		patch["description"] = strings.TrimSpace(v)
	}
	if err := r.db.WithContext(ctx).Model(&Group{}).Where("id = ?", id).Updates(patch).Error; err != nil {
		return nil, err
	}
	return r.GetGroup(ctx, id)
}

func (r *Repository) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", id).Delete(&GroupMember{}).Error; err != nil {
			return err
		}
		return tx.Delete(&Group{}, "id = ?", id).Error
	})
}

func (r *Repository) SetGroupMembers(ctx context.Context, groupID uuid.UUID, deviceIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", groupID).Delete(&GroupMember{}).Error; err != nil {
			return err
		}
		seen := map[uuid.UUID]struct{}{}
		rows := make([]GroupMember, 0, len(deviceIDs))
		for _, deviceID := range deviceIDs {
			if deviceID == uuid.Nil {
				continue
			}
			if _, ok := seen[deviceID]; ok {
				continue
			}
			seen[deviceID] = struct{}{}
			rows = append(rows, GroupMember{GroupID: groupID, DeviceID: deviceID})
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

func uniqueGroupDeviceIDs(members []GroupMember) []uuid.UUID {
	seen := map[uuid.UUID]struct{}{}
	out := make([]uuid.UUID, 0, len(members))
	for _, member := range members {
		if member.DeviceID == uuid.Nil {
			continue
		}
		if _, ok := seen[member.DeviceID]; ok {
			continue
		}
		seen[member.DeviceID] = struct{}{}
		out = append(out, member.DeviceID)
	}
	return out
}

func buildGroupViews(groups []Group, members []GroupMember, deviceViewsByID map[uuid.UUID]DeviceView) []GroupView {
	groupDeviceIDs := map[uuid.UUID][]uuid.UUID{}
	for _, member := range members {
		groupDeviceIDs[member.GroupID] = append(groupDeviceIDs[member.GroupID], member.DeviceID)
	}

	out := make([]GroupView, 0, len(groups))
	for _, group := range groups {
		seen := map[uuid.UUID]struct{}{}
		deviceIDs := make([]uuid.UUID, 0)
		devices := make([]DeviceView, 0)
		hdpIDs := make([]string, 0)
		hdpSeen := map[string]struct{}{}
		for _, deviceID := range groupDeviceIDs[group.ID] {
			if deviceID == uuid.Nil {
				continue
			}
			if _, ok := seen[deviceID]; ok {
				continue
			}
			seen[deviceID] = struct{}{}
			deviceIDs = append(deviceIDs, deviceID)
			view, ok := deviceViewsByID[deviceID]
			if !ok {
				continue
			}
			devices = append(devices, view)
			for _, hdpID := range view.HDPExternalIDs {
				hdpID = strings.TrimSpace(hdpID)
				if hdpID == "" {
					continue
				}
				if _, exists := hdpSeen[hdpID]; exists {
					continue
				}
				hdpSeen[hdpID] = struct{}{}
				hdpIDs = append(hdpIDs, hdpID)
			}
		}
		sort.Slice(devices, func(i, j int) bool { return devices[i].Name < devices[j].Name })
		sort.Slice(deviceIDs, func(i, j int) bool { return deviceIDs[i].String() < deviceIDs[j].String() })
		sort.Strings(hdpIDs)
		out = append(out, GroupView{Group: group, Devices: devices, DeviceIDs: deviceIDs, HDPExternalIDs: hdpIDs})
	}
	return out
}

func (r *Repository) deviceViewsByIDs(ctx context.Context, deviceIDs []uuid.UUID) (map[uuid.UUID]DeviceView, error) {
	out := map[uuid.UUID]DeviceView{}
	if len(deviceIDs) == 0 {
		return out, nil
	}

	var devices []Device
	if err := r.db.WithContext(ctx).Where("id IN ?", deviceIDs).Find(&devices).Error; err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return out, nil
	}

	var deviceTags []DeviceTag
	if err := r.db.WithContext(ctx).Where("device_id IN ?", deviceIDs).Find(&deviceTags).Error; err != nil {
		return nil, err
	}
	tagIDs := make([]uuid.UUID, 0, len(deviceTags))
	seenTagIDs := map[uuid.UUID]struct{}{}
	for _, row := range deviceTags {
		if row.TagID == uuid.Nil {
			continue
		}
		if _, ok := seenTagIDs[row.TagID]; ok {
			continue
		}
		seenTagIDs[row.TagID] = struct{}{}
		tagIDs = append(tagIDs, row.TagID)
	}
	var tags []Tag
	if len(tagIDs) > 0 {
		if err := r.db.WithContext(ctx).Where("id IN ?", tagIDs).Order("name asc").Find(&tags).Error; err != nil {
			return nil, err
		}
	}
	tagByID := map[uuid.UUID]Tag{}
	for _, tag := range tags {
		tagByID[tag.ID] = tag
	}
	tagsByDeviceID := map[uuid.UUID][]Tag{}
	for _, row := range deviceTags {
		tag, ok := tagByID[row.TagID]
		if !ok {
			continue
		}
		tagsByDeviceID[row.DeviceID] = append(tagsByDeviceID[row.DeviceID], tag)
	}
	for deviceID := range tagsByDeviceID {
		rows := tagsByDeviceID[deviceID]
		sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
		tagsByDeviceID[deviceID] = rows
	}

	var bindings []DeviceBinding
	if err := r.db.WithContext(ctx).Where("kind = ? AND device_id IN ?", "hdp", deviceIDs).Find(&bindings).Error; err != nil {
		return nil, err
	}
	bindingsByDeviceID := map[uuid.UUID][]string{}
	for _, binding := range bindings {
		externalID, err := r.resolveBindingExternalID(ctx, binding)
		if err != nil {
			return nil, err
		}
		externalID = strings.TrimSpace(externalID)
		if externalID == "" {
			continue
		}
		bindingsByDeviceID[binding.DeviceID] = append(bindingsByDeviceID[binding.DeviceID], externalID)
	}
	for deviceID := range bindingsByDeviceID {
		rows := bindingsByDeviceID[deviceID]
		sort.Strings(rows)
		uniq := rows[:0]
		var last string
		for _, id := range rows {
			if id == last {
				continue
			}
			last = id
			uniq = append(uniq, id)
		}
		bindingsByDeviceID[deviceID] = uniq
	}

	for _, device := range devices {
		out[device.ID] = DeviceView{Device: device, Tags: tagsByDeviceID[device.ID], HDPExternalIDs: bindingsByDeviceID[device.ID]}
	}
	return out, nil
}
