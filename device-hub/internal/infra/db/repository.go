package db

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	model "github.com/PetoAdam/homenavi/device-hub/internal/devices"
	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
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

type Repository struct {
	db *gorm.DB
}

type DeviceState struct {
	DeviceID  string          `gorm:"primaryKey;type:uuid"`
	State     json.RawMessage `gorm:"type:jsonb"`
	UpdatedAt time.Time
}

func Open(cfg Config) (*Repository, error) {
	dsn := dbx.BuildPostgresDSN(dbx.PostgresConfig{Host: cfg.Host, User: cfg.User, Password: cfg.Password, DBName: cfg.DBName, Port: cfg.Port, SSLMode: cfg.SSLMode})
	gormLogger := logger.New(
		log.New(os.Stdout, "", log.LstdFlags),
		logger.Config{SlowThreshold: 2 * time.Second, LogLevel: logger.Warn, IgnoreRecordNotFoundError: true, Colorful: false},
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormLogger})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.Device{}, &DeviceState{}); err != nil {
		return nil, err
	}
	return &Repository{db: db}, nil
}

func (r *Repository) UpsertDevice(ctx context.Context, d *model.Device) error {
	d.UpdatedAt = time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = d.UpdatedAt
	}
	return r.db.WithContext(ctx).Save(d).Error
}

func (r *Repository) GetByExternal(ctx context.Context, protocol, externalID string) (*model.Device, error) {
	var dev model.Device
	if err := r.db.WithContext(ctx).Where(&model.Device{Protocol: protocol, ExternalID: externalID}).First(&dev).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &dev, nil
}

func (r *Repository) List(ctx context.Context) ([]model.Device, error) {
	var devices []model.Device
	if err := r.db.WithContext(ctx).Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*model.Device, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return nil, nil
	}
	var dev model.Device
	if err := r.db.WithContext(ctx).First(&dev, &model.Device{ID: parsed}).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &dev, nil
}

func (r *Repository) TouchOnline(ctx context.Context, id interface{}) error {
	return r.db.WithContext(ctx).Model(&model.Device{}).Where(map[string]any{"id": id}).Updates(map[string]any{"online": true, "last_seen": time.Now().UTC()}).Error
}

func (r *Repository) SaveDeviceState(ctx context.Context, deviceID string, state json.RawMessage) error {
	ds := &DeviceState{DeviceID: deviceID, State: state, UpdatedAt: time.Now().UTC()}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "device_id"}}, DoUpdates: clause.AssignmentColumns([]string{"state", "updated_at"})}).Create(ds).Error
}

func (r *Repository) GetDeviceState(ctx context.Context, deviceID string) (json.RawMessage, error) {
	var ds DeviceState
	if err := r.db.WithContext(ctx).First(&ds, &DeviceState{DeviceID: deviceID}).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return ds.State, nil
}

func (r *Repository) DeleteDeviceAndState(ctx context.Context, id string) error {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&DeviceState{}, &DeviceState{DeviceID: parsed.String()}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.Device{}, &model.Device{ID: parsed}).Error; err != nil {
			return err
		}
		return nil
	})
}
