package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestRouterHealth(t *testing.T) {
	noop := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
	router := NewRouter(Routes{
		HandleSignup:               noop,
		HandleLoginStart:           noop,
		HandleLoginFinish:          noop,
		HandleRefresh:              noop,
		HandleLogout:               noop,
		HandlePasswordResetRequest: noop,
		HandlePasswordResetConfirm: noop,
		HandlePasswordChange:       noop,
		HandleEmailVerifyRequest:   noop,
		HandleEmailVerifyConfirm:   noop,
		HandleTwoFactorSetup:       noop,
		HandleTwoFactorVerify:      noop,
		HandleTwoFactorEmailReq:    noop,
		HandleTwoFactorEmailVerify: noop,
		HandleMe:                   noop,
		HandleDeleteUser:           noop,
		HandleListUsers:            noop,
		HandleGetUser:              noop,
		HandlePatchUser:            noop,
		HandleLockoutUser:          noop,
		HandleGenerateAvatar:       noop,
		HandleCreateUploadURL:      noop,
		HandleCompleteUpload:       noop,
		HandleUploadProfilePicture: noop,
		HandleGoogleOAuthLogin:     noop,
		HandleGoogleOAuthCallback:  noop,
	}, http.HandlerFunc(noop), otel.Tracer("test"))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "OK" {
		t.Fatalf("expected OK body, got %q", rr.Body.String())
	}
}
