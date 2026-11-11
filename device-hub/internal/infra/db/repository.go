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
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// Config holds database connectivity settings for the current SQL backend.
type Config = dbx.PostgresConfig

type Repository struct {
	db *gorm.DB
}

func New(database *gorm.DB) *Repository {
	return &Repository{db: database}
}

type DeviceState struct {
	DeviceID  string          `gorm:"primaryKey;type:uuid"`
	State     json.RawMessage `gorm:"type:jsonb;not null;default:'{}'"`
	UpdatedAt time.Time
}

func (DeviceState) TableName() string { return "hdp_device_states" }

type DeviceStateKey struct {
	Protocol   string
	ExternalID string
}

type DeviceStateView struct {
	Protocol       string
	ExternalID     string
	Online         bool
	LastSeen       time.Time
	State          json.RawMessage
	StateUpdatedAt *time.Time
}

func (r *Repository) GetDeviceStates(ctx context.Context, keys []DeviceStateKey) ([]DeviceStateView, error) {
	if len(keys) == 0 {
		return []DeviceStateView{}, nil
	}
	query := r.db.WithContext(ctx).Table("hdp_devices AS devices").
		Select("devices.protocol, devices.external_id, devices.online, devices.last_seen, states.state, states.updated_at AS state_updated_at").
		Joins("LEFT JOIN hdp_device_states AS states ON states.device_id = devices.id")
	conditions := r.db.Where("1 = 0")
	for _, key := range keys {
		conditions = conditions.Or("(devices.protocol = ? AND devices.external_id = ?)", key.Protocol, key.ExternalID)
	}
	var result []DeviceStateView
	if err := query.Where(conditions).Scan(&result).Error; err != nil {
		return nil, err
	}
	return result, nil
}

type CommandLifecycle struct {
	CorrelationID string          `gorm:"primaryKey;size:128"`
	DeviceID      string          `gorm:"index;not null"`
	ExternalID    string          `gorm:"index;not null"`
	Status        string          `gorm:"size:32;not null"`
	Version       int64           `gorm:"not null;default:1"`
	Expected      json.RawMessage `gorm:"type:jsonb;not null;default:'{}'"`
	Baseline      json.RawMessage `gorm:"type:jsonb;not null;default:'{}'"`
	Error         string          `gorm:"type:text"`
	StartedAt     time.Time       `gorm:"not null"`
	ExpiresAt     time.Time       `gorm:"not null"`
	TerminalAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (CommandLifecycle) TableName() string { return "hdp_command_lifecycles" }

type PairingLifecycle struct {
	ID         string          `gorm:"primaryKey;size:128"`
	Protocol   string          `gorm:"index;not null"`
	Status     string          `gorm:"size:32;not null"`
	Active     bool            `gorm:"not null;default:true"`
	Version    int64           `gorm:"not null;default:1"`
	Session    json.RawMessage `gorm:"type:jsonb;not null;default:'{}'"`
	StartedAt  time.Time       `gorm:"not null"`
	ExpiresAt  time.Time       `gorm:"not null"`
	TerminalAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (PairingLifecycle) TableName() string { return "hdp_pairing_lifecycles" }

type LifecycleOutbox struct {
	ID          uint64    `gorm:"primaryKey"`
	Topic       string    `gorm:"not null"`
	Payload     []byte    `gorm:"type:jsonb;not null"`
	CreatedAt   time.Time `gorm:"not null"`
	PublishedAt *time.Time
}

func (LifecycleOutbox) TableName() string { return "hdp_lifecycle_outbox" }

func Open(cfg Config) (*Repository, error) {
	dsn := dbx.BuildPostgresDSN(cfg)
	gormLogger := logger.New(
		log.New(os.Stdout, "", log.LstdFlags),
		logger.Config{SlowThreshold: 2 * time.Second, LogLevel: logger.Warn, IgnoreRecordNotFoundError: true, Colorful: false},
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormLogger})
	if err != nil {
		return nil, err
	}
	if err := ensureSchema(db); err != nil {
		return nil, err
	}
	return New(db), nil
}

func ensureSchema(database *gorm.DB) error {
	if err := database.AutoMigrate(&model.Device{}, &DeviceState{}, &CommandLifecycle{}, &PairingLifecycle{}, &LifecycleOutbox{}); err != nil {
		return err
	}
	if err := EnsureMetadataRevision(database); err != nil {
		return err
	}
	if err := database.Model(&model.Device{}).Where("capabilities IS NULL").Update("capabilities", datatypes.JSON([]byte("[]"))).Error; err != nil {
		return err
	}
	if err := database.Model(&model.Device{}).Where("inputs IS NULL").Update("inputs", datatypes.JSON([]byte("[]"))).Error; err != nil {
		return err
	}
	if err := database.Model(&DeviceState{}).Where("state IS NULL").Update("state", json.RawMessage(`{}`)).Error; err != nil {
		return err
	}
	return nil
}

func EnsureMetadataRevision(database *gorm.DB) error {
	if err := database.Exec("CREATE TABLE IF NOT EXISTS hdp_metadata_revision (id integer PRIMARY KEY, revision bigint NOT NULL)").Error; err != nil {
		return err
	}
	if err := database.Exec("INSERT INTO hdp_metadata_revision (id, revision) VALUES (1, 0) ON CONFLICT (id) DO NOTHING").Error; err != nil {
		return err
	}
	switch database.Dialector.Name() {
	case "sqlite":
		for _, operation := range []string{"INSERT", "DELETE"} {
			statement := "CREATE TRIGGER IF NOT EXISTS hdp_metadata_revision_" + strings.ToLower(operation) + " AFTER " + operation + " ON hdp_devices BEGIN UPDATE hdp_metadata_revision SET revision = revision + 1 WHERE id = 1; END"
			if err := database.Exec(statement).Error; err != nil {
				return err
			}
		}
		if err := database.Exec(`CREATE TRIGGER IF NOT EXISTS hdp_metadata_revision_update AFTER UPDATE ON hdp_devices WHEN OLD.protocol IS NOT NEW.protocol OR OLD.external_id IS NOT NEW.external_id OR OLD.type IS NOT NEW.type OR OLD.capabilities IS NOT NEW.capabilities BEGIN UPDATE hdp_metadata_revision SET revision = revision + 1 WHERE id = 1; END`).Error; err != nil {
			return err
		}
	case "postgres":
		if err := database.Exec(`CREATE OR REPLACE FUNCTION hdp_bump_metadata_revision() RETURNS trigger AS $$ BEGIN UPDATE hdp_metadata_revision SET revision = revision + 1 WHERE id = 1; RETURN NULL; END; $$ LANGUAGE plpgsql`).Error; err != nil {
			return err
		}
		if err := database.Exec(`DO $$ BEGIN CREATE TRIGGER hdp_metadata_revision_change AFTER INSERT OR DELETE ON hdp_devices FOR EACH STATEMENT EXECUTE FUNCTION hdp_bump_metadata_revision(); EXCEPTION WHEN duplicate_object THEN NULL; END $$`).Error; err != nil {
			return err
		}
		if err := database.Exec(`DO $$ BEGIN CREATE TRIGGER hdp_metadata_revision_update AFTER UPDATE ON hdp_devices FOR EACH ROW WHEN (OLD.protocol IS DISTINCT FROM NEW.protocol OR OLD.external_id IS DISTINCT FROM NEW.external_id OR OLD.type IS DISTINCT FROM NEW.type OR OLD.capabilities IS DISTINCT FROM NEW.capabilities) EXECUTE FUNCTION hdp_bump_metadata_revision(); EXCEPTION WHEN duplicate_object THEN NULL; END $$`).Error; err != nil {
			return err
		}
	default:
		return errors.New("unsupported metadata revision database")
	}
	return nil
}

func (r *Repository) MetadataRevision(ctx context.Context) (int64, error) {
	var revision int64
	err := r.db.WithContext(ctx).Raw("SELECT revision FROM hdp_metadata_revision WHERE id = 1").Row().Scan(&revision)
	return revision, err
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
	if err := r.db.WithContext(ctx).Order("created_at desc").Find(&devices).Error; err != nil {
		return nil, err
	}
	return devices, nil
}

func (r *Repository) ListMetadataPage(ctx context.Context, after uuid.UUID, limit int) ([]model.Device, error) {
	if limit < 1 || limit > 101 {
		return nil, errors.New("invalid metadata page limit")
	}
	query := r.db.WithContext(ctx).Select("id", "protocol", "external_id", "type", "capabilities", "updated_at").Order("id ASC").Limit(limit)
	if after != uuid.Nil {
		query = query.Where("id > ?", after)
	}
	var devices []model.Device
	if err := query.Find(&devices).Error; err != nil {
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

func (r *Repository) CreateCommandLifecycle(ctx context.Context, lifecycle CommandLifecycle) (bool, error) {
	if lifecycle.Version == 0 {
		lifecycle.Version = 1
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&lifecycle)
	return result.RowsAffected == 1, result.Error
}

func (r *Repository) CompleteCommandLifecycle(ctx context.Context, correlationID, status, errorMessage string) (CommandLifecycle, bool, error) {
	var lifecycle CommandLifecycle
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lifecycle, "correlation_id = ?", correlationID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if lifecycle.TerminalAt != nil {
			return nil
		}
		now := time.Now().UTC()
		result := tx.Model(&CommandLifecycle{}).Where("correlation_id = ? AND version = ? AND terminal_at IS NULL", correlationID, lifecycle.Version).
			Updates(map[string]any{"status": status, "error": errorMessage, "terminal_at": now, "version": lifecycle.Version + 1, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		lifecycle.Status = status
		lifecycle.Error = errorMessage
		lifecycle.TerminalAt = &now
		lifecycle.Version++
		return nil
	})
	if err != nil || lifecycle.TerminalAt == nil {
		return CommandLifecycle{}, false, err
	}
	return lifecycle, true, nil
}

func (r *Repository) CreatePairingLifecycle(ctx context.Context, lifecycle PairingLifecycle) (bool, error) {
	if lifecycle.Version == 0 {
		lifecycle.Version = 1
	}
	var created bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPairingProtocol(tx, lifecycle.Protocol); err != nil {
			return err
		}
		var active PairingLifecycle
		err := tx.Where("protocol = ? AND terminal_at IS NULL", lifecycle.Protocol).First(&active).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(&lifecycle).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}

func (r *Repository) GetActivePairing(ctx context.Context, protocol string) (PairingLifecycle, bool, error) {
	var lifecycle PairingLifecycle
	err := r.db.WithContext(ctx).Where("protocol = ? AND terminal_at IS NULL", protocol).First(&lifecycle).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return PairingLifecycle{}, false, nil
	}
	return lifecycle, err == nil, err
}

func (r *Repository) ListActivePairings(ctx context.Context) ([]PairingLifecycle, error) {
	var lifecycles []PairingLifecycle
	if err := r.db.WithContext(ctx).Where("terminal_at IS NULL").Order("started_at asc").Find(&lifecycles).Error; err != nil {
		return nil, err
	}
	return lifecycles, nil
}

// MutateActivePairing serializes state transitions for one protocol. The
// callback changes a durable session snapshot while the advisory lock and row
// lock are held; returning false leaves the row unchanged.
func (r *Repository) MutateActivePairing(ctx context.Context, protocol string, mutate func(*PairingLifecycle) (bool, error)) (PairingLifecycle, bool, error) {
	var lifecycle PairingLifecycle
	var changed bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPairingProtocol(tx, protocol); err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("protocol = ? AND terminal_at IS NULL", protocol).First(&lifecycle).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		updated, err := mutate(&lifecycle)
		if err != nil || !updated {
			return err
		}
		now := time.Now().UTC()
		updates := map[string]any{"status": lifecycle.Status, "active": lifecycle.Active, "session": lifecycle.Session, "expires_at": lifecycle.ExpiresAt, "version": lifecycle.Version + 1, "updated_at": now}
		if !lifecycle.Active {
			updates["terminal_at"] = now
			lifecycle.TerminalAt = &now
		}
		result := tx.Model(&PairingLifecycle{}).Where("id = ? AND version = ? AND terminal_at IS NULL", lifecycle.ID, lifecycle.Version).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		lifecycle.Version++
		changed = true
		return nil
	})
	if err != nil || !changed {
		return PairingLifecycle{}, false, err
	}
	return lifecycle, true, nil
}

func lockPairingProtocol(tx *gorm.DB, protocol string) error {
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "pairing:"+strings.TrimSpace(protocol)).Error
}

func (r *Repository) CompletePairingLifecycle(ctx context.Context, id, status string, session json.RawMessage) (PairingLifecycle, bool, error) {
	var lifecycle PairingLifecycle
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lifecycle, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if lifecycle.TerminalAt != nil {
			return nil
		}
		now := time.Now().UTC()
		result := tx.Model(&PairingLifecycle{}).Where("id = ? AND version = ? AND terminal_at IS NULL", id, lifecycle.Version).
			Updates(map[string]any{"status": status, "session": session, "active": false, "terminal_at": now, "version": lifecycle.Version + 1, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		lifecycle.Status = status
		lifecycle.Session = session
		lifecycle.Active = false
		lifecycle.TerminalAt = &now
		lifecycle.Version++
		return nil
	})
	if err != nil || lifecycle.TerminalAt == nil {
		return PairingLifecycle{}, false, err
	}
	return lifecycle, true, nil
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
