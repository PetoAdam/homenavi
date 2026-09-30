package demojanitor

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strconv"
	"testing"
	"time"

	cacheinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/cache"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
)

type fakeActivityStore struct {
	values map[string]string
	err    error
}

func (f fakeActivityStore) Get(_ context.Context, key string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	value, ok := f.values[key]
	if !ok {
		return "", cacheinfra.ErrNotFound
	}
	return value, nil
}

type fakeUserDirectory struct {
	pages         [][]clientsinfra.User
	deletedUserID []string
}

func (f *fakeUserDirectory) ListUsersInternal(values url.Values) ([]clientsinfra.User, map[string]interface{}, error) {
	page, err := strconv.Atoi(values.Get("page"))
	if err != nil || page <= 0 {
		page = 1
	}
	if page > len(f.pages) {
		return nil, map[string]interface{}{"total_pages": len(f.pages)}, nil
	}
	return f.pages[page-1], map[string]interface{}{"total_pages": len(f.pages)}, nil
}

func (f *fakeUserDirectory) DeleteUserInternal(userID string) error {
	f.deletedUserID = append(f.deletedUserID, userID)
	return nil
}

type fakeDashboardCleaner struct {
	deletedUserID []string
	errForUserID  map[string]error
}

func (f *fakeDashboardCleaner) DeleteUserDashboard(userID string) error {
	if err := f.errForUserID[userID]; err != nil {
		return err
	}
	f.deletedUserID = append(f.deletedUserID, userID)
	return nil
}

func TestBuildPlanDeletesStaleBeforeCap(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	candidates := []candidate{
		{user: clientsinfra.User{ID: "stale-missing"}, stale: true},
		{user: clientsinfra.User{ID: "stale-old"}, lastActive: now.Add(-20 * time.Minute)},
		{user: clientsinfra.User{ID: "recent-1"}, lastActive: now.Add(-5 * time.Minute)},
		{user: clientsinfra.User{ID: "recent-2"}, lastActive: now.Add(-3 * time.Minute)},
		{user: clientsinfra.User{ID: "recent-3"}, lastActive: now.Add(-1 * time.Minute)},
	}

	plan := buildPlan(now, candidates, 15*time.Minute, 2)

	if len(plan.stale) != 2 {
		t.Fatalf("expected 2 stale users, got %d", len(plan.stale))
	}
	if len(plan.capVictims) != 1 || plan.capVictims[0].ID != "recent-1" {
		t.Fatalf("expected oldest active user to be cap victim, got %#v", plan.capVictims)
	}
}

func TestCleanupDeletesDashboardBeforeUser(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	user := clientsinfra.User{ID: "u-1", UserName: "demo_one", Email: "demo+one@example.com"}
	activity := fakeActivityStore{values: map[string]string{activityKeyPrefix + user.ID: strconv.FormatInt(now.Add(-20*time.Minute).Unix(), 10)}}
	users := &fakeUserDirectory{pages: [][]clientsinfra.User{{user}}}
	dashboards := &fakeDashboardCleaner{errForUserID: map[string]error{}}
	janitor := New(Config{ActivityTTL: 15 * time.Minute, HardCap: 10, PageSize: 100}, activity, users, dashboards, slog.Default())

	if err := janitor.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if len(dashboards.deletedUserID) != 1 || dashboards.deletedUserID[0] != user.ID {
		t.Fatalf("expected dashboard cleanup first, got %#v", dashboards.deletedUserID)
	}
	if len(users.deletedUserID) != 1 || users.deletedUserID[0] != user.ID {
		t.Fatalf("expected user delete, got %#v", users.deletedUserID)
	}
}

func TestCleanupSkipsUserDeleteWhenDashboardCleanupFails(t *testing.T) {
	user := clientsinfra.User{ID: "u-2", UserName: "demo_two", Email: "demo+two@example.com"}
	activity := fakeActivityStore{}
	users := &fakeUserDirectory{pages: [][]clientsinfra.User{{user}}}
	dashboards := &fakeDashboardCleaner{errForUserID: map[string]error{user.ID: errors.New("boom")}}
	janitor := New(Config{}, activity, users, dashboards, slog.Default())

	if err := janitor.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if len(users.deletedUserID) != 0 {
		t.Fatalf("expected user delete to be skipped, got %#v", users.deletedUserID)
	}
}
