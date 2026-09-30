package db

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

func (r *Repository) SeedDemoWorkflows(ctx context.Context, workflows []Workflow) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		seedRepo := &Repository{db: tx}
		for _, workflow := range workflows {
			if err := seedRepo.upsertDemoWorkflow(ctx, workflow); err != nil {
				return fmt.Errorf("upsert workflow %s: %w", workflow.ID, err)
			}
		}
		return nil
	})
}

func (r *Repository) upsertDemoWorkflow(ctx context.Context, workflow Workflow) error {
		existing, err := r.GetWorkflow(ctx, workflow.ID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.CreateWorkflow(ctx, &workflow)
		}
		if err != nil {
			return err
		}
		existing.Name = workflow.Name
		existing.SortOrder = workflow.SortOrder
		existing.Enabled = workflow.Enabled
		existing.Definition = workflow.Definition
		existing.SourceKind = workflow.SourceKind
		existing.SourceFormat = workflow.SourceFormat
		existing.SourceCode = workflow.SourceCode
		existing.SourceRevision = workflow.SourceRevision
		existing.CreatedBy = workflow.CreatedBy
		return r.UpdateWorkflow(ctx, existing)
	}