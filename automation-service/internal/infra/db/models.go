package db

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Workflow is a persisted automation definition.
// Definition is intentionally flexible JSON for forward-compatibility.
type Workflow struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Name           string         `gorm:"not null" json:"name"`
	Enabled        bool           `gorm:"not null;default:false" json:"enabled"`
	Definition     datatypes.JSON `gorm:"type:jsonb;not null" json:"definition"`
	SourceKind     string         `gorm:"type:varchar(16);not null;default:'graph'" json:"source_kind"`
	SourceFormat   string         `gorm:"type:varchar(32);not null;default:'graph-json'" json:"source_format"`
	SourceCode     string         `gorm:"type:text;not null;default:''" json:"source_code,omitempty"`
	SourceRevision int            `gorm:"not null;default:1" json:"source_revision"`
	CreatedBy      string         `gorm:"not null" json:"created_by"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type WorkflowRun struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	WorkflowID   uuid.UUID      `gorm:"type:uuid;index:idx_workflow_runs_workflow_id;not null" json:"workflow_id"`
	Workflow     *Workflow      `gorm:"foreignKey:WorkflowID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	Status       string         `gorm:"not null" json:"status"`
	TriggerEvent datatypes.JSON `gorm:"type:jsonb" json:"trigger_event,omitempty"`
	Error        string         `json:"error,omitempty"`
	StartedAt    time.Time      `gorm:"not null" json:"started_at"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
}

type WorkflowRunStep struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	RunID      uuid.UUID      `gorm:"type:uuid;index:idx_workflow_run_steps_run_id;not null" json:"run_id"`
	Run        *WorkflowRun   `gorm:"foreignKey:RunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	NodeID     string         `gorm:"not null" json:"node_id"`
	Status     string         `gorm:"not null" json:"status"`
	Input      datatypes.JSON `gorm:"type:jsonb" json:"input,omitempty"`
	Output     datatypes.JSON `gorm:"type:jsonb" json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	StartedAt  time.Time      `gorm:"not null" json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
}

// PendingCorrelation tracks an in-flight command correlation for run completion.
type PendingCorrelation struct {
	Corr        string           `gorm:"primaryKey" json:"corr"`
	RunID       uuid.UUID        `gorm:"type:uuid;index:idx_pending_correlations_run_id;not null" json:"run_id"`
	Run         *WorkflowRun     `gorm:"foreignKey:RunID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	WorkflowID  uuid.UUID        `gorm:"type:uuid;index:idx_pending_correlations_workflow_id;not null" json:"workflow_id"`
	Workflow    *Workflow        `gorm:"foreignKey:WorkflowID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	DeviceID    string           `gorm:"not null" json:"device_id"`
	HDPDeviceID *uuid.UUID       `gorm:"type:uuid;index" json:"hdp_device_id,omitempty"`
	HDPDevice   *hdpDeviceRecord `gorm:"foreignKey:HDPDeviceID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"-"`
	CreatedAt   time.Time        `gorm:"not null" json:"created_at"`
	ExpiresAt   time.Time        `gorm:"index:idx_pending_correlations_expires_at;not null" json:"expires_at"`
}

type hdpDeviceRecord struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	Protocol   string    `gorm:"column:protocol"`
	ExternalID string    `gorm:"column:external_id"`
}

func (hdpDeviceRecord) TableName() string { return "hdp_devices" }
