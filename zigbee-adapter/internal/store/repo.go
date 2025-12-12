package store

import (
    "context"
    "encoding/json"
    "errors"
    "log/slog"
    "slices"
    "time"

    "zigbee-adapter/internal/model"

    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    "gorm.io/gorm/clause"
)

type Repository struct {
    db *gorm.DB
}

type DeviceState struct {
    DeviceID  string          `gorm:"primaryKey;type:uuid"`
    State     json.RawMessage `gorm:"type:jsonb"`
    UpdatedAt time.Time
}

func NewRepository(dsn string) (*Repository, error) {
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
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
    if err := r.db.WithContext(ctx).Where("protocol=? AND external_id=?", protocol, externalID).First(&dev).Error; err != nil {
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
    if err := r.db.WithContext(ctx).First(&dev, "id=?", id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, nil
        }
        return nil, err
    }
    return &dev, nil
}

func (r *Repository) TouchOnline(ctx context.Context, id interface{}) error {
    return r.db.WithContext(ctx).Model(&model.Device{}).Where("id=?", id).Updates(map[string]any{"online": true, "last_seen": time.Now().UTC()}).Error
}

func (r *Repository) SetOfflineOlderThan(ctx context.Context, olderThan time.Duration) error {
    cutoff := time.Now().Add(-olderThan)
    res := r.db.WithContext(ctx).Model(&model.Device{}).Where("last_seen < ? AND online = ?", cutoff, true).Update("online", false)
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
    if err := r.db.WithContext(ctx).First(&ds, "device_id=?", deviceID).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, nil
        }
        return nil, err
    }
    return ds.State, nil
}

func (r *Repository) DeleteDeviceAndState(ctx context.Context, id string) error {
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Delete(&DeviceState{}, "device_id = ?", id).Error; err != nil {
            return err
        }
        if err := tx.Delete(&model.Device{}, "id = ?", id).Error; err != nil {
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
        query := tx.Where("protocol = ?", protocol)
        switch op {
        case "NOT IN":
            if len(externalIDs) > 0 {
                query = query.Where("external_id NOT IN ?", externalIDs)
            }
        case "=":
            if len(externalIDs) > 0 {
                query = query.Where("external_id = ?", externalIDs[0])
            }
        default:
            return errors.New("unsupported delete operation")
        }
        if len(keepIDs) > 0 {
            query = query.Where("id NOT IN ?", keepIDs)
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
        if err := tx.Delete(&DeviceState{}, "device_id IN ?", ids).Error; err != nil {
            return err
        }
        if err := tx.Delete(&model.Device{}, "id IN ?", ids).Error; err != nil {
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
            query = query.Where("device_id NOT IN ?", keepIDs)
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
        if err := tx.Delete(&DeviceState{}, "device_id IN ?", ids).Error; err != nil {
            return err
        }
        return nil
    })
    return ids, err
}
