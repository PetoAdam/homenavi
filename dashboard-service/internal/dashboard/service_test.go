package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type fakeRepo struct {
	defaultDashboard *Dashboard
	userDashboards   map[uuid.UUID]*Dashboard
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{userDashboards: map[uuid.UUID]*Dashboard{}}
}

func (f *fakeRepo) GetDefaultDashboard(context.Context) (*Dashboard, error) {
	return f.defaultDashboard, nil
}
func (f *fakeRepo) GetUserDashboard(_ context.Context, userID uuid.UUID) (*Dashboard, error) {
	return f.userDashboards[userID], nil
}
func (f *fakeRepo) CreateDashboard(_ context.Context, d *Dashboard) error {
	f.userDashboards[*d.OwnerUserID] = d
	return nil
}
func (f *fakeRepo) UpdateUserDashboardDoc(_ context.Context, userID uuid.UUID, expectedVersion int, doc datatypes.JSON) (*Dashboard, error) {
	d := f.userDashboards[userID]
	if d == nil || d.LayoutVersion != expectedVersion {
		return nil, nil
	}
	d.Doc = doc
	d.LayoutVersion++
	return d, nil
}
func (f *fakeRepo) UpsertDefaultDashboard(_ context.Context, title string, doc any) (*Dashboard, error) {
	buf, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	if f.defaultDashboard == nil {
		f.defaultDashboard = &Dashboard{ID: uuid.New(), Scope: "default", Title: title, LayoutEngine: "rgl-v1", LayoutVersion: 1, Doc: datatypes.JSON(buf)}
	} else {
		f.defaultDashboard.Title = title
		f.defaultDashboard.Doc = datatypes.JSON(buf)
		f.defaultDashboard.LayoutVersion++
	}
	return f.defaultDashboard, nil
}

type fakeCatalogSource struct{ widgets []WidgetType }

func (f fakeCatalogSource) Widgets(context.Context, AuthContext) ([]WidgetType, error) {
	return f.widgets, nil
}

func TestCatalogMergesIntegrationWidgets(t *testing.T) {
	svc := NewService(newFakeRepo(), fakeCatalogSource{widgets: []WidgetType{{ID: "integration.weather", DisplayName: "Integration Weather"}, {ID: "homenavi.weather", DisplayName: "Duplicate"}}})
	catalog := svc.Catalog(context.Background(), AuthContext{})
	found := false
	countBase := 0
	for _, item := range catalog {
		if item.ID == "integration.weather" {
			found = true
		}
		if item.ID == "homenavi.weather" {
			countBase++
		}
	}
	if !found {
		t.Fatal("expected merged integration widget")
	}
	if countBase != 1 {
		t.Fatalf("expected deduped base widget, got %d", countBase)
	}
}

func TestGetMyDashboardClonesDefault(t *testing.T) {
	repo := newFakeRepo()
	def, err := repo.UpsertDefaultDashboard(context.Background(), "Home", defaultDashboardDoc())
	if err != nil {
		t.Fatalf("upsert default: %v", err)
	}
	svc := NewService(repo, nil)
	userID := uuid.New()
	dashboard, err := svc.GetMyDashboard(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetMyDashboard: %v", err)
	}
	if dashboard.Scope != "user" || dashboard.OwnerUserID == nil || *dashboard.OwnerUserID != userID {
		t.Fatalf("unexpected dashboard: %#v", dashboard)
	}
	if string(dashboard.Doc) == string(def.Doc) {
		t.Fatal("expected cloned dashboard doc to differ from default doc")
	}
}

func TestPutMyDashboardReturnsConflict(t *testing.T) {
	svc := NewService(newFakeRepo(), nil)
	_, err := svc.PutMyDashboard(context.Background(), uuid.New(), 1, json.RawMessage(`{"items":[]}`))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}
