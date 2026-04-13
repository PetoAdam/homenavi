package db

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DeviceView is an enriched projection of a Device with resolved tags and HDP bindings.
type DeviceView struct {
	Device
	Tags           []Tag    `json:"tags"`
	HDPExternalIDs []string `json:"hdp_external_ids"`
}

func (r *Repository) ListDevices(ctx context.Context) ([]DeviceView, error) {
	var devices []Device
	if err := r.db.WithContext(ctx).Order("created_at desc").Find(&devices).Error; err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return []DeviceView{}, nil
	}

	deviceIDs := make([]uuid.UUID, 0, len(devices))
	for _, d := range devices {
		deviceIDs = append(deviceIDs, d.ID)
	}

	var deviceTags []DeviceTag
	if err := r.db.WithContext(ctx).Where("device_id IN ?", deviceIDs).Find(&deviceTags).Error; err != nil {
		return nil, err
	}

	tagIDs := make([]uuid.UUID, 0, len(deviceTags))
	seenTag := map[uuid.UUID]struct{}{}
	for _, dt := range deviceTags {
		if dt.TagID == uuid.Nil {
			continue
		}
		if _, ok := seenTag[dt.TagID]; ok {
			continue
		}
		seenTag[dt.TagID] = struct{}{}
		tagIDs = append(tagIDs, dt.TagID)
	}

	var tags []Tag
	if len(tagIDs) > 0 {
		if err := r.db.WithContext(ctx).Where("id IN ?", tagIDs).Order("name asc").Find(&tags).Error; err != nil {
			return nil, err
		}
	}
	tagByID := map[uuid.UUID]Tag{}
	for _, t := range tags {
		tagByID[t.ID] = t
	}
	deviceTagsByDeviceID := map[uuid.UUID][]Tag{}
	for _, dt := range deviceTags {
		t, ok := tagByID[dt.TagID]
		if !ok {
			continue
		}
		deviceTagsByDeviceID[dt.DeviceID] = append(deviceTagsByDeviceID[dt.DeviceID], t)
	}
	for did := range deviceTagsByDeviceID {
		rows := deviceTagsByDeviceID[did]
		sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
		deviceTagsByDeviceID[did] = rows
	}

	var bindings []DeviceBinding
	if err := r.db.WithContext(ctx).Where("kind = ? AND device_id IN ?", "hdp", deviceIDs).Find(&bindings).Error; err != nil {
		return nil, err
	}
	deviceBindingsByDeviceID := map[uuid.UUID][]string{}
	for _, b := range bindings {
		x := strings.TrimSpace(b.ExternalID)
		if x == "" {
			continue
		}
		deviceBindingsByDeviceID[b.DeviceID] = append(deviceBindingsByDeviceID[b.DeviceID], x)
	}
	for did := range deviceBindingsByDeviceID {
		rows := deviceBindingsByDeviceID[did]
		sort.Strings(rows)
		uniq := rows[:0]
		var last string
		for _, x := range rows {
			if x == last {
				continue
			}
			last = x
			uniq = append(uniq, x)
		}
		deviceBindingsByDeviceID[did] = uniq
	}

	out := make([]DeviceView, 0, len(devices))
	for _, d := range devices {
		out = append(out, DeviceView{Device: d, Tags: deviceTagsByDeviceID[d.ID], HDPExternalIDs: deviceBindingsByDeviceID[d.ID]})
	}
	return out, nil
}

func (r *Repository) CreateDevice(ctx context.Context, dev *Device) error {
	if dev.ID == uuid.Nil {
		dev.ID = uuid.New()
	}
	dev.Name = strings.TrimSpace(dev.Name)
	if dev.Name == "" {
		return errors.New("device.name is required")
	}
	dev.Description = strings.TrimSpace(dev.Description)
	return r.db.WithContext(ctx).Create(dev).Error
}

func (r *Repository) GetDevice(ctx context.Context, id uuid.UUID) (*Device, error) {
	var row Device
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) GetDeviceView(ctx context.Context, id uuid.UUID) (*DeviceView, error) {
	dev, err := r.GetDevice(ctx, id)
	if err != nil {
		return nil, err
	}
	tagsTable := Tag{}.TableName()
	deviceTagsTable := DeviceTag{}.TableName()
	var tags []Tag
	if err := r.db.WithContext(ctx).
		Table(tagsTable).
		Select(tagsTable+".*").
		Joins("join "+deviceTagsTable+" on "+deviceTagsTable+".tag_id = "+tagsTable+".id").
		Where(deviceTagsTable+".device_id = ?", id).
		Order(tagsTable + ".name asc").
		Find(&tags).Error; err != nil {
		return nil, err
	}
	var bindings []DeviceBinding
	if err := r.db.WithContext(ctx).Where("device_id = ? AND kind = ?", id, "hdp").Find(&bindings).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(bindings))
	for _, b := range bindings {
		if strings.TrimSpace(b.ExternalID) == "" {
			continue
		}
		ids = append(ids, b.ExternalID)
	}
	sort.Strings(ids)
	uniq := ids[:0]
	var last string
	for _, x := range ids {
		if x == last {
			continue
		}
		last = x
		uniq = append(uniq, x)
	}
	ids = uniq
	return &DeviceView{Device: *dev, Tags: tags, HDPExternalIDs: ids}, nil
}

func (r *Repository) UpdateDevice(ctx context.Context, id uuid.UUID, patch map[string]any) (*Device, error) {
	if v, ok := patch["name"].(string); ok {
		patch["name"] = strings.TrimSpace(v)
	}
	if v, ok := patch["description"].(string); ok {
		patch["description"] = strings.TrimSpace(v)
	}
	if err := r.db.WithContext(ctx).Model(&Device{}).Where("id = ?", id).Updates(patch).Error; err != nil {
		return nil, err
	}
	return r.GetDevice(ctx, id)
}

func (r *Repository) DeleteDevice(ctx context.Context, id uuid.UUID) error {
	_ = r.db.WithContext(ctx).Where("device_id = ?", id).Delete(&DeviceTag{}).Error
	_ = r.db.WithContext(ctx).Where("device_id = ?", id).Delete(&DeviceBinding{}).Error
	return r.db.WithContext(ctx).Delete(&Device{}, "id = ?", id).Error
}

func (r *Repository) SetDeviceTags(ctx context.Context, deviceID uuid.UUID, tagIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("device_id = ?", deviceID).Delete(&DeviceTag{}).Error; err != nil {
			return err
		}
		seen := map[uuid.UUID]struct{}{}
		rows := make([]DeviceTag, 0, len(tagIDs))
		for _, tid := range tagIDs {
			if tid == uuid.Nil {
				continue
			}
			if _, ok := seen[tid]; ok {
				continue
			}
			seen[tid] = struct{}{}
			rows = append(rows, DeviceTag{DeviceID: deviceID, TagID: tid})
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}
