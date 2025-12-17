package store

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"os"
	"slices"
	"time"

	"zigbee-adapter/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type Repository struct {
	db *gorm.DB
}

func toAnySlice(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

type DeviceState struct {
	DeviceID  string          `gorm:"primaryKey;type:uuid"`
	State     json.RawMessage `gorm:"type:jsonb"`
	UpdatedAt time.Time
}

func NewRepository(dsn string) (*Repository, error) {
	gormLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             2 * time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
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
	var dev model.Device
	if err := r.db.WithContext(ctx).First(&dev, id).Error; err != nil {
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

func (r *Repository) SetOfflineOlderThan(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	res := r.db.WithContext(ctx).
		Model(&model.Device{}).
		Where(clause.Lt{Column: clause.Column{Name: "last_seen"}, Value: cutoff}).
		Where(map[string]any{"online": true}).
		Update("online", false)
	if res.Error != nil {
		slog.Error("offline update error", "error", res.Error)
	}
	return res.Error
}

func (r *Repository) SaveDeviceState(ctx context.Context, deviceID string, state json.RawMessage) error {
	ds := &DeviceState{DeviceID: deviceID, State: state, UpdatedAt: time.Now().UTC()}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "device_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"state", "updated_at"}),
	}).Create(ds).Error
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
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&DeviceState{}, id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.Device{}, id).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *Repository) DeleteDevicesNotIn(ctx context.Context, protocol string, keepExternalIDs []string) ([]model.Device, error) {
	idsToKeep := slices.Compact(append([]string(nil), keepExternalIDs...))
	return r.deleteMatching(ctx, protocol, idsToKeep, "NOT IN")
}

func (r *Repository) DeleteDuplicatesByExternal(ctx context.Context, protocol, externalID, keepID string) ([]model.Device, error) {
	return r.deleteMatching(ctx, protocol, []string{externalID}, "=", keepID)
}

func (r *Repository) deleteMatching(ctx context.Context, protocol string, externalIDs []string, op string, keepIDs ...string) ([]model.Device, error) {
	if protocol == "" {
		return nil, nil
	}
	var removed []model.Device
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&model.Device{}).Where(&model.Device{Protocol: protocol})
		switch op {
		case "NOT IN":
			if len(externalIDs) > 0 {
				query = query.Where(clause.Not(clause.IN{Column: clause.Column{Name: "external_id"}, Values: toAnySlice(externalIDs)}))
			}
		case "=":
			if len(externalIDs) > 0 {
				query = query.Where(&model.Device{ExternalID: externalIDs[0]})
			}
		default:
			return errors.New("unsupported delete operation")
		}
		if len(keepIDs) > 0 {
			query = query.Where(clause.Not(clause.IN{Column: clause.Column{Name: "id"}, Values: toAnySlice(keepIDs)}))
		}
		if err := query.Find(&removed).Error; err != nil {
			return err
		}
		if len(removed) == 0 {
			return nil
		}
		ids := make([]string, len(removed))
		for i, dev := range removed {
			ids[i] = dev.ID.String()
		}
		if err := tx.Where(clause.IN{Column: clause.Column{Name: "device_id"}, Values: toAnySlice(ids)}).Delete(&DeviceState{}).Error; err != nil {
			return err
		}
		if err := tx.Where(clause.IN{Column: clause.Column{Name: "id"}, Values: toAnySlice(ids)}).Delete(&model.Device{}).Error; err != nil {
			return err
		}
		return nil
	})
	return removed, err
}

func (r *Repository) DeleteDeviceStatesNotIn(ctx context.Context, keepIDs []string) ([]string, error) {
	removed := []DeviceState{}
	ids := []string{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&DeviceState{})
		if len(keepIDs) > 0 {
			query = query.Where(clause.Not(clause.IN{Column: clause.Column{Name: "device_id"}, Values: toAnySlice(keepIDs)}))
		}
		if err := query.Find(&removed).Error; err != nil {
			return err
		}
		if len(removed) == 0 {
			return nil
		}
		ids = make([]string, len(removed))
		for i, ds := range removed {
			ids[i] = ds.DeviceID
		}
		if err := tx.Where(clause.IN{Column: clause.Column{Name: "device_id"}, Values: toAnySlice(ids)}).Delete(&DeviceState{}).Error; err != nil {
			return err
		}
		return nil
	})
	return ids, err
}
