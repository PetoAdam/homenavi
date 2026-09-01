package db

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/dbx"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type Config = dbx.PostgresConfig

type Repository struct {
	db *gorm.DB
}

type InstallStatus struct {
	ID        string    `gorm:"primaryKey;size:128"`
	Stage     string    `gorm:"size:64;not null"`
	Progress  int       `gorm:"not null"`
	Message   string    `gorm:"type:text"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (InstallStatus) TableName() string { return "integration_operation_statuses" }

type UpdateStatus struct {
	ID               string `gorm:"primaryKey;size:128"`
	InstalledVersion string `gorm:"size:128"`
	LatestVersion    string `gorm:"size:128"`
	UpdateAvailable  bool   `gorm:"not null"`
	AutoUpdate       bool   `gorm:"not null"`
	CheckedAt        *time.Time
	Error            string    `gorm:"type:text"`
	InProgress       bool      `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (UpdateStatus) TableName() string { return "integration_update_statuses" }

func Open(cfg Config) (*Repository, error) {
	database, err := gorm.Open(postgres.Open(dbx.BuildPostgresDSN(cfg)), &gorm.Config{Logger: logger.New(log.New(os.Stdout, "", log.LstdFlags), logger.Config{SlowThreshold: 2 * time.Second, LogLevel: logger.Warn, IgnoreRecordNotFoundError: true, Colorful: false})})
	if err != nil {
		return nil, err
	}
	if err := database.AutoMigrate(&InstallStatus{}, &UpdateStatus{}); err != nil {
		return nil, err
	}
	return &Repository{db: database}, nil
}

func (r *Repository) SetInstallStatus(ctx context.Context, status InstallStatus) error {
	status.ID = strings.TrimSpace(status.ID)
	status.Stage = strings.TrimSpace(status.Stage)
	if status.UpdatedAt.IsZero() {
		status.UpdatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Save(&status).Error
}

func (r *Repository) GetInstallStatus(ctx context.Context, id string) (InstallStatus, bool, error) {
	var status InstallStatus
	err := r.db.WithContext(ctx).First(&status, "id = ?", strings.TrimSpace(id)).Error
	if err == gorm.ErrRecordNotFound {
		return InstallStatus{}, false, nil
	}
	return status, err == nil, err
}

func (r *Repository) SaveUpdateStatus(ctx context.Context, status UpdateStatus) error {
	status.ID = strings.TrimSpace(status.ID)
	if status.UpdatedAt.IsZero() {
		status.UpdatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Save(&status).Error
}

func (r *Repository) ListUpdateStatuses(ctx context.Context) ([]UpdateStatus, error) {
	var statuses []UpdateStatus
	if err := r.db.WithContext(ctx).Order("id asc").Find(&statuses).Error; err != nil {
		return nil, err
	}
	return statuses, nil
}

func (r *Repository) GetUpdateStatus(ctx context.Context, id string) (UpdateStatus, bool, error) {
	var status UpdateStatus
	err := r.db.WithContext(ctx).First(&status, "id = ?", strings.TrimSpace(id)).Error
	if err == gorm.ErrRecordNotFound {
		return UpdateStatus{}, false, nil
	}
	return status, err == nil, err
}

func (r *Repository) MutateUpdateStatus(ctx context.Context, id string, mutate func(*UpdateStatus) error) (UpdateStatus, error) {
	id = strings.TrimSpace(id)
	var status UpdateStatus
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&UpdateStatus{ID: id, UpdatedAt: now}).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&status, "id = ?", id).Error; err != nil {
			return err
		}
		if err := mutate(&status); err != nil {
			return err
		}
		status.UpdatedAt = now
		return tx.Save(&status).Error
	})
	return status, err
}

func (r *Repository) ClaimUpdate(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
	}
	_, err := r.MutateUpdateStatus(ctx, id, func(status *UpdateStatus) error {
		if status.InProgress {
			return gorm.ErrInvalidData
		}
		status.InProgress = true
		status.Error = ""
		return nil
	})
	if err == gorm.ErrInvalidData {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) DeleteOperationState(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&InstallStatus{}, "id = ?", id).Error; err != nil {
			return err
		}
		return tx.Delete(&UpdateStatus{}, "id = ?", id).Error
	})
}
