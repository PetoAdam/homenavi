package twofactor

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pquerna/otp/totp"
)

func TestEnrollmentHandlersRequireBearerToken(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		body    string
	}{
		{name: "totp setup", handler: (&SetupHandler{}).Handle2FASetup, body: `{}`},
		{name: "totp verify", handler: (&VerifyHandler{}).Handle2FAVerify, body: `{"code":"123456"}`},
		{name: "email request", handler: (&EmailHandler{}).Handle2FAEmailRequest, body: `{}`},
		{name: "email verify", handler: (&EmailHandler{}).Handle2FAEmailVerify, body: `{"code":"123456"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/auth/2fa", strings.NewReader(test.body))
			rec := httptest.NewRecorder()

			test.handler(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 for a request without a bearer token, got %d", rec.Code)
			}
		})
	}
}

func TestAuthenticatedUserIDRejectsEmptyBearerToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/2fa", nil)
	req.Header.Set("Authorization", "Bearer   ")

	_, err := authenticatedUserID(req, nil)
	if err == nil || err.Code != http.StatusUnauthorized {
		t.Fatalf("expected empty bearer token to be unauthorized, got %#v", err)
	}
}

func TestProvisioningQRCodeDataURL(t *testing.T) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "HomeNavi", AccountName: "ada@example.com"})
	if err != nil {
		t.Fatalf("generate TOTP key: %v", err)
	}

	dataURL, err := provisioningQRCodeDataURL(key)
	if err != nil {
		t.Fatalf("generate QR code data URL: %v", err)
	}
	if !strings.HasPrefix(dataURL, "data:image/png;base64,") {
		t.Fatalf("unexpected QR code URL prefix: %q", dataURL)
	}
	pngBytes, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURL, "data:image/png;base64,"))
	if err != nil {
		t.Fatalf("decode QR code data URL: %v", err)
	}
	image, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("decode QR code PNG: %v", err)
	}
	if bounds := image.Bounds(); bounds.Dx() != 256 || bounds.Dy() != 256 {
		t.Fatalf("QR code size = %dx%d, want 256x256", bounds.Dx(), bounds.Dy())
	}
}
