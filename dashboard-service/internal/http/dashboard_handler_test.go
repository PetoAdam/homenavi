package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PetoAdam/homenavi/dashboard-service/internal/auth"
	"github.com/PetoAdam/homenavi/dashboard-service/internal/dashboard"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

type fakeDashboardService struct {
	catalog   []dashboard.WidgetType
	weather   dashboard.WeatherResponse
	dashboard dashboard.Dashboard
	err       error
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

func TestRouterHealth(t *testing.T) {
	router := NewRouter(NewHandler(&fakeDashboardService{}), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), otel.Tracer("test"), nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}
