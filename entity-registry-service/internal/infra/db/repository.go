package db

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// Repository is the database-backed entity registry repository implementation.
type Repository struct {
	db *gorm.DB
}

func Open(cfg Config) (*gorm.DB, error) {
	dsn := dbx.BuildPostgresDSN(dbx.PostgresConfig{User: cfg.User, Password: cfg.Password, DBName: cfg.DBName, Host: cfg.Host, Port: cfg.Port, SSLMode: cfg.SSLMode})
	return gorm.Open(postgres.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
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
			return fmt.Errorf("create table rooms: %w", err)
		}
	}
	if !m.HasColumn(&Room{}, "Meta") {
		if err := m.AddColumn(&Room{}, "Meta"); err != nil {
			return fmt.Errorf("add column rooms.meta: %w", err)
		}
	}
	if !m.HasTable(&Tag{}) {
		if err := m.CreateTable(&Tag{}); err != nil {
			return fmt.Errorf("create table tags: %w", err)
		}
	}
	if !m.HasTable(&Device{}) {
		if err := m.CreateTable(&Device{}); err != nil {
			return fmt.Errorf("create table devices: %w", err)
		}
	}
	if !m.HasTable(&DeviceTag{}) {
		if err := m.CreateTable(&DeviceTag{}); err != nil {
			return fmt.Errorf("create table device_tags: %w", err)
		}
	}
	if !m.HasTable(&DeviceBinding{}) {
		if err := m.CreateTable(&DeviceBinding{}); err != nil {
			return fmt.Errorf("create table device_bindings: %w", err)
		}
	}
	if !m.HasIndex(&Room{}, "Slug") {
		_ = m.CreateIndex(&Room{}, "Slug")
	}
	if !m.HasIndex(&Tag{}, "Slug") {
		_ = m.CreateIndex(&Tag{}, "Slug")
	}
	if !m.HasIndex(&DeviceTag{}, "DeviceID") {
		_ = m.CreateIndex(&DeviceTag{}, "DeviceID")
	}
	if !m.HasIndex(&DeviceTag{}, "TagID") {
		_ = m.CreateIndex(&DeviceTag{}, "TagID")
	}
	if !m.HasIndex(&DeviceBinding{}, "DeviceID") {
		_ = m.CreateIndex(&DeviceBinding{}, "DeviceID")
	}
	if !m.HasIndex(&DeviceBinding{}, "Kind") {
		_ = m.CreateIndex(&DeviceBinding{}, "Kind")
	}
	if !m.HasIndex(&DeviceBinding{}, "ExternalID") {
		_ = m.CreateIndex(&DeviceBinding{}, "ExternalID")
	}
	if !m.HasIndex(&DeviceBinding{}, "idx_kind_external") {
		_ = m.CreateIndex(&DeviceBinding{}, "idx_kind_external")
	}
	return nil
}

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

type DeviceView struct {
	Device
	Tags           []Tag    `json:"tags"`
	HDPExternalIDs []string `json:"hdp_external_ids"`
}

func (r *Repository) FindDeviceIDByHDPExternalID(ctx context.Context, externalID string) (uuid.UUID, bool, error) {
	x := strings.TrimSpace(externalID)
	if x == "" {
		return uuid.Nil, false, nil
	}
	var binding DeviceBinding
	err := r.db.WithContext(ctx).Select("device_id").Where("kind = ? AND external_id = ?", "hdp", x).First(&binding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, err
	}
	if binding.DeviceID == uuid.Nil {
		return uuid.Nil, false, nil
	}
	return binding.DeviceID, true, nil
}

func (r *Repository) EnsureDeviceForHDP(ctx context.Context, externalID, name, description string) (uuid.UUID, bool, error) {
	x := strings.TrimSpace(externalID)
	if x == "" {
		return uuid.Nil, false, errors.New("external_id is required")
	}
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		trimmedName = x
	}
	trimmedDesc := strings.TrimSpace(description)

	var outID uuid.UUID
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var binding DeviceBinding
		err := tx.Select("device_id").Where("kind = ? AND external_id = ?", "hdp", x).First(&binding).Error
		if err == nil {
			outID = binding.DeviceID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		dev := &Device{ID: uuid.New(), Name: trimmedName, Description: trimmedDesc}
		if err := tx.Create(dev).Error; err != nil {
			return err
		}
		b := &DeviceBinding{ID: uuid.New(), DeviceID: dev.ID, Kind: "hdp", ExternalID: x}
		if err := tx.Create(b).Error; err != nil {
			return err
		}
		outID = dev.ID
		created = true
		return nil
	})
	if err != nil {
		if id, ok, lookupErr := r.FindDeviceIDByHDPExternalID(ctx, x); lookupErr == nil && ok {
			return id, false, nil
		}
		return uuid.Nil, false, err
	}
	return outID, created, nil
}

func (r *Repository) ListDevices(ctx context.Context) ([]DeviceView, error) {
	var devices []Device
	if err := r.db.WithContext(ctx).Order("created_at desc").Find(&devices).Error; err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return []DeviceView{}, nil
	}

	deviceIDs := make([]uuid.UUID, 0, len(devices))
	for _, d := range devices {
		deviceIDs = append(deviceIDs, d.ID)
	}

	var deviceTags []DeviceTag
	if err := r.db.WithContext(ctx).Where("device_id IN ?", deviceIDs).Find(&deviceTags).Error; err != nil {
		return nil, err
	}

	tagIDs := make([]uuid.UUID, 0, len(deviceTags))
	seenTag := map[uuid.UUID]struct{}{}
	for _, dt := range deviceTags {
		if dt.TagID == uuid.Nil {
			continue
		}
		if _, ok := seenTag[dt.TagID]; ok {
			continue
		}
		seenTag[dt.TagID] = struct{}{}
		tagIDs = append(tagIDs, dt.TagID)
	}

	var tags []Tag
	if len(tagIDs) > 0 {
		if err := r.db.WithContext(ctx).Where("id IN ?", tagIDs).Order("name asc").Find(&tags).Error; err != nil {
			return nil, err
		}
	}
	tagByID := map[uuid.UUID]Tag{}
	for _, t := range tags {
		tagByID[t.ID] = t
	}
	deviceTagsByDeviceID := map[uuid.UUID][]Tag{}
	for _, dt := range deviceTags {
		t, ok := tagByID[dt.TagID]
		if !ok {
			continue
		}
		deviceTagsByDeviceID[dt.DeviceID] = append(deviceTagsByDeviceID[dt.DeviceID], t)
	}
	for did := range deviceTagsByDeviceID {
		rows := deviceTagsByDeviceID[did]
		sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
		deviceTagsByDeviceID[did] = rows
	}

	var bindings []DeviceBinding
	if err := r.db.WithContext(ctx).Where("kind = ? AND device_id IN ?", "hdp", deviceIDs).Find(&bindings).Error; err != nil {
		return nil, err
	}
	deviceBindingsByDeviceID := map[uuid.UUID][]string{}
	for _, b := range bindings {
		x := strings.TrimSpace(b.ExternalID)
		if x == "" {
			continue
		}
		deviceBindingsByDeviceID[b.DeviceID] = append(deviceBindingsByDeviceID[b.DeviceID], x)
	}
	for did := range deviceBindingsByDeviceID {
		rows := deviceBindingsByDeviceID[did]
		sort.Strings(rows)
		uniq := rows[:0]
		var last string
		for _, x := range rows {
			if x == last {
				continue
			}
			last = x
			uniq = append(uniq, x)
		}
		deviceBindingsByDeviceID[did] = uniq
	}

	out := make([]DeviceView, 0, len(devices))
	for _, d := range devices {
		out = append(out, DeviceView{Device: d, Tags: deviceTagsByDeviceID[d.ID], HDPExternalIDs: deviceBindingsByDeviceID[d.ID]})
	}
	return out, nil
}

func (r *Repository) CreateDevice(ctx context.Context, dev *Device) error {
	if dev.ID == uuid.Nil {
		dev.ID = uuid.New()
	}
	dev.Name = strings.TrimSpace(dev.Name)
	if dev.Name == "" {
		return errors.New("device.name is required")
	}
	dev.Description = strings.TrimSpace(dev.Description)
	return r.db.WithContext(ctx).Create(dev).Error
}

func (r *Repository) GetDevice(ctx context.Context, id uuid.UUID) (*Device, error) {
	var row Device
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) GetDeviceView(ctx context.Context, id uuid.UUID) (*DeviceView, error) {
	dev, err := r.GetDevice(ctx, id)
	if err != nil {
		return nil, err
	}
	tagsTable := Tag{}.TableName()
	deviceTagsTable := DeviceTag{}.TableName()
	var tags []Tag
	if err := r.db.WithContext(ctx).
		Table(tagsTable).
		Select(tagsTable+".*").
		Joins("join "+deviceTagsTable+" on "+deviceTagsTable+".tag_id = "+tagsTable+".id").
		Where(deviceTagsTable+".device_id = ?", id).
		Order(tagsTable + ".name asc").
		Find(&tags).Error; err != nil {
		return nil, err
	}
	var bindings []DeviceBinding
	if err := r.db.WithContext(ctx).Where("device_id = ? AND kind = ?", id, "hdp").Find(&bindings).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(bindings))
	for _, b := range bindings {
		if strings.TrimSpace(b.ExternalID) == "" {
			continue
		}
		ids = append(ids, b.ExternalID)
	}
	sort.Strings(ids)
	uniq := ids[:0]
	var last string
	for _, x := range ids {
		if x == last {
			continue
		}
		last = x
		uniq = append(uniq, x)
	}
	ids = uniq
	return &DeviceView{Device: *dev, Tags: tags, HDPExternalIDs: ids}, nil
}

func (r *Repository) UpdateDevice(ctx context.Context, id uuid.UUID, patch map[string]any) (*Device, error) {
	if v, ok := patch["name"].(string); ok {
		patch["name"] = strings.TrimSpace(v)
	}
	if v, ok := patch["description"].(string); ok {
		patch["description"] = strings.TrimSpace(v)
	}
	if err := r.db.WithContext(ctx).Model(&Device{}).Where("id = ?", id).Updates(patch).Error; err != nil {
		return nil, err
	}
	return r.GetDevice(ctx, id)
}

func (r *Repository) DeleteDevice(ctx context.Context, id uuid.UUID) error {
	_ = r.db.WithContext(ctx).Where("device_id = ?", id).Delete(&DeviceTag{}).Error
	_ = r.db.WithContext(ctx).Where("device_id = ?", id).Delete(&DeviceBinding{}).Error
	return r.db.WithContext(ctx).Delete(&Device{}, "id = ?", id).Error
}

func (r *Repository) SetDeviceTags(ctx context.Context, deviceID uuid.UUID, tagIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("device_id = ?", deviceID).Delete(&DeviceTag{}).Error; err != nil {
			return err
		}
		seen := map[uuid.UUID]struct{}{}
		rows := make([]DeviceTag, 0, len(tagIDs))
		for _, tid := range tagIDs {
			if tid == uuid.Nil {
				continue
			}
			if _, ok := seen[tid]; ok {
				continue
			}
			seen[tid] = struct{}{}
			rows = append(rows, DeviceTag{DeviceID: deviceID, TagID: tid})
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

func (r *Repository) SetDeviceHDPBindings(ctx context.Context, deviceID uuid.UUID, externalIDs []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("device_id = ? AND kind = ?", deviceID, "hdp").Delete(&DeviceBinding{}).Error; err != nil {
			return err
		}
		seen := map[string]struct{}{}
		rows := make([]DeviceBinding, 0, len(externalIDs))
		for _, raw := range externalIDs {
			x := strings.TrimSpace(raw)
			if x == "" {
				continue
			}
			if _, ok := seen[x]; ok {
				continue
			}
			seen[x] = struct{}{}
			rows = append(rows, DeviceBinding{ID: uuid.New(), DeviceID: deviceID, Kind: "hdp", ExternalID: x})
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

func (r *Repository) ResolveSelectorToHDP(ctx context.Context, selector string) ([]string, []uuid.UUID, error) {
	sel := strings.TrimSpace(selector)
	if sel == "" {
		return nil, nil, errors.New("selector is required")
	}
	parts := strings.SplitN(sel, ":", 2)
	if len(parts) != 2 {
		return nil, nil, errors.New("invalid selector format")
	}
	kind := strings.ToLower(strings.TrimSpace(parts[0]))
	arg := strings.TrimSpace(parts[1])
	if arg == "" {
		return nil, nil, errors.New("invalid selector")
	}

	var deviceIDs []uuid.UUID
	switch kind {
	case "tag":
		var tag Tag
		if err := r.db.WithContext(ctx).First(&tag, "slug = ?", strings.ToLower(arg)).Error; err != nil {
			return []string{}, []uuid.UUID{}, nil
		}
		var rows []DeviceTag
		if err := r.db.WithContext(ctx).Where("tag_id = ?", tag.ID).Find(&rows).Error; err != nil {
			return nil, nil, err
		}
		for _, row := range rows {
			deviceIDs = append(deviceIDs, row.DeviceID)
		}
	case "room":
		if id, err := uuid.Parse(arg); err == nil {
			var devs []Device
			if err := r.db.WithContext(ctx).Where("room_id = ?", id).Find(&devs).Error; err != nil {
				return nil, nil, err
			}
			for _, d := range devs {
				deviceIDs = append(deviceIDs, d.ID)
			}
		} else {
			var room Room
			if err := r.db.WithContext(ctx).First(&room, "slug = ?", strings.ToLower(arg)).Error; err != nil {
				return []string{}, []uuid.UUID{}, nil
			}
			var devs []Device
			if err := r.db.WithContext(ctx).Where("room_id = ?", room.ID).Find(&devs).Error; err != nil {
				return nil, nil, err
			}
			for _, d := range devs {
				deviceIDs = append(deviceIDs, d.ID)
			}
		}
	default:
		return nil, nil, errors.New("unsupported selector kind")
	}

	if len(deviceIDs) == 0 {
		return []string{}, []uuid.UUID{}, nil
	}

	var bindings []DeviceBinding
	if err := r.db.WithContext(ctx).Where("kind = ? AND device_id IN ?", "hdp", deviceIDs).Find(&bindings).Error; err != nil {
		return nil, nil, err
	}

	out := make([]string, 0, len(bindings))
	seen := map[string]struct{}{}
	for _, b := range bindings {
		id := strings.TrimSpace(b.ExternalID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, deviceIDs, nil
}

func (r *Repository) DB() *gorm.DB {
	return r.db
}

func (r *Repository) ListDevicesPage(ctx context.Context, limit int, cursor *uuid.UUID) ([]Device, *uuid.UUID, error) {
	query := r.db.WithContext(ctx).Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}, Desc: false})
	if cursor != nil && *cursor != uuid.Nil {
		query = query.Where("id > ?", *cursor)
	}
	if limit <= 0 {
		limit = 100
	}
	rows := make([]Device, 0, limit+1)
	if err := query.Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	if len(rows) <= limit {
		return rows, nil, nil
	}
	next := rows[limit].ID
	return rows[:limit], &next, nil
}
