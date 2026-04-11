package http

import "testing"

func TestPendingCommandSatisfied_ExpectedStateFallsBackToCorrWithChangedBaseline(t *testing.T) {
	entry := &pendingCommand{
		DeviceID: "zigbee/0x1234",
		Corr:     "corr-1",
		Expected: map[string]any{"power": "on"},
		Baseline: map[string]any{"state": false},
	}
	state := map[string]any{
		"state":          true,
		"correlation_id": "corr-1",
	}

	if !pendingCommandSatisfied(entry, state, "corr-1") {
		t.Fatalf("expected correlation-linked changed state to satisfy pending command")
	}
}

func TestPendingCommandSatisfied_ExpectedStateDoesNotClearOnCorrWithoutChange(t *testing.T) {
	entry := &pendingCommand{
		DeviceID: "zigbee/0x1234",
		Corr:     "corr-2",
		Expected: map[string]any{"power": "on"},
		Baseline: map[string]any{"state": false},
	}
	state := map[string]any{
		"state": false,
	}

	if pendingCommandSatisfied(entry, state, "corr-2") {
		t.Fatalf("expected unchanged state to remain pending")
	}
}
