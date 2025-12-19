package oauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"auth-service/internal/models/entities"
	"auth-service/internal/services"
)

type fakeGoogleAuthService struct {
	validateCalled     bool
	exchangeCalled     bool
	issueAccessCalled  bool
	issueRefreshCalled bool
}

func (f *fakeGoogleAuthService) ValidateOAuthState(state string) error {
	f.validateCalled = true
	return nil
}

func (f *fakeGoogleAuthService) ExchangeGoogleOAuthCode(code, redirectURI string) (*services.GoogleUserInfo, error) {
	f.exchangeCalled = true
	return &services.GoogleUserInfo{ID: "google-123", Email: "locked@example.com"}, nil
}

func (f *fakeGoogleAuthService) IssueAccessToken(user *entities.User) (string, error) {
	f.issueAccessCalled = true
	return "access", nil
}

func (f *fakeGoogleAuthService) IssueRefreshToken(userID string) (string, error) {
	f.issueRefreshCalled = true
	return "refresh", nil
}

type fakeGoogleUserService struct {
	userByEmail *entities.User
}

func (f *fakeGoogleUserService) GetUserByEmail(email string) (*entities.User, error) {
	return f.userByEmail, nil
}

func (f *fakeGoogleUserService) GetUserByGoogleID(googleID string) (*entities.User, error) {
	return nil, http.ErrNoLocation
}

func (f *fakeGoogleUserService) LinkGoogleID(userID, googleID string) error {
	return nil
}

func (f *fakeGoogleUserService) CreateGoogleUser(userInfo *services.GoogleUserInfo) (*entities.User, error) {
	return nil, http.ErrNoLocation
}

func TestHandleOAuthGoogleCallback_BlocksLockedUser(t *testing.T) {
	authSvc := &fakeGoogleAuthService{}
	userSvc := &fakeGoogleUserService{userByEmail: &entities.User{ID: "u-1", Email: "locked@example.com", LockoutEnabled: true}}

	h := NewGoogleHandler(authSvc, userSvc)

	req := httptest.NewRequest(http.MethodGet, "/oauth/google/callback?code=abc&state=xyz", nil)
	rw := httptest.NewRecorder()
	h.HandleOAuthGoogleCallback(rw, req)

	if rw.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected %d got %d", http.StatusTemporaryRedirect, rw.Code)
	}
	loc := rw.Header().Get("Location")
	if loc != "/?error=account_locked&reason=admin_lock" {
		t.Fatalf("unexpected redirect location: %q", loc)
	}
	if !authSvc.validateCalled || !authSvc.exchangeCalled {
		t.Fatalf("expected oauth state validate + exchange to be called")
	}
	if authSvc.issueAccessCalled || authSvc.issueRefreshCalled {
		t.Fatalf("did not expect tokens to be issued for locked user")
	}
}
