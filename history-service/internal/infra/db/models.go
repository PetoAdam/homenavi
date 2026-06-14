package db

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type DeviceStatePoint struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	DeviceID    string         `gorm:"index:idx_device_ts,priority:1" json:"device_id"`
	HDPDeviceID *uuid.UUID     `gorm:"type:uuid;index" json:"hdp_device_id,omitempty"`
	TS          time.Time      `gorm:"index:idx_device_ts,priority:2" json:"ts"`
	Payload     datatypes.JSON `gorm:"type:jsonb" json:"payload"`
	Topic       string         `json:"topic"`
	Retained    bool           `json:"retained"`
	IngestedAt  time.Time      `json:"ingested_at"`
}

func (DeviceStatePoint) TableName() string { return "hdp_device_state_points" }

type hdpDeviceRecord struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	Protocol   string    `gorm:"column:protocol"`
	ExternalID string    `gorm:"column:external_id"`
}

func (hdpDeviceRecord) TableName() string { return "hdp_devices" }
