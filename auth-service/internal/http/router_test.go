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

func TestRouterJWKS(t *testing.T) {
	router := NewRouter(Routes{
		HandleJWKS: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"keys":[]}`))
		},
	}, http.NotFoundHandler(), otel.Tracer("test"))
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/jwks.json", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != `{"keys":[]}` {
		t.Fatalf("unexpected JWKS body %q", rr.Body.String())
	}
}

func TestRouterBrowserSessionAuthorization(t *testing.T) {
	for _, test := range []struct {
		name          string
		method        string
		path          string
		authorization string
		want          string
	}{
		{name: "authorization browser GET", method: http.MethodGet, path: "/api/auth/oauth/authorize"},
		{name: "authorization browser POST", method: http.MethodPost, path: "/api/auth/oauth/authorize"},
		{name: "explicit authorization", method: http.MethodGet, path: "/api/auth/oauth/authorize", authorization: "Bearer explicit-token", want: "Bearer explicit-token"},
		{name: "other authenticated route", method: http.MethodGet, path: "/api/auth/me", want: "Bearer session-token"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var authorization string
			handler := func(w http.ResponseWriter, r *http.Request) {
				authorization = r.Header.Get("Authorization")
				if _, err := r.Cookie("auth_token"); err != nil {
					t.Errorf("browser session cookie missing: %v", err)
				}
				w.WriteHeader(http.StatusNoContent)
			}
			router := NewRouter(Routes{HandleOAuthAuthorize: handler, HandleMe: handler}, http.NotFoundHandler(), otel.Tracer("test"))
			req := httptest.NewRequest(test.method, test.path, nil)
			req.AddCookie(&http.Cookie{Name: "auth_token", Value: "session-token"})
			if test.authorization != "" {
				req.Header.Set("Authorization", test.authorization)
			}
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != http.StatusNoContent || authorization != test.want {
				t.Fatalf("status=%d authorization=%q, want %d and %q", rr.Code, authorization, http.StatusNoContent, test.want)
			}
		})
	}
}
