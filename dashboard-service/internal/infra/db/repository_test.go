package db

import "testing"

func TestDashboardRecordTableName(t *testing.T) {
	if got := (dashboardRecord{}).TableName(); got != "dashboards" {
		t.Fatalf("expected dashboards table, got %q", got)
	}
}
