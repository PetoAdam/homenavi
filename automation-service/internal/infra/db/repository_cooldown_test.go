package db

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newCooldownTestRepository(t *testing.T, now time.Time) (*Repository, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	database, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		NowFunc:                func() time.Time { return now },
		SkipDefaultTransaction: true,
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return &Repository{db: database}, mock, func() { _ = sqlDB.Close() }
}

func TestClaimTriggerCooldownCreatesOrRefreshesExpiredClaim(t *testing.T) {
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	repo, mock, closeDB := newCooldownTestRepository(t, now)
	defer closeDB()

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "trigger_cooldowns"`)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	claimed, err := repo.ClaimTriggerCooldown(context.Background(), uuid.New(), "state-trigger", 30*time.Second, now)
	if err != nil {
		t.Fatalf("ClaimTriggerCooldown() error = %v", err)
	}
	if !claimed {
		t.Fatal("expected cooldown claim to succeed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestClaimTriggerCooldownRejectsActiveClaim(t *testing.T) {
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	repo, mock, closeDB := newCooldownTestRepository(t, now)
	defer closeDB()

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "trigger_cooldowns"`)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	claimed, err := repo.ClaimTriggerCooldown(context.Background(), uuid.New(), "state-trigger", 30*time.Second, now)
	if err != nil {
		t.Fatalf("ClaimTriggerCooldown() error = %v", err)
	}
	if claimed {
		t.Fatal("expected active cooldown claim to be rejected")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestClaimTriggerCooldownValidatesInputsAndBypassesZeroCooldown(t *testing.T) {
	repo := &Repository{db: nil}
	if _, err := repo.ClaimTriggerCooldown(context.Background(), uuid.Nil, "trigger", time.Second, time.Now()); err == nil {
		t.Fatal("expected empty workflow id error")
	}
	if _, err := repo.ClaimTriggerCooldown(context.Background(), uuid.New(), "", time.Second, time.Now()); err == nil {
		t.Fatal("expected empty trigger id error")
	}
	claimed, err := repo.ClaimTriggerCooldown(context.Background(), uuid.New(), "trigger", 0, time.Now())
	if err != nil {
		t.Fatalf("ClaimTriggerCooldown() zero cooldown error = %v", err)
	}
	if !claimed {
		t.Fatal("expected zero cooldown to bypass persistence claim")
	}
}

func TestClaimScheduledTriggerCreatesOneOccurrenceClaim(t *testing.T) {
	now := time.Date(2026, time.August, 18, 12, 0, 0, 500, time.UTC)
	repo, mock, closeDB := newCooldownTestRepository(t, now)
	defer closeDB()

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "scheduled_trigger_claims"`)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	claimed, err := repo.ClaimScheduledTrigger(context.Background(), uuid.New(), "schedule-trigger", now)
	if err != nil {
		t.Fatalf("ClaimScheduledTrigger() error = %v", err)
	}
	if !claimed {
		t.Fatal("expected scheduled occurrence claim to succeed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestClaimScheduledTriggerRejectsDuplicateOccurrence(t *testing.T) {
	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.UTC)
	repo, mock, closeDB := newCooldownTestRepository(t, now)
	defer closeDB()

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "scheduled_trigger_claims"`)).
		WillReturnResult(sqlmock.NewResult(1, 0))

	claimed, err := repo.ClaimScheduledTrigger(context.Background(), uuid.New(), "schedule-trigger", now)
	if err != nil {
		t.Fatalf("ClaimScheduledTrigger() error = %v", err)
	}
	if claimed {
		t.Fatal("expected duplicate scheduled occurrence claim to be rejected")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
