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
	if err := database.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(0x484e4155544f)).Error; err != nil {
			return fmt.Errorf("lock automation schema: %w", err)
		}
		return ensureSchema(tx)
	}); err != nil {
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
	if !m.HasColumn(&WorkflowRun{}, "IdempotencyKey") {
		if err := m.AddColumn(&WorkflowRun{}, "IdempotencyKey"); err != nil {
			return fmt.Errorf("add workflow_runs.idempotency_key: %w", err)
		}
	}
	if err := database.Model(&WorkflowRun{}).Where("idempotency_key = ''").Update("idempotency_key", nil).Error; err != nil {
		return fmt.Errorf("backfill workflow_runs.idempotency_key: %w", err)
	}
	if !m.HasIndex(&WorkflowRun{}, "idx_workflow_runs_idempotency_key") {
		if err := m.CreateIndex(&WorkflowRun{}, "idx_workflow_runs_idempotency_key"); err != nil {
			return fmt.Errorf("create workflow_runs idempotency index: %w", err)
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
	for _, model := range []any{&TriggerCooldown{}, &ScheduledTriggerClaim{}} {
		if !m.HasTable(model) {
			if err := m.CreateTable(model); err != nil {
				return fmt.Errorf("create lifecycle claim table: %w", err)
			}
		}
	}
	if !m.HasColumn(&PendingCorrelation{}, "HDPDeviceID") {
		if err := m.AddColumn(&PendingCorrelation{}, "HDPDeviceID"); err != nil {
			return fmt.Errorf("add pending_correlations.hdp_device_id: %w", err)
		}
	}
	for _, column := range []string{"DispatchPending", "ResultStatus", "ResultError"} {
		if !m.HasColumn(&PendingCorrelation{}, column) {
			if err := m.AddColumn(&PendingCorrelation{}, column); err != nil {
				return fmt.Errorf("add pending_correlations.%s: %w", column, err)
			}
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

func (r *Repository) GetRunByIdempotencyKey(ctx context.Context, key string) (*WorkflowRun, error) {
	var run WorkflowRun
	if err := r.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&run).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *Repository) FinishRun(ctx context.Context, runID uuid.UUID, status string, errMsg string) error {
	now := time.Now().UTC()
	updates := map[string]any{"status": status, "finished_at": &now, "error": errMsg}
	result := r.db.WithContext(ctx).Model(&WorkflowRun{}).
		Where("id = ? AND status = ? AND NOT EXISTS (SELECT 1 FROM workflow_run_steps WHERE run_id = workflow_runs.id AND status = ?)", runID, "running", "running").Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("run is missing, already finished, or has unfinished steps")
	}
	return nil
}

func (r *Repository) FailRun(ctx context.Context, runID uuid.UUID, errMsg string) ([]WorkflowRunStep, bool, error) {
	var steps []WorkflowRunStep
	finished := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pending []PendingCorrelation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("run_id = ?", runID).Find(&pending).Error; err != nil {
			return err
		}
		var run WorkflowRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", runID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if run.Status != "running" {
			return nil
		}
		if err := tx.Where("run_id = ? AND status = ?", runID, "running").Find(&steps).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Model(&WorkflowRunStep{}).Where("run_id = ? AND status = ?", runID, "running").Updates(map[string]any{"status": "failed", "finished_at": &now, "error": errMsg}).Error; err != nil {
			return err
		}
		if err := tx.Model(&WorkflowRun{}).Where("id = ?", runID).Updates(map[string]any{"status": "failed", "finished_at": &now, "error": errMsg}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&PendingCorrelation{}, "run_id = ?", runID).Error; err != nil {
			return err
		}
		finished = true
		return nil
	})
	return steps, finished, err
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
	result := r.db.WithContext(ctx).Model(&WorkflowRunStep{}).Where("id = ? AND status = ?", stepID, "running").Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("step is missing or already finished")
	}
	return nil
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
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&pending, "corr = ?", corr).Error; err != nil {
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

func (r *Repository) ResolvePendingCommand(ctx context.Context, corr string, resultStatus *string, resultError string) (*PendingCorrelation, error) {
	var completed *PendingCorrelation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pending PendingCorrelation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&pending, "corr = ?", corr).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if resultStatus != nil && pending.ResultStatus == nil {
			pending.ResultStatus = resultStatus
			pending.ResultError = resultError
		}
		if resultStatus == nil {
			pending.DispatchPending = false
		}
		if pending.DispatchPending || pending.ResultStatus == nil {
			return tx.Model(&PendingCorrelation{}).Where("corr = ?", corr).Updates(map[string]any{
				"dispatch_pending": pending.DispatchPending, "result_status": pending.ResultStatus, "result_error": pending.ResultError,
			}).Error
		}
		var err error
		completed, err = completePendingCommand(tx, &pending)
		return err
	})
	return completed, err
}

func completePendingCommand(tx *gorm.DB, pending *PendingCorrelation) (*PendingCorrelation, error) {
	now := time.Now().UTC()
	updated := tx.Model(&WorkflowRun{}).Where("id = ? AND status = ? AND NOT EXISTS (SELECT 1 FROM workflow_run_steps WHERE run_id = workflow_runs.id AND status = ?)", pending.RunID, "running", "running").Updates(map[string]any{
		"status": *pending.ResultStatus, "finished_at": &now, "error": pending.ResultError,
	})
	if updated.Error != nil {
		return nil, updated.Error
	}
	if updated.RowsAffected == 0 {
		var run WorkflowRun
		if err := tx.First(&run, "id = ?", pending.RunID).Error; err == nil && run.Status == "running" {
			return nil, errors.New("pending command run has unfinished steps")
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if err := tx.Delete(&PendingCorrelation{}, "corr = ?", pending.Corr).Error; err != nil {
		return nil, err
	}
	if updated.RowsAffected == 1 {
		return pending, nil
	}
	return nil, nil
}

func (r *Repository) FinishPendingDispatch(ctx context.Context, corr string, stepID uuid.UUID, onStepFinished func()) (*PendingCorrelation, error) {
	if onStepFinished == nil {
		return nil, errors.New("step completion callback is required")
	}
	var completed *PendingCorrelation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pending PendingCorrelation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&pending, "corr = ?", corr).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		updated := tx.Model(&WorkflowRunStep{}).Where("id = ? AND run_id = ? AND status = ?", stepID, pending.RunID, "running").Updates(map[string]any{"status": "success", "finished_at": &now, "error": ""})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("command step is missing or already finished")
		}
		pending.DispatchPending = false
		if pending.ResultStatus == nil {
			return tx.Model(&PendingCorrelation{}).Where("corr = ?", corr).Update("dispatch_pending", false).Error
		}
		var err error
		completed, err = completePendingCommand(tx, &pending)
		return err
	})
	if err != nil {
		return nil, err
	}
	onStepFinished()
	return completed, nil
}

type ExpiredPendingCommand struct {
	Pending PendingCorrelation
	Steps   []WorkflowRunStep
	Status  string
}

func (r *Repository) ExpirePendingCommands(ctx context.Context, now time.Time) ([]ExpiredPendingCommand, bool, error) {
	var expired []ExpiredPendingCommand
	var more bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pendingRows []PendingCorrelation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("expires_at <= ?", now).Order("expires_at").Limit(100).Find(&pendingRows).Error; err != nil {
			return err
		}
		more = len(pendingRows) == 100
		for _, pending := range pendingRows {
			var steps []WorkflowRunStep
			if err := tx.Where("run_id = ? AND status = ?", pending.RunID, "running").Find(&steps).Error; err != nil {
				return err
			}
			stepStatus, errMsg := "unknown", "command outcome unknown: dispatch or result timed out"
			if pending.ResultStatus != nil {
				stepStatus, errMsg = "success", ""
			}
			if err := tx.Model(&WorkflowRunStep{}).Where("run_id = ? AND status = ?", pending.RunID, "running").Updates(map[string]any{"status": stepStatus, "finished_at": &now, "error": errMsg}).Error; err != nil {
				return err
			}
			if pending.ResultStatus != nil {
				completed, err := completePendingCommand(tx, &pending)
				if err != nil {
					return err
				}
				if completed != nil {
					expired = append(expired, ExpiredPendingCommand{Pending: *completed, Steps: steps, Status: *completed.ResultStatus})
				}
				continue
			}
			updated := tx.Model(&WorkflowRun{}).Where("id = ? AND status = ?", pending.RunID, "running").Updates(map[string]any{"status": "unknown", "finished_at": &now, "error": errMsg})
			if updated.Error != nil {
				return updated.Error
			}
			if err := tx.Delete(&PendingCorrelation{}, "corr = ?", pending.Corr).Error; err != nil {
				return err
			}
			if updated.RowsAffected == 1 {
				expired = append(expired, ExpiredPendingCommand{Pending: pending, Steps: steps, Status: "unknown"})
			}
		}
		return nil
	})
	return expired, more, err
}

// ClaimTriggerCooldown atomically allows one trigger firing during a cooldown window.
func (r *Repository) ClaimTriggerCooldown(ctx context.Context, workflowID uuid.UUID, triggerNodeID string, cooldown time.Duration, now time.Time) (bool, error) {
	triggerNodeID = strings.TrimSpace(triggerNodeID)
	if workflowID == uuid.Nil || triggerNodeID == "" {
		return false, fmt.Errorf("workflow id and trigger node id are required")
	}
	if cooldown <= 0 {
		return true, nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	claim := TriggerCooldown{
		WorkflowID:    workflowID,
		TriggerNodeID: triggerNodeID,
		ExpiresAt:     now.Add(cooldown),
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "workflow_id"}, {Name: "trigger_node_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"expires_at": claim.ExpiresAt,
			"updated_at": now,
		}),
		Where: clause.Where{Exprs: []clause.Expression{
			clause.Expr{SQL: "trigger_cooldowns.expires_at <= ?", Vars: []any{now}},
		}},
	}).Create(&claim)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// ClaimScheduledTrigger atomically assigns one scheduled occurrence to one
// automation-service replica. Occurrence time is precise to a cron second.
func (r *Repository) ClaimScheduledTrigger(ctx context.Context, workflowID uuid.UUID, triggerNodeID string, occurredAt time.Time) (bool, error) {
	triggerNodeID = strings.TrimSpace(triggerNodeID)
	if workflowID == uuid.Nil || triggerNodeID == "" {
		return false, fmt.Errorf("workflow id and trigger node id are required")
	}
	if occurredAt.IsZero() {
		return false, fmt.Errorf("scheduled occurrence time is required")
	}
	claim := ScheduledTriggerClaim{
		WorkflowID:    workflowID,
		TriggerNodeID: triggerNodeID,
		OccurredAt:    occurredAt.UTC().Truncate(time.Second),
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&claim)
	return result.RowsAffected == 1, result.Error
}

func (r *Repository) PruneScheduledTriggerClaims(ctx context.Context, before time.Time) error {
	if before.IsZero() {
		before = time.Now().UTC().Add(-24 * time.Hour)
	}
	return r.db.WithContext(ctx).Where("occurred_at < ?", before).Delete(&ScheduledTriggerClaim{}).Error
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
