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

func TestDefaultDashboardDocUsesCurrentColumnModeShapes(t *testing.T) {
	doc := defaultDashboardDoc()

	if got := len(doc.Items); got != 4 {
		t.Fatalf("expected 4 default dashboard items, got %d", got)
	}

	assertMaxRight := func(mode string, want int) {
		t.Helper()
		items := doc.LayoutsByCols[mode]
		if len(items) == 0 {
			t.Fatalf("expected layout for column mode %q", mode)
		}
		maxRight := 0
		for _, item := range items {
			x, _ := item["x"].(int)
			w, _ := item["w"].(int)
			if x+w > maxRight {
				maxRight = x + w
			}
		}
		if maxRight != want {
			t.Fatalf("expected %s maxRight %d, got %d", mode, want, maxRight)
		}
	}

	assertMaxRight("4", 4)
	assertMaxRight("3", 3)
	assertMaxRight("2", 2)
	assertMaxRight("1", 1)
}

func TestCloneDashboardDocNormalizesLegacyBreakpointLayouts(t *testing.T) {
	raw := datatypes.JSON([]byte(`{
		"items":[{"instance_id":"legacy-weather","widget_type":"homenavi.weather","enabled":true,"settings":{}}],
		"layouts":{"lg":[{"i":"legacy-weather","x":0,"y":0,"w":1,"h":8}],"sm":[{"i":"legacy-weather","x":0,"y":0,"w":1,"h":8}]}
	}`))

	cloned, err := cloneDashboardDoc(raw)
	if err != nil {
		t.Fatalf("cloneDashboardDoc: %v", err)
	}

	var doc DashboardDoc
	if err := json.Unmarshal(cloned, &doc); err != nil {
		t.Fatalf("unmarshal cloned doc: %v", err)
	}

	if len(doc.LayoutsByCols["4"]) != 1 {
		t.Fatalf("expected cloned 4-column layout, got %d items", len(doc.LayoutsByCols["4"]))
	}
	if len(doc.LayoutsByCols["3"]) != 1 {
		t.Fatalf("expected cloned 3-column fallback layout, got %d items", len(doc.LayoutsByCols["3"]))
	}
	if len(doc.LayoutsByCols["2"]) != 1 {
		t.Fatalf("expected cloned 2-column layout, got %d items", len(doc.LayoutsByCols["2"]))
	}
	if len(doc.Items) != 1 {
		t.Fatalf("expected cloned items, got %d", len(doc.Items))
	}
}
