package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	authdomain "github.com/PetoAdam/homenavi/auth-service/internal/auth"
	clientsinfra "github.com/PetoAdam/homenavi/auth-service/internal/infra/clients"
)

type fakeDemoBootstrapAuthService struct {
	issuedFor *clientsinfra.User
}

func (f *fakeDemoBootstrapAuthService) IssueTokenPair(user *clientsinfra.User) (*authdomain.TokenPair, error) {
	f.issuedFor = user
	return &authdomain.TokenPair{AccessToken: "access-token", RefreshToken: "refresh-token"}, nil
}

type fakeDemoBootstrapUserService struct {
	createdUser *clientsinfra.User
	userName    string
	email       string
	password    string
	firstName   string
	lastName    string
}

func (f *fakeDemoBootstrapUserService) CreateResidentDemoUser(userName, email, password, firstName, lastName string) (*clientsinfra.User, error) {
	f.userName = userName
	f.email = email
	f.password = password
	f.firstName = firstName
	f.lastName = lastName
	return f.createdUser, nil
}

func TestHandleDemoBootstrapIssuesNormalTokensForResidentUser(t *testing.T) {
	authSvc := &fakeDemoBootstrapAuthService{}
	userSvc := &fakeDemoBootstrapUserService{createdUser: &clientsinfra.User{ID: "demo-user", Role: "resident", FirstName: "Demo", LastName: "Visitor"}}
	h := NewDemoBootstrapHandler(authSvc, userSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/demo/bootstrap", nil)
	rr := httptest.NewRecorder()
	h.HandleDemoBootstrap(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if authSvc.issuedFor == nil || authSvc.issuedFor.Role != "resident" {
		t.Fatalf("expected resident user token issuance, got %#v", authSvc.issuedFor)
	}
	if userSvc.userName == "" || userSvc.email == "" || userSvc.password == "" {
		t.Fatalf("expected generated demo credentials to be passed")
	}
	var payload map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["access_token"] != "access-token" || payload["refresh_token"] != "refresh-token" {
		t.Fatalf("unexpected token payload: %#v", payload)
	}
}