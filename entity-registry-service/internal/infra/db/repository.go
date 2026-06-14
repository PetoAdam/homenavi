package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Config holds database connectivity settings for the current SQL backend.
type Config = dbx.PostgresConfig

// Repository is the database-backed entity registry repository implementation.
type Repository struct {
	db *gorm.DB
}

func Open(cfg Config) (*gorm.DB, error) {
	dsn := dbx.BuildPostgresDSN(cfg)
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

func New(database *gorm.DB) (*Repository, error) {
	if err := ensureSchema(database); err != nil {
		return nil, err
	}
	return &Repository{db: database}, nil
}

func ensureSchema(database *gorm.DB) error {
	m := database.Migrator()
	if !m.HasTable(&Room{}) {
		if err := m.CreateTable(&Room{}); err != nil {
			return fmt.Errorf("create ers_rooms: %w", err)
		}
	}
	if !m.HasColumn(&Room{}, "Meta") {
		if err := m.AddColumn(&Room{}, "Meta"); err != nil {
			return fmt.Errorf("add ers_rooms.meta: %w", err)
		}
	}
	if !m.HasTable(&Tag{}) {
		if err := m.CreateTable(&Tag{}); err != nil {
			return fmt.Errorf("create ers_tags: %w", err)
		}
	}
	if !m.HasTable(&Group{}) {
		if err := m.CreateTable(&Group{}); err != nil {
			return fmt.Errorf("create ers_groups: %w", err)
		}
	}
	if !m.HasColumn(&Group{}, "Meta") {
		if err := m.AddColumn(&Group{}, "Meta"); err != nil {
			return fmt.Errorf("add ers_groups.meta: %w", err)
		}
	}
	if !m.HasTable(&Device{}) {
		if err := m.CreateTable(&Device{}); err != nil {
			return fmt.Errorf("create ers_devices: %w", err)
		}
	}
	if !m.HasColumn(&Device{}, "Meta") {
		if err := m.AddColumn(&Device{}, "Meta"); err != nil {
			return fmt.Errorf("add ers_devices.meta: %w", err)
		}
	}
	if !m.HasTable(&DeviceTag{}) {
		if err := m.CreateTable(&DeviceTag{}); err != nil {
			return fmt.Errorf("create ers_device_tags: %w", err)
		}
	}
	if !m.HasTable(&GroupMember{}) {
		if err := m.CreateTable(&GroupMember{}); err != nil {
			return fmt.Errorf("create ers_group_members: %w", err)
		}
	}
	if !m.HasTable(&DeviceBinding{}) {
		if err := m.CreateTable(&DeviceBinding{}); err != nil {
			return fmt.Errorf("create ers_device_bindings: %w", err)
		}
	}
	if !m.HasColumn(&DeviceBinding{}, "HDPDeviceID") {
		if err := m.AddColumn(&DeviceBinding{}, "HDPDeviceID"); err != nil {
			return fmt.Errorf("add ers_device_bindings.hdp_device_id: %w", err)
		}
	}
	if m.HasTable(&hdpDeviceRecord{}) && !m.HasConstraint(&DeviceBinding{}, "HDPDevice") {
		_ = m.CreateConstraint(&DeviceBinding{}, "HDPDevice")
	}
	if !m.HasConstraint(&Device{}, "Room") {
		_ = m.CreateConstraint(&Device{}, "Room")
	}
	if !m.HasConstraint(&DeviceTag{}, "Device") {
		_ = m.CreateConstraint(&DeviceTag{}, "Device")
	}
	if !m.HasConstraint(&DeviceTag{}, "Tag") {
		_ = m.CreateConstraint(&DeviceTag{}, "Tag")
	}
	if !m.HasConstraint(&GroupMember{}, "Group") {
		_ = m.CreateConstraint(&GroupMember{}, "Group")
	}
	if !m.HasConstraint(&GroupMember{}, "Device") {
		_ = m.CreateConstraint(&GroupMember{}, "Device")
	}
	if !m.HasConstraint(&DeviceBinding{}, "Device") {
		_ = m.CreateConstraint(&DeviceBinding{}, "Device")
	}
	if err := backfillERSMetadata(database.WithContext(context.Background())); err != nil {
		return err
	}
	if err := backfillERSBindingDeviceIDs(database.WithContext(context.Background())); err != nil {
		return err
	}
	return nil
}

func backfillERSMetadata(database *gorm.DB) error {
	emptyJSON := datatypes.JSON([]byte("{}"))
	if err := database.Model(&Room{}).Where("meta IS NULL").Update("meta", emptyJSON).Error; err != nil {
		return fmt.Errorf("backfill ers_rooms.meta: %w", err)
	}
	if err := database.Model(&Group{}).Where("meta IS NULL").Update("meta", emptyJSON).Error; err != nil {
		return fmt.Errorf("backfill ers_groups.meta: %w", err)
	}
	if err := database.Model(&Device{}).Where("meta IS NULL").Update("meta", emptyJSON).Error; err != nil {
		return fmt.Errorf("backfill ers_devices.meta: %w", err)
	}
	return nil
}

func backfillERSBindingDeviceIDs(database *gorm.DB) error {
	var bindings []DeviceBinding
	if err := database.Where("kind = ? AND hdp_device_id IS NULL", "hdp").Find(&bindings).Error; err != nil {
		return fmt.Errorf("load ers hdp bindings: %w", err)
	}
	for _, binding := range bindings {
		hdpDeviceID, err := resolveHDPDeviceID(database, binding.ExternalID)
		if err != nil {
			return fmt.Errorf("resolve ers hdp binding %s: %w", binding.ID, err)
		}
		if hdpDeviceID == nil {
			continue
		}
		if err := database.Model(&DeviceBinding{}).Where("id = ?", binding.ID).Update("hdp_device_id", *hdpDeviceID).Error; err != nil {
			return fmt.Errorf("update ers hdp binding %s: %w", binding.ID, err)
		}
	}
	return nil
}

func resolveHDPDeviceID(database *gorm.DB, externalRef string) (*uuid.UUID, error) {
	protocol, externalID, ok := splitHDPExternalRef(externalRef)
	if !ok {
		return nil, nil
	}
	device, err := loadHDPDeviceByExternal(database, protocol, externalID)
	if err != nil {
		return nil, err
	}
	if device == nil {
		return nil, nil
	}
	id := device.ID
	return &id, nil
}

func loadHDPDeviceByExternal(database *gorm.DB, protocol, externalID string) (*hdpDeviceRecord, error) {
	if !database.Migrator().HasTable(&hdpDeviceRecord{}) {
		return nil, nil
	}
	var device hdpDeviceRecord
	query := database.Where("protocol = ? AND external_id = ?", protocol, externalID)
	if err := query.First(&device).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &device, nil
}

func loadHDPDeviceByID(database *gorm.DB, id uuid.UUID) (*hdpDeviceRecord, error) {
	if !database.Migrator().HasTable(&hdpDeviceRecord{}) {
		return nil, nil
	}
	var device hdpDeviceRecord
	if err := database.First(&device, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &device, nil
}

func splitHDPExternalRef(externalRef string) (string, string, bool) {
	trimmed := strings.TrimSpace(externalRef)
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	protocol := strings.TrimSpace(parts[0])
	externalID := strings.TrimSpace(parts[1])
	if protocol == "" || externalID == "" {
		return "", "", false
	}
	return protocol, externalID, true
}
