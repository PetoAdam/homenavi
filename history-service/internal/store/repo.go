package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repo struct {
	db *gorm.DB
}

func OpenPostgres(user, password, dbName, host, port, sslMode string) (*gorm.DB, error) {
	if sslMode == "" {
		sslMode = "disable"
	}
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC", host, user, password, dbName, port, sslMode)
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

func New(db *gorm.DB) (*Repo, error) {
	if err := db.AutoMigrate(&DeviceStatePoint{}); err != nil {
		return nil, err
	}
	return &Repo{db: db}, nil
}

func (r *Repo) InsertStatePoint(ctx context.Context, p *DeviceStatePoint) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if p.IngestedAt.IsZero() {
		p.IngestedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Create(p).Error
}

type Page struct {
	Points     []DeviceStatePoint `json:"points"`
	NextCursor string             `json:"next_cursor,omitempty"`
}

func (r *Repo) ListStatePoints(ctx context.Context, deviceID string, from, to time.Time, limit int, cursor *Cursor, desc bool) (Page, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 10000 {
		limit = 10000
	}

	exprs := []clause.Expression{
		clause.Eq{Column: clause.Column{Name: "device_id"}, Value: deviceID},
	}
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

	order := clause.OrderBy{Columns: []clause.OrderByColumn{
		{Column: clause.Column{Name: "ts"}, Desc: desc},
		{Column: clause.Column{Name: "id"}, Desc: desc},
	}}

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
