package db

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
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
		&gorm.Config{Logger: gormLogger},
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
			return fmt.Errorf("create workflows: %w", err)
		}
	}
	for _, column := range []string{"SortOrder", "SourceKind", "SourceFormat", "SourceCode", "SourceRevision"} {
		if !m.HasColumn(&Workflow{}, column) {
			if err := m.AddColumn(&Workflow{}, column); err != nil {
				return fmt.Errorf("add workflows.%s: %w", strings.ToLower(column), err)
			}
		}
	}
	if !m.HasTable(&WorkflowRun{}) {
		if err := m.CreateTable(&WorkflowRun{}); err != nil {
			return fmt.Errorf("create workflow_runs: %w", err)
		}
	}
	if !m.HasTable(&WorkflowRunStep{}) {
		if err := m.CreateTable(&WorkflowRunStep{}); err != nil {
			return fmt.Errorf("create workflow_run_steps: %w", err)
		}
	}
	if !m.HasTable(&PendingCorrelation{}) {
		if err := m.CreateTable(&PendingCorrelation{}); err != nil {
			return fmt.Errorf("create pending_correlations: %w", err)
		}
	}
	if !m.HasColumn(&PendingCorrelation{}, "HDPDeviceID") {
		if err := m.AddColumn(&PendingCorrelation{}, "HDPDeviceID"); err != nil {
			return fmt.Errorf("add pending_correlations.hdp_device_id: %w", err)
		}
	}
	if !m.HasConstraint(&WorkflowRun{}, "Workflow") {
		_ = m.CreateConstraint(&WorkflowRun{}, "Workflow")
	}
	if !m.HasConstraint(&WorkflowRunStep{}, "Run") {
		_ = m.CreateConstraint(&WorkflowRunStep{}, "Run")
	}
	if !m.HasConstraint(&PendingCorrelation{}, "Run") {
		_ = m.CreateConstraint(&PendingCorrelation{}, "Run")
	}
	if !m.HasConstraint(&PendingCorrelation{}, "Workflow") {
		_ = m.CreateConstraint(&PendingCorrelation{}, "Workflow")
	}
	if m.HasTable(&hdpDeviceRecord{}) && !m.HasConstraint(&PendingCorrelation{}, "HDPDevice") {
		_ = m.CreateConstraint(&PendingCorrelation{}, "HDPDevice")
	}
	if err := backfillWorkflowSourceDefaults(database.WithContext(context.Background())); err != nil {
		return err
	}
	if err := backfillWorkflowSortOrder(database.WithContext(context.Background())); err != nil {
		return err
	}
	if err := backfillPendingCorrelationHDPDeviceIDs(database.WithContext(context.Background())); err != nil {
		return err
	}
	return nil
}

func backfillWorkflowSourceDefaults(database *gorm.DB) error {
	if err := database.Model(&Workflow{}).Where("source_kind = '' OR source_kind IS NULL").Update("source_kind", "graph").Error; err != nil {
		return fmt.Errorf("backfill workflows.source_kind: %w", err)
	}
	if err := database.Model(&Workflow{}).Where("source_format = '' OR source_format IS NULL").Update("source_format", "graph-json").Error; err != nil {
		return fmt.Errorf("backfill workflows.source_format: %w", err)
	}
	if err := database.Model(&Workflow{}).Where("source_code IS NULL").Update("source_code", "").Error; err != nil {
		return fmt.Errorf("backfill workflows.source_code: %w", err)
	}
	if err := database.Model(&Workflow{}).Where("source_revision IS NULL OR source_revision = 0").Update("source_revision", 1).Error; err != nil {
		return fmt.Errorf("backfill workflows.source_revision: %w", err)
	}
	return nil
}

func backfillWorkflowSortOrder(database *gorm.DB) error {
	if err := database.Exec(`
		WITH ordered AS (
			SELECT id, ROW_NUMBER() OVER (ORDER BY created_at ASC, id ASC) AS position
			FROM workflows
			WHERE sort_order IS NULL OR sort_order = 0
		)
		UPDATE workflows AS workflows
		SET sort_order = ordered.position
		FROM ordered
		WHERE workflows.id = ordered.id
	`).Error; err != nil {
		return fmt.Errorf("backfill workflows.sort_order: %w", err)
	}
	return nil
}

func backfillPendingCorrelationHDPDeviceIDs(database *gorm.DB) error {
	var pendingRows []PendingCorrelation
	if err := database.Where("hdp_device_id IS NULL AND device_id <> ''").Find(&pendingRows).Error; err != nil {
		return fmt.Errorf("load pending correlations for backfill: %w", err)
	}
	for _, pending := range pendingRows {
		hdpDeviceID, err := resolveHDPDeviceID(database, pending.DeviceID)
		if err != nil {
			return fmt.Errorf("resolve pending correlation %s: %w", pending.Corr, err)
		}
		if hdpDeviceID == nil {
			continue
		}
		if err := database.Model(&PendingCorrelation{}).Where("corr = ?", pending.Corr).Update("hdp_device_id", *hdpDeviceID).Error; err != nil {
			return fmt.Errorf("update pending correlation %s: %w", pending.Corr, err)
		}
	}
	return nil
}

func resolveHDPDeviceID(database *gorm.DB, externalRef string) (*uuid.UUID, error) {
	protocol, externalID, ok := splitHDPExternalRef(externalRef)
	if !ok {
		return nil, nil
	}
	device, err := loadHDPDeviceByExternal(database, protocol, externalID)
	if err != nil {
		return nil, err
	}
	if device == nil {
		return nil, nil
	}
	id := device.ID
	return &id, nil
}

func loadHDPDeviceByExternal(database *gorm.DB, protocol, externalID string) (*hdpDeviceRecord, error) {
	if !database.Migrator().HasTable(&hdpDeviceRecord{}) {
		return nil, nil
	}
	var device hdpDeviceRecord
	if err := database.Where("protocol = ? AND external_id = ?", protocol, externalID).First(&device).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &device, nil
}

func splitHDPExternalRef(externalRef string) (string, string, bool) {
	trimmed := strings.TrimSpace(externalRef)
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	protocol := strings.TrimSpace(parts[0])
	externalID := strings.TrimSpace(parts[1])
	if protocol == "" || externalID == "" {
		return "", "", false
	}
	return protocol, externalID, true
}

func (r *Repository) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	var rows []Workflow
	if err := r.db.WithContext(ctx).Order("sort_order desc, created_at desc").Find(&rows).Error; err != nil {
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
	if workflow.SortOrder == 0 {
		nextOrder, err := r.nextWorkflowSortOrder(ctx)
		if err != nil {
			return err
		}
		workflow.SortOrder = nextOrder
	}
	applyWorkflowSourceDefaults(workflow)
	return r.db.WithContext(ctx).Create(workflow).Error
}

func (r *Repository) UpdateWorkflow(ctx context.Context, workflow *Workflow) error {
	applyWorkflowSourceDefaults(workflow)
	return r.db.WithContext(ctx).Save(workflow).Error
}

func (r *Repository) DeleteWorkflow(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&Workflow{}, "id = ?", id).Error
}

func (r *Repository) SetWorkflowEnabled(ctx context.Context, id uuid.UUID, enabled bool) error {
	return r.db.WithContext(ctx).Model(&Workflow{}).Where("id = ?", id).Update("enabled", enabled).Error
}

func (r *Repository) ReorderWorkflows(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for index, id := range ids {
			order := len(ids) - index
			if err := tx.Model(&Workflow{}).Where("id = ?", id).Update("sort_order", order).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) nextWorkflowSortOrder(ctx context.Context) (int, error) {
	var next int
	if err := r.db.WithContext(ctx).Model(&Workflow{}).Select("COALESCE(MAX(sort_order), 0) + 1").Scan(&next).Error; err != nil {
		return 0, err
	}
	if next <= 0 {
		return 1, nil
	}
	return next, nil
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
	if pending != nil && pending.HDPDeviceID == nil {
		hdpDeviceID, err := resolveHDPDeviceID(r.db.WithContext(ctx), pending.DeviceID)
		if err != nil {
			return err
		}
		pending.HDPDeviceID = hdpDeviceID
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(pending).Error
}

func (r *Repository) ResolveHDPDeviceIDByExternalRef(ctx context.Context, externalRef string) (uuid.UUID, bool, error) {
	hdpDeviceID, err := resolveHDPDeviceID(r.db.WithContext(ctx), externalRef)
	if err != nil {
		return uuid.Nil, false, err
	}
	if hdpDeviceID == nil {
		return uuid.Nil, false, nil
	}
	return *hdpDeviceID, true, nil
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

func applyWorkflowSourceDefaults(workflow *Workflow) {
	if workflow == nil {
		return
	}
	workflow.SourceKind = strings.TrimSpace(workflow.SourceKind)
	if workflow.SourceKind == "" {
		workflow.SourceKind = "graph"
	}
	workflow.SourceFormat = strings.TrimSpace(workflow.SourceFormat)
	if workflow.SourceFormat == "" {
		workflow.SourceFormat = "graph-json"
	}
	if workflow.SourceRevision <= 0 {
		workflow.SourceRevision = 1
	}
	if workflow.SourceCode == "" && workflow.SourceKind == "graph" {
		workflow.SourceCode = ""
	}
}
