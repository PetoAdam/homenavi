package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Config holds database connectivity settings for the current SQL backend.
type Config = dbx.PostgresConfig

// Repository is the database-backed history repository implementation.
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
	if err := database.AutoMigrate(&DeviceStatePoint{}); err != nil {
		return fmt.Errorf("auto migrate history schema: %w", err)
	}
	return backfillHistoryHDPDeviceIDs(database.WithContext(context.Background()))
}

func (r *Repository) InsertStatePoint(ctx context.Context, p *DeviceStatePoint) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if p.IngestedAt.IsZero() {
		p.IngestedAt = p.TS.UTC()
	}
	if p.HDPDeviceID == nil {
		hdpDeviceID, err := resolveHDPDeviceID(r.db.WithContext(ctx), p.DeviceID)
		if err != nil {
			return err
		}
		p.HDPDeviceID = hdpDeviceID
	}
	return r.db.WithContext(ctx).Create(p).Error
}

type Page struct {
	Points     []DeviceStatePoint `json:"points"`
	NextCursor string             `json:"next_cursor,omitempty"`
}

func (r *Repository) ListStatePoints(ctx context.Context, deviceID string, from, to time.Time, limit int, cursor *Cursor, desc bool) (Page, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 10000 {
		limit = 10000
	}
	exprs := []clause.Expression{clause.Eq{Column: clause.Column{Name: "device_id"}, Value: deviceID}}
	if !from.IsZero() {
		exprs = append(exprs, clause.Gte{Column: clause.Column{Name: "ts"}, Value: from})
	}
	if !to.IsZero() {
		exprs = append(exprs, clause.Lte{Column: clause.Column{Name: "ts"}, Value: to})
	}
	if cursor != nil {
		if desc {
			exprs = append(exprs, clause.Or(
				clause.Lt{Column: clause.Column{Name: "ts"}, Value: cursor.TS},
				clause.And(
					clause.Eq{Column: clause.Column{Name: "ts"}, Value: cursor.TS},
					clause.Lt{Column: clause.Column{Name: "id"}, Value: cursor.ID},
				),
			))
		} else {
			exprs = append(exprs, clause.Or(
				clause.Gt{Column: clause.Column{Name: "ts"}, Value: cursor.TS},
				clause.And(
					clause.Eq{Column: clause.Column{Name: "ts"}, Value: cursor.TS},
					clause.Gt{Column: clause.Column{Name: "id"}, Value: cursor.ID},
				),
			))
		}
	}
	order := clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "ts"}, Desc: desc}, {Column: clause.Column{Name: "id"}, Desc: desc}}}
	var rows []DeviceStatePoint
	q := r.db.WithContext(ctx).Clauses(clause.Where{Exprs: exprs}, order).Limit(limit + 1)
	if err := q.Find(&rows).Error; err != nil {
		return Page{}, err
	}
	var next *Cursor
	if len(rows) > limit {
		last := rows[limit-1]
		next = &Cursor{TS: last.TS, ID: last.ID}
		rows = rows[:limit]
	}
	out := Page{Points: rows}
	if next != nil {
		out.NextCursor = EncodeCursor(*next)
	}
	return out, nil
}

func backfillHistoryHDPDeviceIDs(database *gorm.DB) error {
	var points []DeviceStatePoint
	if err := database.Where("hdp_device_id IS NULL AND device_id <> ''").Find(&points).Error; err != nil {
		return fmt.Errorf("load history rows for hdp_device_id backfill: %w", err)
	}
	for _, point := range points {
		hdpDeviceID, err := resolveHDPDeviceID(database, point.DeviceID)
		if err != nil {
			return fmt.Errorf("resolve history row %s: %w", point.ID, err)
		}
		if hdpDeviceID == nil {
			continue
		}
		if err := database.Model(&DeviceStatePoint{}).Where("id = ?", point.ID).Update("hdp_device_id", *hdpDeviceID).Error; err != nil {
			return fmt.Errorf("update history row %s: %w", point.ID, err)
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
	if err := database.Where("protocol = ? AND external_id = ?", protocol, externalID).First(&device).Error; err != nil {
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
