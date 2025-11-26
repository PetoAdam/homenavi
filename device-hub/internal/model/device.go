package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Device struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Protocol     string         `gorm:"index,uniqueIndex:idx_devices_protocol_external;not null" json:"protocol"`
	ExternalID   string         `gorm:"index,uniqueIndex:idx_devices_protocol_external;not null" json:"external_id"` // zigbee ieee or friendly name
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	Manufacturer string         `json:"manufacturer"`
	Model        string         `json:"model"`
	Description  string         `gorm:"type:text" json:"description"`
	Firmware     string         `json:"firmware"`
	Icon         string         `json:"icon"`
	Capabilities datatypes.JSON `gorm:"type:jsonb" json:"capabilities"`
	Inputs       datatypes.JSON `gorm:"type:jsonb" json:"inputs"`
	Online       bool           `json:"online"`
	LastSeen     time.Time      `json:"last_seen"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// BeforeCreate GORM hook: ensure UUID is set
func (d *Device) BeforeCreate(tx *gorm.DB) (err error) {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}
