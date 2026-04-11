package db

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *Repository) ListTags(ctx context.Context) ([]Tag, error) {
	var rows []Tag
	if err := r.db.WithContext(ctx).Order("name asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) CreateTag(ctx context.Context, tag *Tag) error {
	if tag.ID == uuid.Nil {
		tag.ID = uuid.New()
	}
	tag.Slug = strings.TrimSpace(strings.ToLower(tag.Slug))
	tag.Name = strings.TrimSpace(tag.Name)
	if tag.Slug == "" || tag.Name == "" {
		return errors.New("tag.slug and tag.name are required")
	}
	return r.db.WithContext(ctx).Create(tag).Error
}

func (r *Repository) GetTag(ctx context.Context, id uuid.UUID) (*Tag, error) {
	var row Tag
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) DeleteTag(ctx context.Context, id uuid.UUID) error {
	_ = r.db.WithContext(ctx).Where("tag_id = ?", id).Delete(&DeviceTag{}).Error
	return r.db.WithContext(ctx).Delete(&Tag{}, "id = ?", id).Error
}

func (r *Repository) SetTagMembers(ctx context.Context, tagID uuid.UUID, deviceIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tag_id = ?", tagID).Delete(&DeviceTag{}).Error; err != nil {
			return err
		}
		if len(deviceIDs) == 0 {
			return nil
		}
		seen := map[uuid.UUID]struct{}{}
		rows := make([]DeviceTag, 0, len(deviceIDs))
		for _, did := range deviceIDs {
			if did == uuid.Nil {
				continue
			}
			if _, ok := seen[did]; ok {
				continue
			}
			seen[did] = struct{}{}
			rows = append(rows, DeviceTag{DeviceID: did, TagID: tagID})
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}
