package http

import "testing"

func TestPendingCommandSatisfied_ExpectedStateFallsBackToCorrWithChangedBaseline(t *testing.T) {
	entry := &pendingCommand{
		DeviceID: "zigbee/0x1234",
		Corr:     "corr-1",
		Expected: map[string]any{"power": "on"},
		Baseline: map[string]any{"state": false},
		StartedAt: 100,
	}
	state := map[string]any{
		"state":          true,
		"correlation_id": "corr-1",
	}

	if !pendingCommandSatisfied(entry, state, "corr-1", 110) {
		t.Fatalf("expected correlation-linked changed state to satisfy pending command")
	}
}

func TestPendingCommandSatisfied_ExpectedStateDoesNotClearOnCorrWithoutChange(t *testing.T) {
	entry := &pendingCommand{
		DeviceID: "zigbee/0x1234",
		Corr:     "corr-2",
		Expected: map[string]any{"power": "on"},
		Baseline: map[string]any{"state": false},
		StartedAt: 100,
	}
	state := map[string]any{
		"state": false,
	}

	if pendingCommandSatisfied(entry, state, "corr-2", 110) {
		t.Fatalf("expected unchanged state to remain pending")
	}
}

func TestPendingCommandSatisfied_ExpectedStateFallsBackToPostCommandChangedBaselineWithoutCorr(t *testing.T) {
	entry := &pendingCommand{
		DeviceID:  "zigbee/0x1234",
		Corr:      "corr-3",
		Expected:  map[string]any{"power": "on"},
		Baseline:  map[string]any{"state": false},
		StartedAt: 100,
	}
	state := map[string]any{
		"state": true,
	}

	if !pendingCommandSatisfied(entry, state, "", 140) {
		t.Fatalf("expected post-command changed baseline to satisfy pending command even without correlation")
	}
}

func TestPendingCommandSatisfied_ExpectedStateDoesNotFallbackBeforeCommandStart(t *testing.T) {
	entry := &pendingCommand{
		DeviceID:  "zigbee/0x1234",
		Corr:      "corr-4",
		Expected:  map[string]any{"power": "on"},
		Baseline:  map[string]any{"state": false},
		StartedAt: 200,
	}
	state := map[string]any{
		"state": true,
	}

	if pendingCommandSatisfied(entry, state, "", 150) {
		t.Fatalf("expected pre-command state changes to not satisfy pending command")
	}
}
