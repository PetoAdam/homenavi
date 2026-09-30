package http

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PetoAdam/homenavi/dashboard-service/internal/auth"
	"github.com/PetoAdam/homenavi/dashboard-service/internal/dashboard"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

type fakeDashboardService struct {
	catalog   []dashboard.WidgetType
	weather   dashboard.WeatherResponse
	dashboard dashboard.Dashboard
	err       error
	deletedID uuid.UUID
}

func (f *fakeDashboardService) Catalog(context.Context, dashboard.AuthContext) []dashboard.WidgetType {
	return f.catalog
}
func (f *fakeDashboardService) Weather(city string) dashboard.WeatherResponse {
	if f.weather.City == "" {
		f.weather = dashboard.WeatherResponse{City: city}
	}
	return f.weather
}
func (f *fakeDashboardService) GetMyDashboard(context.Context, uuid.UUID) (dashboard.Dashboard, error) {
	return f.dashboard, f.err
}
func (f *fakeDashboardService) PutMyDashboard(context.Context, uuid.UUID, int, json.RawMessage) (dashboard.Dashboard, error) {
	return f.dashboard, f.err
}
func (f *fakeDashboardService) DeleteUserDashboard(_ context.Context, userID uuid.UUID) error {
	f.deletedID = userID
	return f.err
}
func (f *fakeDashboardService) GetDefaultDashboard(context.Context) (dashboard.Dashboard, error) {
	return f.dashboard, f.err
}
func (f *fakeDashboardService) PutDefaultDashboard(context.Context, string, json.RawMessage) (dashboard.Dashboard, error) {
	return f.dashboard, f.err
}

func TestHandleCatalog(t *testing.T) {
	h := NewHandler(&fakeDashboardService{catalog: []dashboard.WidgetType{{ID: "homenavi.weather"}}})
	rr := httptest.NewRecorder()
	h.HandleCatalog(rr, httptest.NewRequest(http.MethodGet, "/api/widgets/catalog", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleGetMyDashboardUnauthorized(t *testing.T) {
	h := NewHandler(&fakeDashboardService{})
	rr := httptest.NewRecorder()
	h.HandleGetMyDashboard(rr, httptest.NewRequest(http.MethodGet, "/api/dashboard/me", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestHandlePutMyDashboardConflict(t *testing.T) {
	h := NewHandler(&fakeDashboardService{err: dashboard.ErrConflict})
	req := httptest.NewRequest(http.MethodPut, "/api/dashboard/me", strings.NewReader(`{"layout_version":1,"doc":{}}`))
	req = req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{Role: "resident", Name: "Alice", RegisteredClaims: jwt.RegisteredClaims{Subject: uuid.New().String()}}))
	rr := httptest.NewRecorder()
	h.HandlePutMyDashboard(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rr.Code)
	}
}

func TestHandleDeleteUserDashboard(t *testing.T) {
	service := &fakeDashboardService{}
	h := NewHandler(service)
	userID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/dashboard/users/"+userID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("userID", userID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.HandleDeleteUserDashboard(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
	if service.deletedID != userID {
		t.Fatalf("expected delete for %s, got %s", userID, service.deletedID)
	}
}

func TestRouterDeleteUserDashboardRequiresServiceRole(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	service := &fakeDashboardService{}
	router := NewRouter(NewHandler(service), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), otel.Tracer("test"), &privateKey.PublicKey)
	userID := uuid.New().String()
	req := httptest.NewRequest(http.MethodDelete, "/api/dashboard/users/"+userID, nil)
	req.Header.Set("Authorization", "Bearer "+signedToken(t, privateKey, "admin", "admin-user"))
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestRouterHealth(t *testing.T) {
	router := NewRouter(NewHandler(&fakeDashboardService{}), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), otel.Tracer("test"), nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func signedToken(t *testing.T, privateKey *rsa.PrivateKey, role, subject string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"role": role,
		"sub":  subject,
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(2 * time.Minute).Unix(),
	})
	signed, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}
