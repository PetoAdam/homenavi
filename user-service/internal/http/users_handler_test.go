package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PetoAdam/homenavi/user-service/internal/auth"
	"github.com/PetoAdam/homenavi/user-service/internal/users"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func newHandlerWithService(t *testing.T, repo *fakeRepo) *UsersHandler {
	t.Helper()
	return NewUsersHandler(users.NewService(repo))
}

type fakeRepo struct {
	users map[uuid.UUID]users.User
}

func newFakeRepo() *fakeRepo { return &fakeRepo{users: map[uuid.UUID]users.User{}} }
func (f *fakeRepo) Create(_ context.Context, user *users.User) error {
	f.users[user.ID] = *user
	return nil
}
func (f *fakeRepo) FindByID(_ context.Context, id uuid.UUID) (users.User, error) {
	u, ok := f.users[id]
	if !ok {
		return users.User{}, users.ErrNotFound
	}
	return u, nil
}
func (f *fakeRepo) FindByEmail(_ context.Context, email string) (users.User, error) {
	for _, u := range f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return users.User{}, users.ErrNotFound
}
func (f *fakeRepo) FindByUserName(_ context.Context, userName string) (users.User, error) {
	for _, u := range f.users {
		if u.UserName == userName {
			return u, nil
		}
	}
	return users.User{}, users.ErrNotFound
}
func (f *fakeRepo) FindByGoogleID(_ context.Context, googleID string) (users.User, error) {
	for _, u := range f.users {
		if u.GoogleID != nil && *u.GoogleID == googleID {
			return u, nil
		}
	}
	return users.User{}, users.ErrNotFound
}
func (f *fakeRepo) List(_ context.Context, _ string, page, size int) ([]users.User, int64, error) {
	_ = page
	_ = size
	out := make([]users.User, 0, len(f.users))
	for _, u := range f.users {
		out = append(out, u)
	}
	return out, int64(len(out)), nil
}
func (f *fakeRepo) UpdateFields(_ context.Context, id uuid.UUID, fields map[string]any) error {
	u := f.users[id]
	if v, ok := fields["lockout_enabled"]; ok {
		u.LockoutEnabled = v.(bool)
	}
	f.users[id] = u
	return nil
}
func (f *fakeRepo) Delete(_ context.Context, id uuid.UUID) error { delete(f.users, id); return nil }

func TestHandleCreateSuccess(t *testing.T) {
	h := newHandlerWithService(t, newFakeRepo())
	body := bytes.NewBufferString(`{"user_name":"alice","email":"alice@example.com","password":"secret"}`)
	rr := httptest.NewRecorder()
	h.HandleCreate(rr, httptest.NewRequest(http.MethodPost, "/users", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
}

func TestHandleGetNotFound(t *testing.T) {
	h := newHandlerWithService(t, newFakeRepo())
	rr := httptest.NewRecorder()
	id := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/users/"+id, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	h.HandleGet(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleQueryList(t *testing.T) {
	repo := newFakeRepo()
	id := uuid.New()
	repo.users[id] = users.User{ID: id, UserName: "alice", Email: "alice@example.com", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	h := newHandlerWithService(t, repo)
	req := httptest.NewRequest(http.MethodGet, "/users?page=1&page_size=10", nil)
	req = req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{Role: "resident", Sub: "resident-1"}))
	rr := httptest.NewRecorder()
	h.HandleQuery(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var payload map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["total"].(float64) != 1 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}
