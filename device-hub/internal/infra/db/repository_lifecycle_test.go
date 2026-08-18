package db

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newLifecycleTestRepository(t *testing.T, now time.Time) (*Repository, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	database, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		NowFunc:                func() time.Time { return now },
		SkipDefaultTransaction: true,
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("gorm.Open: %v", err)
	}
	return &Repository{db: database}, mock, func() { _ = sqlDB.Close() }
}

func TestCreatePairingLifecycleRejectsSecondActiveSession(t *testing.T) {
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	repo, mock, closeDB := newLifecycleTestRepository(t, now)
	defer closeDB()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(hashtext($1))")).
		WithArgs("pairing:zigbee").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "hdp_pairing_lifecycles"`).
		WithArgs("zigbee", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "protocol", "terminal_at"}).AddRow("existing", "zigbee", nil))
	mock.ExpectCommit()

	created, err := repo.CreatePairingLifecycle(context.Background(), PairingLifecycle{ID: "new", Protocol: "zigbee", Status: "starting", Active: true, StartedAt: now, ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatalf("CreatePairingLifecycle: %v", err)
	}
	if created {
		t.Fatal("expected active protocol claim to be rejected")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestCompleteCommandLifecycleRejectsMissingRow(t *testing.T) {
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	repo, mock, closeDB := newLifecycleTestRepository(t, now)
	defer closeDB()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "hdp_command_lifecycles"`).
		WithArgs("corr-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"correlation_id"}))
	mock.ExpectCommit()

	_, won, err := repo.CompleteCommandLifecycle(context.Background(), "corr-1", "applied", "")
	if err != nil {
		t.Fatalf("CompleteCommandLifecycle: %v", err)
	}
	if won {
		t.Fatal("expected missing lifecycle transition to be ignored")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
