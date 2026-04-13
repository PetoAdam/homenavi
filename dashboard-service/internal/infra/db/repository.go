package db

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/PetoAdam/homenavi/dashboard-service/internal/dashboard"
	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Config holds database connectivity settings for the current SQL backend.
type Config struct {
	User     string
	Password string
	DBName   string
	Host     string
	Port     string
	SSLMode  string
}

// Repository is the database-backed dashboard repository implementation.
type Repository struct {
	db *gorm.DB
}

func New(cfg Config, logger *slog.Logger) (*Repository, error) {
	dsn := dbx.BuildPostgresDSN(dbx.PostgresConfig{Host: cfg.Host, Port: cfg.Port, User: cfg.User, Password: cfg.Password, DBName: cfg.DBName, SSLMode: cfg.SSLMode})
	var db *gorm.DB
	var err error
	for i := 0; i < 30; i++ {
		db, err = gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{})
		if err == nil {
			if err = ensureSchema(db); err == nil {
				break
			}
		}
		if i == 29 {
			return nil, err
		}
		if logger != nil {
			logger.Warn("waiting for database", "attempt", i+1, "max", 30, "error", err)
		}
		time.Sleep(2 * time.Second)
	}
	return &Repository{db: db}, nil
}

func ensureSchema(db *gorm.DB) error {
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

func (r *Repository) GetDefaultDashboard(ctx context.Context) (*dashboard.Dashboard, error) {
	var d dashboardRecord
	err := r.db.WithContext(ctx).Where("scope = ? AND owner_user_id IS NULL", "default").First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := d.toDomain()
	return &result, nil
}

func (r *Repository) GetUserDashboard(ctx context.Context, userID uuid.UUID) (*dashboard.Dashboard, error) {
	var d dashboardRecord
	err := r.db.WithContext(ctx).Where("scope = ? AND owner_user_id = ?", "user", userID).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := d.toDomain()
	return &result, nil
}

func (r *Repository) CreateDashboard(ctx context.Context, d *dashboard.Dashboard) error {
	record := fromDomain(*d)
	return r.db.WithContext(ctx).Create(&record).Error
}

func (r *Repository) UpdateUserDashboardDoc(ctx context.Context, userID uuid.UUID, expectedVersion int, nextDoc datatypes.JSON) (*dashboard.Dashboard, error) {
	res := r.db.WithContext(ctx).Model(&dashboardRecord{}).Where("scope = ? AND owner_user_id = ? AND layout_version = ?", "user", userID, expectedVersion).Updates(map[string]any{"doc": nextDoc, "layout_version": expectedVersion + 1})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	return r.GetUserDashboard(ctx, userID)
}

func (r *Repository) UpsertDefaultDashboard(ctx context.Context, title string, doc any) (*dashboard.Dashboard, error) {
	existing, err := r.GetDefaultDashboard(ctx)
	if err != nil {
		return nil, err
	}
	buf, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		d := &dashboard.Dashboard{ID: uuid.New(), Scope: "default", Title: title, LayoutEngine: "rgl-v1", LayoutVersion: 1, Doc: datatypes.JSON(buf)}
		if err := r.CreateDashboard(ctx, d); err != nil {
			return nil, err
		}
		return d, nil
	}
	if err := r.db.WithContext(ctx).Model(&dashboardRecord{}).Where("id = ?", existing.ID).Updates(map[string]any{"doc": datatypes.JSON(buf), "title": title, "layout_version": existing.LayoutVersion + 1}).Error; err != nil {
		return nil, err
	}
	return r.GetDefaultDashboard(ctx)
}

type dashboardRecord struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey"`
	Scope         string         `gorm:"type:varchar(16);index"`
	OwnerUserID   *uuid.UUID     `gorm:"type:uuid;index"`
	Title         string         `gorm:"type:varchar(128)"`
	LayoutEngine  string         `gorm:"type:varchar(32)"`
	LayoutVersion int            `gorm:"not null;default:1"`
	Doc           datatypes.JSON `gorm:"type:jsonb"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (dashboardRecord) TableName() string {
	return "dashboards"
}

func (r dashboardRecord) toDomain() dashboard.Dashboard {
	return dashboard.Dashboard{ID: r.ID, Scope: r.Scope, OwnerUserID: r.OwnerUserID, Title: r.Title, LayoutEngine: r.LayoutEngine, LayoutVersion: r.LayoutVersion, Doc: r.Doc, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func fromDomain(d dashboard.Dashboard) dashboardRecord {
	return dashboardRecord{ID: d.ID, Scope: d.Scope, OwnerUserID: d.OwnerUserID, Title: d.Title, LayoutEngine: d.LayoutEngine, LayoutVersion: d.LayoutVersion, Doc: d.Doc, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
}

var _ dashboard.Repository = (*Repository)(nil)
