package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrDiscoveryUnavailable = errors.New("discovery projection is not ready or is stale")
var ErrDiscoveryCursor = errors.New("invalid or expired discovery cursor")
var ErrDiscoveryRoomNotFound = errors.New("room not found")
var ErrDiscoveryRoomAmbiguous = errors.New("room name is ambiguous")
var ErrDiscoveryInvalid = errors.New("invalid discovery filter")
var ErrDiscoveryTooLarge = errors.New("discovery result exceeds size limit")
var ErrDiscoveryBindingConflict = errors.New("conflicting device bindings")

type DiscoveryRoomAmbiguity struct {
	Candidates []DiscoveryRoom
}

func (err *DiscoveryRoomAmbiguity) Error() string { return ErrDiscoveryRoomAmbiguous.Error() }
func (err *DiscoveryRoomAmbiguity) Unwrap() error { return ErrDiscoveryRoomAmbiguous }

type DiscoveryFilter struct {
	RoomID     string `json:"room_id,omitempty"`
	Room       string `json:"room,omitempty"`
	Name       string `json:"name,omitempty"`
	DeviceType string `json:"device_type,omitempty"`
	Capability string `json:"capability,omitempty"`
	Limit      int    `json:"limit"`
	Cursor     string `json:"cursor,omitempty"`
}

type DiscoveryResult struct {
	DeviceID     string  `json:"device_id"`
	Name         string  `json:"name"`
	RoomID       *string `json:"room_id,omitempty"`
	Room         *string `json:"room,omitempty"`
	Type         string  `json:"type"`
	SortName     string  `json:"-" gorm:"column:sort_name"`
	BindingCount int     `json:"-" gorm:"column:binding_count"`
}

type DiscoveryPage struct {
	Devices    []DiscoveryResult `json:"devices"`
	NextCursor string            `json:"next_cursor,omitempty"`
	Room       *DiscoveryRoom    `json:"room,omitempty"`
}

type DiscoveryRoom struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type discoveryCursor struct {
	Generation int64  `json:"g"`
	Filter     string `json:"f"`
	Name       string `json:"n"`
	ID         string `json:"i"`
}

func (r *Repository) SearchDiscovery(ctx context.Context, filter DiscoveryFilter) (DiscoveryPage, error) {
	var page DiscoveryPage
	filter.RoomID = strings.TrimSpace(filter.RoomID)
	filter.Room = strings.TrimSpace(filter.Room)
	filter.Name = strings.TrimSpace(filter.Name)
	filter.DeviceType = strings.TrimSpace(filter.DeviceType)
	filter.Capability = strings.TrimSpace(filter.Capability)
	for _, value := range []string{filter.RoomID, filter.Room, filter.Name, filter.DeviceType, filter.Capability} {
		if len(value) > 128 {
			return page, ErrDiscoveryInvalid
		}
	}
	if filter.Room != "" && filter.RoomID != "" || filter.Limit < 1 || filter.Limit > 100 || len(filter.Cursor) > 2048 {
		return page, ErrDiscoveryInvalid
	}
	cursorValue := filter.Cursor
	requestedLimit := filter.Limit
	filter.Cursor = ""
	filter.Limit = 0
	encodedFilter, err := json.Marshal(filter)
	filter.Cursor = cursorValue
	filter.Limit = requestedLimit
	if err != nil {
		return page, err
	}
	digest := sha256.Sum256(encodedFilter)
	fingerprint := hex.EncodeToString(digest[:])
	var cursor discoveryCursor
	if filter.Cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(filter.Cursor)
		if err != nil || json.Unmarshal(decoded, &cursor) != nil || cursor.Filter != fingerprint || cursor.ID == "" {
			return page, ErrDiscoveryCursor
		}
	}
	opts := &sql.TxOptions{}
	if r.db.Dialector.Name() == "postgres" {
		opts.Isolation = sql.LevelRepeatableRead
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var status DiscoveryStatus
		if err := tx.First(&status, 1).Error; err != nil {
			return err
		}
		if status.SourceRevision < 0 || status.LastReconciledAt.IsZero() || time.Since(status.LastReconciledAt) > 60*time.Second {
			return ErrDiscoveryUnavailable
		}
		if filter.Cursor != "" && cursor.Generation != status.Generation {
			return ErrDiscoveryCursor
		}
		if filter.RoomID != "" || filter.Room != "" {
			query := tx.Model(&Room{})
			if filter.RoomID != "" {
				if _, err := uuid.Parse(filter.RoomID); err != nil {
					return ErrDiscoveryInvalid
				}
				query = query.Where("id = ?", filter.RoomID)
			} else {
				query = query.Where("LOWER(name) = ?", strings.ToLower(filter.Room))
			}
			var rooms []Room
			if err := query.Order("id ASC").Limit(10).Find(&rooms).Error; err != nil {
				return err
			}
			if len(rooms) == 0 {
				return ErrDiscoveryRoomNotFound
			}
			if len(rooms) > 1 {
				candidates := make([]DiscoveryRoom, 0, len(rooms))
				for _, room := range rooms {
					candidates = append(candidates, DiscoveryRoom{ID: room.ID, Name: room.Name})
				}
				return &DiscoveryRoomAmbiguity{Candidates: candidates}
			}
			page.Room = &DiscoveryRoom{ID: rooms[0].ID, Name: rooms[0].Name}
		}
		bindingMatch := "binding.kind = 'hdp' AND (binding.hdp_device_id = projection.source_id OR (binding.hdp_device_id IS NULL AND binding.external_id = projection.device_id))"
		conflictMatch := "conflict.kind = 'hdp' AND (conflict.hdp_device_id = projection.source_id OR (conflict.hdp_device_id IS NULL AND conflict.external_id = projection.device_id))"
		query := tx.Table("ers_discovery_devices AS projection").
			Select("projection.device_id, COALESCE(device.name, projection.device_id) AS name, LOWER(COALESCE(device.name, projection.device_id)) AS sort_name, room.id AS room_id, room.name AS room, projection.type, (SELECT COUNT(*) FROM ers_device_bindings AS conflict WHERE " + conflictMatch + ") AS binding_count").
			Joins("LEFT JOIN ers_device_bindings AS binding ON " + bindingMatch).
			Joins("LEFT JOIN ers_devices AS device ON device.id = binding.device_id").
			Joins("LEFT JOIN ers_rooms AS room ON room.id = device.room_id")
		if page.Room != nil {
			query = query.Where("device.room_id = ?", page.Room.ID)
		}
		name := "LOWER(COALESCE(device.name, projection.device_id))"
		if filter.Name != "" {
			escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(filter.Name))
			query = query.Where(name+` LIKE ? ESCAPE '\'`, "%"+escaped+"%")
		}
		if filter.DeviceType != "" {
			query = query.Where("projection.type = ?", strings.ToLower(filter.DeviceType))
		}
		if filter.Capability != "" {
			query = query.Where("EXISTS (SELECT 1 FROM ers_discovery_capabilities AS cap WHERE cap.device_id = projection.device_id AND cap.capability = ?)", strings.ToLower(filter.Capability))
		}
		if filter.Cursor != "" {
			query = query.Where("("+name+" > ? OR ("+name+" = ? AND projection.device_id > ?))", cursor.Name, cursor.Name, cursor.ID)
		}
		page.Devices = []DiscoveryResult{}
		if err := query.Order(name + " ASC").Order("projection.device_id ASC").Limit(filter.Limit + 1).Scan(&page.Devices).Error; err != nil {
			return err
		}
		for _, device := range page.Devices {
			if device.BindingCount > 1 {
				return ErrDiscoveryBindingConflict
			}
		}
		hasMore := len(page.Devices) > filter.Limit
		if hasMore {
			page.Devices = page.Devices[:filter.Limit]
		}
		for {
			encoded, err := json.Marshal(page)
			if err != nil {
				return err
			}
			if len(encoded) <= 128<<10 {
				break
			}
			if len(page.Devices) <= 1 {
				return ErrDiscoveryTooLarge
			}
			page.Devices = page.Devices[:len(page.Devices)-1]
			hasMore = true
		}
		if hasMore {
			last := page.Devices[len(page.Devices)-1]
			if len(last.Name) > 1024 {
				return ErrDiscoveryTooLarge
			}
			data, err := json.Marshal(discoveryCursor{Generation: status.Generation, Filter: fingerprint, Name: last.SortName, ID: last.DeviceID})
			if err != nil {
				return err
			}
			page.NextCursor = base64.RawURLEncoding.EncodeToString(data)
		}
		return nil
	}, opts)
	return page, err
}

type DiscoveryDevice struct {
	DeviceID string     `gorm:"primaryKey;type:text"`
	SourceID *uuid.UUID `gorm:"type:uuid;index"`
	Type     string     `gorm:"index;not null"`
}

func (DiscoveryDevice) TableName() string { return "ers_discovery_devices" }

type DiscoveryCapability struct {
	DeviceID   string `gorm:"primaryKey;type:text;index:idx_discovery_capability_device,priority:2"`
	Capability string `gorm:"primaryKey;type:text;index:idx_discovery_capability_device,priority:1"`
}

func (DiscoveryCapability) TableName() string { return "ers_discovery_capabilities" }

type DiscoveryStatus struct {
	ID               int `gorm:"primaryKey"`
	SourceRevision   int64
	Generation       int64
	LastReconciledAt time.Time
}

func (DiscoveryStatus) TableName() string { return "ers_discovery_status" }

type DiscoveryMetadata struct {
	DeviceID     string          `json:"device_id"`
	SourceID     *uuid.UUID      `json:"source_id"`
	DeviceType   string          `json:"device_type"`
	Capabilities json.RawMessage `json:"capabilities"`
}

func (r *Repository) ApplyDiscoverySnapshot(ctx context.Context, revision int64, items []DiscoveryMetadata) error {
	if revision < 0 {
		return errors.New("invalid source revision")
	}
	devices := make([]DiscoveryDevice, 0, len(items))
	capabilities := make([]DiscoveryCapability, 0)
	seenDevices := make(map[string]bool, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.DeviceID)
		if id == "" || id != item.DeviceID || len(id) > 512 || seenDevices[id] || len(item.DeviceType) > 128 {
			return errors.New("invalid or duplicate device metadata")
		}
		seenDevices[id] = true
		devices = append(devices, DiscoveryDevice{DeviceID: id, SourceID: item.SourceID, Type: strings.ToLower(strings.TrimSpace(item.DeviceType))})
		var entries []struct {
			ID       string `json:"id"`
			Property string `json:"property"`
		}
		if err := json.Unmarshal(item.Capabilities, &entries); err != nil || len(entries) > 1000 {
			return errors.New("invalid capabilities")
		}
		seen := map[string]bool{}
		for _, entry := range entries {
			for _, raw := range []string{entry.ID, entry.Property} {
				value := strings.ToLower(strings.TrimSpace(raw))
				if len(value) > 128 {
					return errors.New("capability identifier too long")
				}
				if value != "" && !seen[value] {
					seen[value] = true
					capabilities = append(capabilities, DiscoveryCapability{DeviceID: id, Capability: value})
				}
			}
		}
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		initial := DiscoveryStatus{ID: 1, SourceRevision: -1}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
			return err
		}
		var status DiscoveryStatus
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&status, 1).Error; err != nil {
			return err
		}
		if revision < status.SourceRevision {
			return errors.New("older discovery snapshot")
		}
		if revision > status.SourceRevision {
			if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&DiscoveryCapability{}).Error; err != nil {
				return err
			}
			if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&DiscoveryDevice{}).Error; err != nil {
				return err
			}
			if len(devices) > 0 {
				if err := tx.CreateInBatches(devices, 100).Error; err != nil {
					return err
				}
			}
			if len(capabilities) > 0 {
				if err := tx.CreateInBatches(capabilities, 100).Error; err != nil {
					return err
				}
			}
		}
		updates := map[string]any{"source_revision": revision, "last_reconciled_at": time.Now().UTC()}
		if revision > status.SourceRevision {
			updates["generation"] = gorm.Expr("generation + 1")
		}
		return tx.Model(&status).Updates(updates).Error
	})
}

func ensureDiscoveryGeneration(database *gorm.DB) error {
	initial := DiscoveryStatus{ID: 1, SourceRevision: -1}
	if err := database.Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
		return err
	}
	for _, table := range []string{"ers_rooms", "ers_devices", "ers_device_bindings"} {
		switch database.Dialector.Name() {
		case "sqlite":
			for _, operation := range []string{"INSERT", "UPDATE", "DELETE"} {
				statement := "CREATE TRIGGER IF NOT EXISTS " + table + "_discovery_" + strings.ToLower(operation) + " AFTER " + operation + " ON " + table + " BEGIN UPDATE ers_discovery_status SET generation = generation + 1 WHERE id = 1; END"
				if err := database.Exec(statement).Error; err != nil {
					return err
				}
			}
		case "postgres":
			if err := database.Exec(`CREATE OR REPLACE FUNCTION ers_bump_discovery_generation() RETURNS trigger AS $$ BEGIN UPDATE ers_discovery_status SET generation = generation + 1 WHERE id = 1; RETURN NULL; END; $$ LANGUAGE plpgsql`).Error; err != nil {
				return err
			}
			statement := "DO $$ BEGIN CREATE TRIGGER " + table + "_discovery_change AFTER INSERT OR UPDATE OR DELETE ON " + table + " FOR EACH STATEMENT EXECUTE FUNCTION ers_bump_discovery_generation(); EXCEPTION WHEN duplicate_object THEN NULL; END $$"
			if err := database.Exec(statement).Error; err != nil {
				return err
			}
		default:
			return errors.New("unsupported discovery generation database")
		}
	}
	return nil
}
