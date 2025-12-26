package store

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type Repo struct {
	db *gorm.DB
}

func OpenPostgres(user, password, dbName, host, port, sslMode string) (*gorm.DB, error) {
	if sslMode == "" {
		sslMode = "disable"
	}
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC", host, user, password, dbName, port, sslMode)
	// Reduce log noise: "record not found" is expected in normal correlation flows.
	// Keep warnings/errors, and suppress ErrRecordNotFound specifically.
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

func New(db *gorm.DB) (*Repo, error) {
	// Ensure minimal schema exists.
	if err := ensureSchema(db); err != nil {
		return nil, err
	}
	return &Repo{db: db}, nil
}

func ensureSchema(db *gorm.DB) error {
	m := db.Migrator()

	// Create missing tables only. We intentionally avoid AutoMigrate here because it
	// can trigger driver/migrator edge-cases in some environments; our schema is
	// stable and managed by explicit model definitions.
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

	// Ensure indexes exist (names come from struct tags in models.go).
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

func (r *Repo) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	var rows []Workflow
	if err := r.db.WithContext(ctx).Order("created_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repo) GetWorkflow(ctx context.Context, id uuid.UUID) (*Workflow, error) {
	var w Workflow
	if err := r.db.WithContext(ctx).First(&w, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *Repo) CreateWorkflow(ctx context.Context, w *Workflow) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(w).Error
}

func (r *Repo) UpdateWorkflow(ctx context.Context, w *Workflow) error {
	return r.db.WithContext(ctx).Save(w).Error
}

func (r *Repo) DeleteWorkflow(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&Workflow{}, "id = ?", id).Error
}

func (r *Repo) SetWorkflowEnabled(ctx context.Context, id uuid.UUID, enabled bool) error {
	return r.db.WithContext(ctx).Model(&Workflow{}).Where("id = ?", id).Update("enabled", enabled).Error
}

func (r *Repo) CreateRun(ctx context.Context, run *WorkflowRun) error {
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

func (r *Repo) FinishRun(ctx context.Context, runID uuid.UUID, status string, errMsg string) error {
	now := time.Now().UTC()
	updates := map[string]any{"status": status, "finished_at": &now, "error": errMsg}
	return r.db.WithContext(ctx).Model(&WorkflowRun{}).Where("id = ?", runID).Updates(updates).Error
}

func (r *Repo) CreateStep(ctx context.Context, step *WorkflowRunStep) error {
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

func (r *Repo) FinishStep(ctx context.Context, stepID uuid.UUID, status string, errMsg string) error {
	now := time.Now().UTC()
	updates := map[string]any{"status": status, "finished_at": &now, "error": errMsg}
	return r.db.WithContext(ctx).Model(&WorkflowRunStep{}).Where("id = ?", stepID).Updates(updates).Error
}

func (r *Repo) ListRuns(ctx context.Context, workflowID uuid.UUID, limit int) ([]WorkflowRun, error) {
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

func (r *Repo) GetRunWithSteps(ctx context.Context, runID uuid.UUID) (*WorkflowRun, []WorkflowRunStep, error) {
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

func (r *Repo) UpsertPendingCorr(ctx context.Context, p *PendingCorrelation) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(p).Error
}

func (r *Repo) ConsumePendingCorr(ctx context.Context, corr string) (*PendingCorrelation, error) {
	var p PendingCorrelation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&p, "corr = ?", corr).Error; err != nil {
			return err
		}
		return tx.Delete(&PendingCorrelation{}, "corr = ?", corr).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Normal case: correlation wasn't tracked or already consumed.
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *Repo) PruneExpiredPending(ctx context.Context) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Where("expires_at < ?", now).Delete(&PendingCorrelation{}).Error
}
