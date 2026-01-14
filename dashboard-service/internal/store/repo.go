package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

func New(db *gorm.DB) (*Repo, error) {
	// NOTE: GORM AutoMigrate has been observed to fail in some containerized
	// environments with an "insufficient arguments" error during schema probing.
	// This service uses a small, stable schema, so we create it explicitly.
	if err := ensureSchema(db); err != nil {
		return nil, err
	}
	return &Repo{db: db}, nil
}

func ensureSchema(db *gorm.DB) error {
	// Keep SQL simple and idempotent.
	// created_at/updated_at are managed by the application layer for now.
	if err := db.Exec(`
CREATE TABLE IF NOT EXISTS dashboards (
  id uuid PRIMARY KEY,
  scope varchar(16) NOT NULL,
  owner_user_id uuid NULL,
  title varchar(128) NOT NULL,
  layout_engine varchar(32) NOT NULL,
  layout_version integer NOT NULL DEFAULT 1,
  doc jsonb NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
`).Error; err != nil {
		return err
	}

	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_dashboards_scope ON dashboards(scope);`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_dashboards_owner_user_id ON dashboards(owner_user_id);`).Error; err != nil {
		return err
	}
	return nil
}

func (r *Repo) GetDefaultDashboard(ctx context.Context) (*Dashboard, error) {
	var d Dashboard
	err := r.db.WithContext(ctx).
		Where("scope = ? AND owner_user_id IS NULL", "default").
		First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *Repo) GetUserDashboard(ctx context.Context, userID uuid.UUID) (*Dashboard, error) {
	var d Dashboard
	err := r.db.WithContext(ctx).
		Where("scope = ? AND owner_user_id = ?", "user", userID).
		First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *Repo) CreateDashboard(ctx context.Context, d *Dashboard) error {
	return r.db.WithContext(ctx).Create(d).Error
}

func (r *Repo) UpdateUserDashboardDoc(ctx context.Context, userID uuid.UUID, expectedVersion int, nextDoc datatypes.JSON) (*Dashboard, error) {
	// Optimistic concurrency: update only if version matches.
	res := r.db.WithContext(ctx).
		Model(&Dashboard{}).
		Where("scope = ? AND owner_user_id = ? AND layout_version = ?", "user", userID, expectedVersion).
		Updates(map[string]any{
			"doc":            nextDoc,
			"layout_version": expectedVersion + 1,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	return r.GetUserDashboard(ctx, userID)
}

func (r *Repo) UpsertDefaultDashboard(ctx context.Context, title string, doc any) (*Dashboard, error) {
	existing, err := r.GetDefaultDashboard(ctx)
	if err != nil {
		return nil, err
	}
	buf, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		d := &Dashboard{
			ID:            uuid.New(),
			Scope:         "default",
			OwnerUserID:   nil,
			Title:         title,
			LayoutEngine:  "rgl-v1",
			LayoutVersion: 1,
			Doc:           datatypes.JSON(buf),
		}
		if err := r.CreateDashboard(ctx, d); err != nil {
			return nil, err
		}
		return d, nil
	}
	// Replace default doc; bump version.
	if err := r.db.WithContext(ctx).
		Model(&Dashboard{}).
		Where("id = ?", existing.ID).
		Updates(map[string]any{"doc": datatypes.JSON(buf), "title": title, "layout_version": existing.LayoutVersion + 1}).Error; err != nil {
		return nil, err
	}
	return r.GetDefaultDashboard(ctx)
}
