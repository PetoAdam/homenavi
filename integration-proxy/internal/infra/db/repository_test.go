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

func newTestRepository(t *testing.T, now time.Time) (*Repository, sqlmock.Sqlmock, func()) {
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

func TestClaimUpdateClaimsAvailableRow(t *testing.T) {
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	repo, mock, closeDB := newTestRepository(t, now)
	defer closeDB()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "integration_update_statuses"`)).
		WillReturnResult(sqlmock.NewResult(1, 0))
	mock.ExpectQuery(`SELECT .* FROM "integration_update_statuses"`).
		WithArgs("spotify", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "in_progress", "updated_at"}).AddRow("spotify", false, now))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "integration_update_statuses"`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	claimed, err := repo.ClaimUpdate(context.Background(), "spotify")
	if err != nil {
		t.Fatalf("ClaimUpdate: %v", err)
	}
	if !claimed {
		t.Fatal("expected available update to be claimed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestClaimUpdateRejectsInProgressRow(t *testing.T) {
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	repo, mock, closeDB := newTestRepository(t, now)
	defer closeDB()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "integration_update_statuses"`)).
		WillReturnResult(sqlmock.NewResult(1, 0))
	mock.ExpectQuery(`SELECT .* FROM "integration_update_statuses"`).
		WithArgs("spotify", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "in_progress", "updated_at"}).AddRow("spotify", true, now))
	mock.ExpectRollback()

	claimed, err := repo.ClaimUpdate(context.Background(), "spotify")
	if err != nil {
		t.Fatalf("ClaimUpdate: %v", err)
	}
	if claimed {
		t.Fatal("expected in-progress update claim to be rejected")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestDeleteOperationStateRemovesInstallAndUpdateRows(t *testing.T) {
	repo, mock, closeDB := newTestRepository(t, time.Now().UTC())
	defer closeDB()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "integration_operation_statuses"`)).
		WithArgs("spotify").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "integration_update_statuses"`)).
		WithArgs("spotify").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	if err := repo.DeleteOperationState(context.Background(), "spotify"); err != nil {
		t.Fatalf("DeleteOperationState: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
