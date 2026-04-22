package db

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/PetoAdam/homenavi/shared/dbx"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// Config holds database connectivity settings for the current SQL backend.
type Config = dbx.PostgresConfig

// Repository is the database-backed automation repository implementation.
type Repository struct {
	db *gorm.DB
}

func Open(cfg Config) (*gorm.DB, error) {
	dsn := dbx.BuildPostgresDSN(cfg)
	gormLogger := logger.New(
		log.New(os.Stdout, "", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
	return gorm.Open(
		postgres.New(postgres.Config{DSN: dsn}),
		&gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: gormLogger},
	)
}

func New(database *gorm.DB) (*Repository, error) {
	if err := ensureSchema(database); err != nil {
		return nil, err
	}
	return &Repository{db: database}, nil
}

func ensureSchema(database *gorm.DB) error {
	m := database.Migrator()
	if !m.HasTable(&Workflow{}) {
		if err := m.CreateTable(&Workflow{}); err != nil {
			return fmt.Errorf("create table workflows: %w", err)
		}
	}
	if !m.HasTable(&WorkflowRun{}) {
		if err := m.CreateTable(&WorkflowRun{}); err != nil {
			return fmt.Errorf("create table workflow_runs: %w", err)
		}
	}
	if !m.HasTable(&WorkflowRunStep{}) {
		if err := m.CreateTable(&WorkflowRunStep{}); err != nil {
			return fmt.Errorf("create table workflow_run_steps: %w", err)
		}
	}
	if !m.HasTable(&PendingCorrelation{}) {
		if err := m.CreateTable(&PendingCorrelation{}); err != nil {
			return fmt.Errorf("create table pending_correlations: %w", err)
		}
	}
	if !m.HasIndex(&WorkflowRun{}, "WorkflowID") {
		if err := m.CreateIndex(&WorkflowRun{}, "WorkflowID"); err != nil {
			return fmt.Errorf("create index workflow_runs.workflow_id: %w", err)
		}
	}
	if !m.HasIndex(&WorkflowRunStep{}, "RunID") {
		if err := m.CreateIndex(&WorkflowRunStep{}, "RunID"); err != nil {
			return fmt.Errorf("create index workflow_run_steps.run_id: %w", err)
		}
	}
	if !m.HasIndex(&PendingCorrelation{}, "RunID") {
		if err := m.CreateIndex(&PendingCorrelation{}, "RunID"); err != nil {
			return fmt.Errorf("create index pending_correlations.run_id: %w", err)
		}
	}
	if !m.HasIndex(&PendingCorrelation{}, "WorkflowID") {
		if err := m.CreateIndex(&PendingCorrelation{}, "WorkflowID"); err != nil {
			return fmt.Errorf("create index pending_correlations.workflow_id: %w", err)
		}
	}
	if !m.HasIndex(&PendingCorrelation{}, "ExpiresAt") {
		if err := m.CreateIndex(&PendingCorrelation{}, "ExpiresAt"); err != nil {
			return fmt.Errorf("create index pending_correlations.expires_at: %w", err)
		}
	}
	return nil
}

func (r *Repository) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	var rows []Workflow
	if err := r.db.WithContext(ctx).Order("created_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) GetWorkflow(ctx context.Context, id uuid.UUID) (*Workflow, error) {
	var workflow Workflow
	if err := r.db.WithContext(ctx).First(&workflow, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &workflow, nil
}

func (r *Repository) CreateWorkflow(ctx context.Context, workflow *Workflow) error {
	if workflow.ID == uuid.Nil {
		workflow.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(workflow).Error
}

func (r *Repository) UpdateWorkflow(ctx context.Context, workflow *Workflow) error {
	return r.db.WithContext(ctx).Save(workflow).Error
}

func (r *Repository) DeleteWorkflow(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&Workflow{}, "id = ?", id).Error
}

func (r *Repository) SetWorkflowEnabled(ctx context.Context, id uuid.UUID, enabled bool) error {
	return r.db.WithContext(ctx).Model(&Workflow{}).Where("id = ?", id).Update("enabled", enabled).Error
}

func (r *Repository) CreateRun(ctx context.Context, run *WorkflowRun) error {
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	if run.Status == "" {
		run.Status = "running"
	}
	return r.db.WithContext(ctx).Create(run).Error
}

func (r *Repository) FinishRun(ctx context.Context, runID uuid.UUID, status string, errMsg string) error {
	now := time.Now().UTC()
	updates := map[string]any{"status": status, "finished_at": &now, "error": errMsg}
	return r.db.WithContext(ctx).Model(&WorkflowRun{}).Where("id = ?", runID).Updates(updates).Error
}

func (r *Repository) CreateStep(ctx context.Context, step *WorkflowRunStep) error {
	if step.ID == uuid.Nil {
		step.ID = uuid.New()
	}
	if step.StartedAt.IsZero() {
		step.StartedAt = time.Now().UTC()
	}
	if step.Status == "" {
		step.Status = "running"
	}
	return r.db.WithContext(ctx).Create(step).Error
}

func (r *Repository) FinishStep(ctx context.Context, stepID uuid.UUID, status string, errMsg string) error {
	now := time.Now().UTC()
	updates := map[string]any{"status": status, "finished_at": &now, "error": errMsg}
	return r.db.WithContext(ctx).Model(&WorkflowRunStep{}).Where("id = ?", stepID).Updates(updates).Error
}

func (r *Repository) ListRuns(ctx context.Context, workflowID uuid.UUID, limit int) ([]WorkflowRun, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []WorkflowRun
	q := r.db.WithContext(ctx).Where("workflow_id = ?", workflowID).Order("started_at desc").Limit(limit)
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repository) GetRunWithSteps(ctx context.Context, runID uuid.UUID) (*WorkflowRun, []WorkflowRunStep, error) {
	var run WorkflowRun
	if err := r.db.WithContext(ctx).First(&run, "id = ?", runID).Error; err != nil {
		return nil, nil, err
	}
	var steps []WorkflowRunStep
	if err := r.db.WithContext(ctx).Where("run_id = ?", runID).Order("started_at asc").Find(&steps).Error; err != nil {
		return &run, nil, err
	}
	return &run, steps, nil
}

func (r *Repository) UpsertPendingCorr(ctx context.Context, pending *PendingCorrelation) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(pending).Error
}

func (r *Repository) ConsumePendingCorr(ctx context.Context, corr string) (*PendingCorrelation, error) {
	var pending PendingCorrelation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&pending, "corr = ?", corr).Error; err != nil {
			return err
		}
		return tx.Delete(&PendingCorrelation{}, "corr = ?", corr).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &pending, nil
}

func (r *Repository) PruneExpiredPending(ctx context.Context) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Where("expires_at < ?", now).Delete(&PendingCorrelation{}).Error
}
