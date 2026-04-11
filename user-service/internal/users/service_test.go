package users

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type fakeRepo struct {
	byEmail    map[string]User
	byUserName map[string]User
	byID       map[uuid.UUID]User
	updated    map[string]any
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byEmail: map[string]User{}, byUserName: map[string]User{}, byID: map[uuid.UUID]User{}}
}

func (f *fakeRepo) Create(_ context.Context, user *User) error {
	f.byEmail[user.Email] = *user
	f.byUserName[user.UserName] = *user
	f.byID[user.ID] = *user
	return nil
}
func (f *fakeRepo) FindByID(_ context.Context, id uuid.UUID) (User, error) {
	u, ok := f.byID[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}
func (f *fakeRepo) FindByEmail(_ context.Context, email string) (User, error) {
	u, ok := f.byEmail[email]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}
func (f *fakeRepo) FindByUserName(_ context.Context, userName string) (User, error) {
	u, ok := f.byUserName[userName]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}
func (f *fakeRepo) FindByGoogleID(_ context.Context, googleID string) (User, error) {
	for _, u := range f.byID {
		if u.GoogleID != nil && *u.GoogleID == googleID {
			return u, nil
		}
	}
	return User{}, ErrNotFound
}
func (f *fakeRepo) List(_ context.Context, _ string, page, size int) ([]User, int64, error) {
	_ = page
	_ = size
	out := make([]User, 0, len(f.byID))
	for _, u := range f.byID {
		out = append(out, u)
	}
	return out, int64(len(out)), nil
}
func (f *fakeRepo) UpdateFields(_ context.Context, _ uuid.UUID, fields map[string]any) error {
	f.updated = fields
	return nil
}
func (f *fakeRepo) Delete(_ context.Context, _ uuid.UUID) error { return nil }

func TestCreateRequiresPasswordOrGoogleID(t *testing.T) {
	svc := NewService(newFakeRepo())
	_, err := svc.Create(context.Background(), CreateInput{UserName: "alice", Email: "a@example.com"})
	if !errors.Is(err, ErrPasswordOrGoogleIDRequired) {
		t.Fatalf("expected ErrPasswordOrGoogleIDRequired, got %v", err)
	}
}

func TestPatchRejectsRoleChangeForUser(t *testing.T) {
	repo := newFakeRepo()
	id := uuid.New()
	repo.byID[id] = User{ID: id, Role: "user"}
	svc := NewService(repo)
	err := svc.Patch(context.Background(), Actor{Subject: id.String(), Role: "user"}, id.String(), map[string]any{"role": "admin"})
	if !errors.Is(err, ErrCannotChangeRole) {
		t.Fatalf("expected ErrCannotChangeRole, got %v", err)
	}
}
