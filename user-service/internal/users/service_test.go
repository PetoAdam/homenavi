package users

import (
	"context"
	"errors"
	"testing"

	"github.com/PetoAdam/homenavi/shared/authx"
	"github.com/google/uuid"
)

type fakeRepo struct {
	byEmail    map[string]User
	byUserName map[string]User
	byID       map[uuid.UUID]User
	updated    map[string]any
	recovery   map[uuid.UUID]map[string]bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byEmail: map[string]User{}, byUserName: map[string]User{}, byID: map[uuid.UUID]User{}, recovery: map[uuid.UUID]map[string]bool{}}
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
func (f *fakeRepo) ReplaceRecoveryCodes(_ context.Context, id uuid.UUID, hashes []string) error {
	f.recovery[id] = make(map[string]bool, len(hashes))
	for _, hash := range hashes {
		f.recovery[id][hash] = false
	}
	return nil
}
func (f *fakeRepo) ConsumeRecoveryCode(_ context.Context, id uuid.UUID, hash string) (bool, error) {
	used, ok := f.recovery[id][hash]
	if !ok || used {
		return false, nil
	}
	f.recovery[id][hash] = true
	return true, nil
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

func TestPatchAllowsOnlyNamedAuthServicePrincipal(t *testing.T) {
	repo := newFakeRepo()
	id := uuid.New()
	repo.byID[id] = User{ID: id, Role: "user"}
	svc := NewService(repo)

	err := svc.Patch(context.Background(), Actor{Subject: authx.ServicePrincipalAuth, Role: authx.RoleService}, id.String(), map[string]any{"two_factor_enabled": true})
	if err != nil {
		t.Fatalf("expected named auth service principal to patch user, got %v", err)
	}
	if !repo.updated["two_factor_enabled"].(bool) {
		t.Fatal("expected two-factor update")
	}

	err = svc.Patch(context.Background(), Actor{Subject: "another-service", Role: authx.RoleService}, id.String(), map[string]any{"two_factor_enabled": true})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected unknown service principal to be forbidden, got %v", err)
	}
}

func TestRecoveryCodesRequireNamedAuthServiceAndAreOneTime(t *testing.T) {
	repo := newFakeRepo()
	id := uuid.New()
	svc := NewService(repo)
	actor := Actor{Subject: authx.ServicePrincipalAuth, Role: authx.RoleService}
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	if err := svc.ReplaceRecoveryCodes(context.Background(), actor, id.String(), []string{hash}); err != nil {
		t.Fatalf("store recovery code: %v", err)
	}
	consumed, err := svc.ConsumeRecoveryCode(context.Background(), actor, id.String(), hash)
	if err != nil || !consumed {
		t.Fatalf("first consumption = %t, %v", consumed, err)
	}
	consumed, err = svc.ConsumeRecoveryCode(context.Background(), actor, id.String(), hash)
	if err != nil || consumed {
		t.Fatalf("second consumption = %t, %v", consumed, err)
	}
	if err := svc.ReplaceRecoveryCodes(context.Background(), Actor{Subject: "other", Role: authx.RoleService}, id.String(), []string{hash}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected other service to be forbidden, got %v", err)
	}
}
