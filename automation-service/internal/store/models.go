package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Workflow is a persisted automation definition.
// Definition is intentionally flexible JSON for forward-compatibility.
type Workflow struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Name       string         `gorm:"not null" json:"name"`
	Enabled    bool           `gorm:"not null;default:false" json:"enabled"`
	Definition datatypes.JSON `gorm:"type:jsonb;not null" json:"definition"`
	CreatedBy  string         `gorm:"not null" json:"created_by"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

type WorkflowRun struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	WorkflowID   uuid.UUID      `gorm:"type:uuid;index:idx_workflow_runs_workflow_id;not null" json:"workflow_id"`
	Status       string         `gorm:"not null" json:"status"` // running|success|failed
	TriggerEvent datatypes.JSON `gorm:"type:jsonb" json:"trigger_event,omitempty"`
	Error        string         `json:"error,omitempty"`
	StartedAt    time.Time      `gorm:"not null" json:"started_at"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
}

type WorkflowRunStep struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	RunID      uuid.UUID      `gorm:"type:uuid;index:idx_workflow_run_steps_run_id;not null" json:"run_id"`
	NodeID     string         `gorm:"not null" json:"node_id"`
	Status     string         `gorm:"not null" json:"status"` // running|success|failed
	Input      datatypes.JSON `gorm:"type:jsonb" json:"input,omitempty"`
	Output     datatypes.JSON `gorm:"type:jsonb" json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	StartedAt  time.Time      `gorm:"not null" json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
}

// PendingCorrelation tracks an in-flight command correlation for run completion.
type PendingCorrelation struct {
	Corr       string    `gorm:"primaryKey" json:"corr"`
	RunID      uuid.UUID `gorm:"type:uuid;index:idx_pending_correlations_run_id;not null" json:"run_id"`
	WorkflowID uuid.UUID `gorm:"type:uuid;index:idx_pending_correlations_workflow_id;not null" json:"workflow_id"`
	DeviceID   string    `gorm:"not null" json:"device_id"`
	CreatedAt  time.Time `gorm:"not null" json:"created_at"`
	ExpiresAt  time.Time `gorm:"index:idx_pending_correlations_expires_at;not null" json:"expires_at"`
}
