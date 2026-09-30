package app

import (
	"encoding/json"
	"testing"

	"github.com/PetoAdam/homenavi/automation-service/internal/engine"
	"github.com/google/uuid"
)

func TestDemoWorkflows_AreValidAndStable(t *testing.T) {
	workflows, err := demoWorkflows()
	if err != nil {
		t.Fatalf("demoWorkflows() error = %v", err)
	}
	if len(workflows) != 3 {
		t.Fatalf("expected 3 demo workflows, got %d", len(workflows))
	}
	seen := make(map[uuid.UUID]struct{}, len(workflows))
	for _, workflow := range workflows {
		if workflow.ID == uuid.Nil {
			t.Fatal("expected stable workflow id")
		}
		if _, ok := seen[workflow.ID]; ok {
			t.Fatalf("duplicate workflow id %s", workflow.ID)
		}
		seen[workflow.ID] = struct{}{}
		var definition engine.Definition
		if err := json.Unmarshal(workflow.Definition, &definition); err != nil {
			t.Fatalf("unmarshal definition for %s: %v", workflow.Name, err)
		}
		if err := definition.NormalizeAndValidate(); err != nil {
			t.Fatalf("invalid definition for %s: %v", workflow.Name, err)
		}
	}
}