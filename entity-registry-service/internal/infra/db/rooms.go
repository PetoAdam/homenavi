package db

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *Repository) ListRooms(ctx context.Context) ([]Room, error) {
	var rows []Room
	if err := r.db.WithContext(ctx).Order("sort_order asc, name asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) CreateRoom(ctx context.Context, room *Room) error {
	if room.ID == uuid.Nil {
		room.ID = uuid.New()
	}
	room.Slug = strings.TrimSpace(strings.ToLower(room.Slug))
	room.Name = strings.TrimSpace(room.Name)
	if room.Slug == "" || room.Name == "" {
		return errors.New("room.slug and room.name are required")
	}
	return r.db.WithContext(ctx).Create(room).Error
}

func (r *Repository) UpdateRoom(ctx context.Context, id uuid.UUID, patch map[string]any) (*Room, error) {
	if len(patch) == 0 {
		return r.GetRoom(ctx, id)
	}
	if v, ok := patch["name"].(string); ok {
		patch["name"] = strings.TrimSpace(v)
	}
	if v, ok := patch["slug"].(string); ok {
		patch["slug"] = strings.TrimSpace(strings.ToLower(v))
	}
	if err := r.db.WithContext(ctx).Model(&Room{}).Where("id = ?", id).Updates(patch).Error; err != nil {
		return nil, err
	}
	return r.GetRoom(ctx, id)
}

func (r *Repository) GetRoom(ctx context.Context, id uuid.UUID) (*Room, error) {
	var row Room
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) DeleteRoom(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Device{}).Where("room_id = ?", id).Update("room_id", nil).Error; err != nil {
			return err
		}
		return tx.Delete(&Room{}, "id = ?", id).Error
	})
}
