package db

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Room struct {
	ID        uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	Slug      string         `json:"slug" gorm:"uniqueIndex;not null"`
	Name      string         `json:"name" gorm:"not null"`
	SortOrder int            `json:"sort_order"`
	Meta      datatypes.JSON `json:"meta" gorm:"type:jsonb"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

func (Room) TableName() string { return "ers_rooms" }

type Tag struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	Slug      string    `json:"slug" gorm:"uniqueIndex;not null"`
	Name      string    `json:"name" gorm:"not null"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Tag) TableName() string { return "ers_tags" }

type Group struct {
	ID          uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	Slug        string         `json:"slug" gorm:"uniqueIndex;not null"`
	Name        string         `json:"name" gorm:"not null"`
	Description string         `json:"description"`
	Meta        datatypes.JSON `json:"meta" gorm:"type:jsonb"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func (Group) TableName() string { return "ers_groups" }

type Device struct {
	ID          uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	Name        string         `json:"name" gorm:"not null"`
	Description string         `json:"description"`
	RoomID      *uuid.UUID     `json:"room_id" gorm:"type:uuid"`
	Room        *Room          `json:"-" gorm:"foreignKey:RoomID"`
	Meta        datatypes.JSON `json:"meta" gorm:"type:jsonb"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func (Device) TableName() string { return "ers_devices" }

type DeviceTag struct {
	DeviceID uuid.UUID `gorm:"type:uuid;primaryKey;index;not null"`
	TagID    uuid.UUID `gorm:"type:uuid;primaryKey;index;not null"`
}

func (DeviceTag) TableName() string { return "ers_device_tags" }

type GroupMember struct {
	GroupID  uuid.UUID `gorm:"type:uuid;primaryKey;index;not null"`
	DeviceID uuid.UUID `gorm:"type:uuid;primaryKey;index;not null"`
}

func (GroupMember) TableName() string { return "ers_group_members" }

type DeviceBinding struct {
	ID         uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	DeviceID   uuid.UUID `json:"device_id" gorm:"type:uuid;index;not null"`
	Kind       string    `json:"kind" gorm:"index;not null;uniqueIndex:idx_kind_external"`
	ExternalID string    `json:"external_id" gorm:"index;not null;uniqueIndex:idx_kind_external"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (DeviceBinding) TableName() string { return "ers_device_bindings" }
