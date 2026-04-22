package db

import (
	"fmt"

	"github.com/PetoAdam/homenavi/shared/dbx"
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
