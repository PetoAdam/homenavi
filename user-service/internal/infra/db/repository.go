package db

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/PetoAdam/homenavi/user-service/internal/users"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Config holds database connectivity settings for the current SQL backend.
type Config struct {
	Host     string
	User     string
	Password string
	DBName   string
	Port     string
	SSLMode  string
}

// Repository is the database-backed users repository implementation.
type Repository struct {
	db *gorm.DB
}

func New(cfg Config, logger *slog.Logger) (*Repository, error) {
	dsn := dbx.BuildPostgresDSN(dbx.PostgresConfig{Host: cfg.Host, User: cfg.User, Password: cfg.Password, DBName: cfg.DBName, Port: cfg.Port, SSLMode: cfg.SSLMode})
	var db *gorm.DB
	var err error
	for i := 0; i < 30; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			if err = db.AutoMigrate(&userRow{}, &emailVerificationRow{}); err == nil {
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
	repo := &Repository{db: db}
	if err := repo.ensureDefaultAdmin(); err != nil && logger != nil {
		logger.Error("failed to ensure default admin", "error", err)
	}
	return repo, nil
}

func (r *Repository) Create(ctx context.Context, user *users.User) error {
	row := toUserRow(*user)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	*user = toUser(row)
	return nil
}

func (r *Repository) FindByID(ctx context.Context, id uuid.UUID) (users.User, error) {
	var row userRow
	if err := r.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return users.User{}, normalizeError(err)
	}
	return toUser(row), nil
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (users.User, error) {
	var row userRow
	if err := r.db.WithContext(ctx).Where(&userRow{Email: email}).First(&row).Error; err != nil {
		return users.User{}, normalizeError(err)
	}
	return toUser(row), nil
}

func (r *Repository) FindByUserName(ctx context.Context, userName string) (users.User, error) {
	var row userRow
	if err := r.db.WithContext(ctx).Where(&userRow{UserName: userName}).First(&row).Error; err != nil {
		return users.User{}, normalizeError(err)
	}
	return toUser(row), nil
}

func (r *Repository) FindByGoogleID(ctx context.Context, googleID string) (users.User, error) {
	var row userRow
	if err := r.db.WithContext(ctx).Where(&userRow{GoogleID: &googleID}).First(&row).Error; err != nil {
		return users.User{}, normalizeError(err)
	}
	return toUser(row), nil
}

func (r *Repository) List(ctx context.Context, query string, page, size int) ([]users.User, int64, error) {
	offset := (page - 1) * size
	dbq := r.db.WithContext(ctx).Model(&userRow{})
	if query != "" {
		like := "%" + escapeLike(query) + "%"
		likeNorm := "%" + strings.ToUpper(escapeLike(query)) + "%"
		dbq = dbq.Where(clause.Like{Column: clause.Column{Name: "normalized_email"}, Value: likeNorm}).
			Or(clause.Like{Column: clause.Column{Name: "normalized_user_name"}, Value: likeNorm}).
			Or(clause.Like{Column: clause.Column{Name: "first_name"}, Value: like}).
			Or(clause.Like{Column: clause.Column{Name: "last_name"}, Value: like})
	}
	var total int64
	if err := dbq.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []userRow
	if err := dbq.Order(clause.OrderByColumn{Column: clause.Column{Name: "created_at"}, Desc: true}).Offset(offset).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]users.User, 0, len(rows))
	for _, row := range rows {
		items = append(items, toUser(row))
	}
	return items, total, nil
}

func (r *Repository) UpdateFields(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	return normalizeError(r.db.WithContext(ctx).Model(&userRow{ID: id}).Updates(fields).Error)
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	return normalizeError(r.db.WithContext(ctx).Delete(&userRow{}, id).Error)
}

func (r *Repository) ensureDefaultAdmin() error {
	const adminEmail = "admin@example.com"
	const adminUser = "admin"
	if _, err := r.FindByEmail(context.Background(), adminEmail); err == nil {
		return nil
	} else if !errors.Is(err, users.ErrNotFound) {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	ph := string(hash)
	user := userRow{
		ID:                 uuid.New(),
		UserName:           adminUser,
		NormalizedUserName: strings.ToUpper(adminUser),
		Email:              adminEmail,
		NormalizedEmail:    strings.ToUpper(adminEmail),
		Role:               "admin",
		PasswordHash:       &ph,
	}
	return r.db.Create(&user).Error
}

func normalizeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return users.ErrNotFound
	}
	return err
}

func escapeLike(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '%', '_', '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

var _ users.Repository = (*Repository)(nil)
