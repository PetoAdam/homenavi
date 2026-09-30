package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/PetoAdam/homenavi/shared/mockdemo"
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
func (f *fakeRepo) DeleteUserDashboard(_ context.Context, userID uuid.UUID) error {
	delete(f.userDashboards, userID)
	return nil
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

func TestDeleteUserDashboardRemovesOwnedDashboard(t *testing.T) {
	repo := newFakeRepo()
	userID := uuid.New()
	repo.userDashboards[userID] = &Dashboard{ID: uuid.New(), Scope: "user", OwnerUserID: &userID}
	svc := NewService(repo, nil)

	if err := svc.DeleteUserDashboard(context.Background(), userID); err != nil {
		t.Fatalf("DeleteUserDashboard: %v", err)
	}
	if repo.userDashboards[userID] != nil {
		t.Fatal("expected owned dashboard to be deleted")
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

func TestGetDefaultDashboardUsesDemoTemplateWhenEnabled(t *testing.T) {
	t.Setenv("DEMO_MODE", "true")

	repo := newFakeRepo()
	svc := NewService(repo, nil)

	dashboard, err := svc.GetDefaultDashboard(context.Background())
	if err != nil {
		t.Fatalf("GetDefaultDashboard: %v", err)
	}

	var doc DashboardDoc
	if err := json.Unmarshal(dashboard.Doc, &doc); err != nil {
		t.Fatalf("unmarshal doc: %v", err)
	}

	if got := len(doc.Items); got != 14 {
		t.Fatalf("expected 14 demo dashboard items, got %d", got)
	}

	settingsByType := map[string][]map[string]any{}
	instanceIDsByType := map[string][]string{}
	for _, item := range doc.Items {
		widgetType, _ := item["widget_type"].(string)
		settings, _ := item["settings"].(map[string]any)
		instanceID, _ := item["instance_id"].(string)
		settingsByType[widgetType] = append(settingsByType[widgetType], settings)
		instanceIDsByType[widgetType] = append(instanceIDsByType[widgetType], instanceID)
	}

	if _, ok := settingsByType["homenavi.demo.overview"]; !ok {
		t.Fatal("expected demo overview widget in demo dashboard")
	}
	weatherSettings, ok := settingsByType["homenavi.weather"]
	if !ok || len(weatherSettings) != 1 {
		t.Fatal("expected weather widget in demo dashboard")
	}
	if city, _ := weatherSettings[0]["city"].(string); city != "Budapest" {
		t.Fatalf("expected demo weather city Budapest, got %q", city)
	}
	deviceSettings, ok := settingsByType["homenavi.device"]
	if !ok || len(deviceSettings) != 4 {
		t.Fatalf("expected 4 device widgets in demo dashboard, got %#v", settingsByType["homenavi.device"])
	}
	seenDemoDevices := map[string]bool{}
	for _, settings := range deviceSettings {
		deviceID, _ := settings["hdp_device_id"].(string)
		seenDemoDevices[deviceID] = true
	}
	if !seenDemoDevices["mock/air-purifier"] {
		t.Fatal("expected air purifier device widget in demo dashboard")
	}
	if !seenDemoDevices["mock/bedroom-blind"] {
		t.Fatal("expected bedroom blind device widget in demo dashboard")
	}
	if !seenDemoDevices["mock/coffee-maker"] {
		t.Fatal("expected coffee maker device widget in demo dashboard")
	}
	if !seenDemoDevices["mock/sofa-lamp"] {
		t.Fatal("expected sofa lamp device widget in demo dashboard")
	}
	multiSettings, ok := settingsByType["homenavi.device.multi"]
	if !ok || len(multiSettings) != 1 {
		t.Fatal("expected quick controls widget in demo dashboard")
	}
	groupSettings, ok := settingsByType["homenavi.group.controls"]
	if !ok || len(groupSettings) != 1 {
		t.Fatal("expected group controls widget in demo dashboard")
	}
	if groupID, _ := groupSettings[0]["group_id"].(string); groupID != demoEveningLightsGroupID {
		t.Fatalf("expected evening lights group %q, got %q", demoEveningLightsGroupID, groupID)
	}
	graphSettings, ok := settingsByType["homenavi.device.graph"]
	if !ok || len(graphSettings) != 1 {
		t.Fatal("expected device graph widget in demo dashboard")
	}
	if metricKey, _ := graphSettings[0]["metric_key"].(string); metricKey != "air_quality" {
		t.Fatalf("expected demo graph metric air_quality, got %q", metricKey)
	}
	automationSettings, ok := settingsByType["homenavi.automation.manual_trigger"]
	if !ok || len(automationSettings) != 3 {
		t.Fatalf("expected 3 automation widgets in demo dashboard, got %#v", automationSettings)
	}
	seenWorkflows := map[string]bool{}
	for _, settings := range automationSettings {
		workflowID, _ := settings["workflow_id"].(string)
		seenWorkflows[workflowID] = true
	}
	for _, workflowID := range []string{demoGoodMorningWorkflowID, demoMovieNightWorkflowID, demoLeaveHomeWorkflowID} {
		if !seenWorkflows[workflowID] {
			t.Fatalf("expected demo workflow %q in dashboard", workflowID)
		}
	}
	if _, ok := settingsByType["homenavi.map"]; !ok {
		t.Fatal("expected map widget in demo dashboard")
	}
	if spotifySettings, ok := settingsByType["integration.spotify.player"]; !ok || len(spotifySettings) != 1 {
		t.Fatalf("expected spotify widget in demo dashboard, got %#v", spotifySettings)
	}
	mapInstanceID := instanceIDsByType["homenavi.map"][0]
	overviewInstanceID := instanceIDsByType["homenavi.demo.overview"][0]
	weatherInstanceID := instanceIDsByType["homenavi.weather"][0]
	groupInstanceID := instanceIDsByType["homenavi.group.controls"][0]
	spotifyInstanceID := instanceIDsByType["integration.spotify.player"][0]
	quickLightsInstanceID := instanceIDsByType["homenavi.device.multi"][0]
	movieNightInstanceID := ""
	goodMorningInstanceID := ""
	leaveHomeInstanceID := ""
	coffeeInstanceID := ""
	for index, settings := range automationSettings {
		title, _ := settings["title"].(string)
		if title == demoMovieNightWidgetTitle {
			movieNightInstanceID = instanceIDsByType["homenavi.automation.manual_trigger"][index]
		}
		if title == demoGoodMorningWidgetTitle {
			goodMorningInstanceID = instanceIDsByType["homenavi.automation.manual_trigger"][index]
		}
		if title == demoLeaveHomeWidgetTitle {
			leaveHomeInstanceID = instanceIDsByType["homenavi.automation.manual_trigger"][index]
		}
	}
	for index, settings := range deviceSettings {
		deviceID, _ := settings["hdp_device_id"].(string)
		if deviceID == mockdemo.CoffeeMakerHDPDeviceID {
			coffeeInstanceID = instanceIDsByType["homenavi.device"][index]
			break
		}
	}
	if movieNightInstanceID == "" {
		t.Fatal("expected Movie Night automation widget in demo dashboard")
	}
	if goodMorningInstanceID == "" || leaveHomeInstanceID == "" {
		t.Fatal("expected Good Morning and Leave Home automation widgets in demo dashboard")
	}
	if coffeeInstanceID == "" {
		t.Fatal("expected Coffee Maker widget in demo dashboard")
	}
	deviceInstanceIDs := instanceIDsByType["homenavi.device"]
	toInt := func(value any) int {
		switch n := value.(type) {
		case int:
			return n
		case int32:
			return int(n)
		case int64:
			return int(n)
		case float32:
			return int(n)
		case float64:
			return int(n)
		default:
			return 0
		}
	}

	for _, mode := range []string{"4", "3", "2", "1"} {
		items := doc.LayoutsByCols[mode]
		if len(items) != 14 {
			t.Fatalf("expected 14 layout items for mode %s, got %d", mode, len(items))
		}

		mapY := -1
		mapX := -1
		overviewX := -1
		weatherX := -1
		quickLightsX := -1
		goodMorningX := -1
		leaveHomeX := -1
		coffeeY := -1
		groupY := -1
		spotifyY := -1
		spotifyH := 0
		movieNightY := -1
		for _, item := range items {
			instanceID, _ := item["i"].(string)
			width := toInt(item["w"])
			x := toInt(item["x"])
			y := toInt(item["y"])
			height := toInt(item["h"])
			if instanceID == mapInstanceID {
				mapX = x
				mapY = y
			}
			if instanceID == overviewInstanceID {
				overviewX = x
			}
			if instanceID == weatherInstanceID {
				weatherX = x
			}
			if instanceID == quickLightsInstanceID {
				quickLightsX = x
			}
			if instanceID == goodMorningInstanceID {
				goodMorningX = x
			}
			if instanceID == leaveHomeInstanceID {
				leaveHomeX = x
			}
			if instanceID == spotifyInstanceID {
				spotifyY = y
				spotifyH = height
			}
			if instanceID == movieNightInstanceID {
				movieNightY = y
			}
			if instanceID == coffeeInstanceID {
				coffeeY = y
			}
			if instanceID == groupInstanceID {
				groupY = y
				if width != 1 {
					t.Fatalf("expected group controls to stay single-column in mode %s, got width %d", mode, width)
				}
				if height != 6 {
					t.Fatalf("expected group controls to be 6 rows tall in mode %s, got height %d", mode, height)
				}
			}
			for _, deviceInstanceID := range deviceInstanceIDs {
				if instanceID == deviceInstanceID && width != 1 {
					t.Fatalf("expected device widget %s to stay single-column in mode %s, got width %d", deviceInstanceID, mode, width)
				}
			}
		}
		if mapY < 0 {
			t.Fatalf("expected map widget in layout for mode %s", mode)
		}
		if spotifyY < 0 {
			t.Fatalf("expected spotify widget in layout for mode %s", mode)
		}
		if movieNightY < 0 {
			t.Fatalf("expected Movie Night widget in layout for mode %s", mode)
		}
		if spotifyY > movieNightY {
			t.Fatalf("expected spotify widget above Movie Night in mode %s, got spotify y=%d movieNight y=%d", mode, spotifyY, movieNightY)
		}
		if spotifyH != 4 {
			t.Fatalf("expected spotify widget height 4 in mode %s, got h=%d", mode, spotifyH)
		}
		if coffeeY < 0 || groupY < 0 {
			t.Fatalf("expected coffee maker and group controls widgets in layout for mode %s", mode)
		}
		if mode != "1" && coffeeY > groupY {
			t.Fatalf("expected coffee maker to be placed no lower than group controls in mode %s, got coffee y=%d group y=%d", mode, coffeeY, groupY)
		}
		if mode == "4" {
			if mapX != 2 || overviewX != 2 {
				t.Fatalf("expected overview and map on the right in mode 4, got overview x=%d map x=%d", overviewX, mapX)
			}
			if weatherX != 0 || quickLightsX != 0 || goodMorningX != 1 || leaveHomeX != 1 {
				t.Fatalf("expected weather/quick lights on the left and automation tiles in the left stack for mode 4, got weather x=%d quicklights x=%d goodmorning x=%d leavehome x=%d", weatherX, quickLightsX, goodMorningX, leaveHomeX)
			}
		}
		if mode == "3" || mode == "2" {
			if mapX != 1 || overviewX != 1 {
				t.Fatalf("expected overview and map in the right column block for mode %s, got overview x=%d map x=%d", mode, overviewX, mapX)
			}
			if weatherX != 0 || quickLightsX != 0 || goodMorningX != 0 || leaveHomeX != 0 {
				t.Fatalf("expected weather, quick lights, and automation stack on the left in mode %s, got weather x=%d quicklights x=%d goodmorning x=%d leavehome x=%d", mode, weatherX, quickLightsX, goodMorningX, leaveHomeX)
			}
		}
	}
}

func TestGetDefaultDashboardRefreshesPersistedDemoDefaultWhenEnabled(t *testing.T) {
	t.Setenv("DEMO_MODE", "true")

	repo := newFakeRepo()
	_, err := repo.UpsertDefaultDashboard(context.Background(), "Home", defaultDashboardDoc())
	if err != nil {
		t.Fatalf("seed stale default: %v", err)
	}
	svc := NewService(repo, nil)

	dashboard, err := svc.GetDefaultDashboard(context.Background())
	if err != nil {
		t.Fatalf("GetDefaultDashboard: %v", err)
	}

	var doc DashboardDoc
	if err := json.Unmarshal(dashboard.Doc, &doc); err != nil {
		t.Fatalf("unmarshal doc: %v", err)
	}

	if got := len(doc.Items); got != 14 {
		t.Fatalf("expected refreshed demo dashboard items, got %d", got)
	}
	if repo.defaultDashboard == nil || repo.defaultDashboard.LayoutVersion < 2 {
		t.Fatalf("expected persisted default dashboard to be refreshed, got %#v", repo.defaultDashboard)
	}
	seen := map[string]bool{}
	for _, item := range doc.Items {
		widgetType, _ := item["widget_type"].(string)
		seen[widgetType] = true
	}
	if !seen["homenavi.demo.overview"] || !seen["homenavi.automation.manual_trigger"] || !seen["homenavi.device.multi"] || !seen["homenavi.group.controls"] || !seen["integration.spotify.player"] {
		t.Fatalf("expected refreshed demo widgets, got %#v", seen)
	}
}

func TestGetMyDashboardRefreshesPersistedDemoUserDashboardWhenEnabled(t *testing.T) {
	t.Setenv("DEMO_MODE", "true")

	repo := newFakeRepo()
	_, err := repo.UpsertDefaultDashboard(context.Background(), "Home", defaultDashboardDoc())
	if err != nil {
		t.Fatalf("seed stale default: %v", err)
	}
	userID := uuid.New()
	repo.userDashboards[userID] = &Dashboard{
		ID:            uuid.New(),
		Scope:         "user",
		OwnerUserID:   &userID,
		Title:         "Home",
		LayoutEngine:  "rgl-v1",
		LayoutVersion: 4,
		Doc:           datatypes.JSON([]byte(`{"items":[{"instance_id":"legacy-map","widget_type":"homenavi.map","enabled":true,"settings":{}}],"layouts_by_cols":{"4":[{"i":"legacy-map","x":0,"y":0,"w":1,"h":8}],"3":[{"i":"legacy-map","x":0,"y":0,"w":1,"h":8}],"2":[{"i":"legacy-map","x":0,"y":0,"w":1,"h":8}],"1":[{"i":"legacy-map","x":0,"y":0,"w":1,"h":8}]}}`)),
	}
	svc := NewService(repo, nil)

	dashboard, err := svc.GetMyDashboard(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetMyDashboard: %v", err)
	}

	var doc DashboardDoc
	if err := json.Unmarshal(dashboard.Doc, &doc); err != nil {
		t.Fatalf("unmarshal doc: %v", err)
	}

	if got := len(doc.Items); got != 14 {
		t.Fatalf("expected refreshed demo user dashboard items, got %d", got)
	}
	if dashboard.LayoutVersion != 5 {
		t.Fatalf("expected demo user dashboard version 5 after refresh, got %d", dashboard.LayoutVersion)
	}
	seen := map[string]bool{}
	for _, item := range doc.Items {
		widgetType, _ := item["widget_type"].(string)
		seen[widgetType] = true
	}
	for _, widgetType := range []string{"homenavi.demo.overview", "homenavi.weather", "homenavi.device.multi", "homenavi.group.controls", "homenavi.device.graph", "homenavi.map", "integration.spotify.player"} {
		if !seen[widgetType] {
			t.Fatalf("expected refreshed user demo widget %s", widgetType)
		}
	}
}

func TestGetDefaultDashboardUsesNormalTemplateWhenDemoDisabled(t *testing.T) {
	t.Setenv("DEMO_MODE", "false")

	repo := newFakeRepo()
	svc := NewService(repo, nil)

	dashboard, err := svc.GetDefaultDashboard(context.Background())
	if err != nil {
		t.Fatalf("GetDefaultDashboard: %v", err)
	}

	var doc DashboardDoc
	if err := json.Unmarshal(dashboard.Doc, &doc); err != nil {
		t.Fatalf("unmarshal doc: %v", err)
	}

	if got := len(doc.Items); got != 4 {
		t.Fatalf("expected 4 normal dashboard items, got %d", got)
	}

	seen := map[string]bool{}
	for _, item := range doc.Items {
		widgetType, _ := item["widget_type"].(string)
		seen[widgetType] = true
	}

	if seen["homenavi.demo.overview"] {
		t.Fatal("did not expect demo overview widget in normal dashboard")
	}
	for _, widgetType := range []string{"homenavi.weather", "homenavi.device", "homenavi.automation.manual_trigger", "homenavi.map"} {
		if !seen[widgetType] {
			t.Fatalf("expected normal widget %s", widgetType)
		}
	}

	for _, mode := range []string{"4", "3", "2", "1"} {
		items := doc.LayoutsByCols[mode]
		if len(items) != 4 {
			t.Fatalf("expected 4 layout items for mode %s, got %d", mode, len(items))
		}
	}
}

func TestCatalogIncludesDemoOverviewWidget(t *testing.T) {
	svc := NewService(newFakeRepo(), nil)
	catalog := svc.Catalog(context.Background(), AuthContext{})

	for _, item := range catalog {
		if item.ID != "homenavi.demo.overview" {
			continue
		}
		if item.Source != "first_party" {
			t.Fatalf("expected first_party source, got %q", item.Source)
		}
		if item.DisplayName == "" {
			t.Fatal("expected display name for demo overview widget")
		}
		return
	}

	t.Fatalf("expected %q in catalog", "homenavi.demo.overview")
}

func TestCatalogIncludesDemoControlWidgets(t *testing.T) {
	svc := NewService(newFakeRepo(), nil)
	catalog := svc.Catalog(context.Background(), AuthContext{})
	seen := map[string]bool{}
	for _, item := range catalog {
		seen[item.ID] = true
	}
	for _, widgetID := range []string{"homenavi.device.multi", "homenavi.group.controls"} {
		if !seen[widgetID] {
			t.Fatalf("expected %q in catalog", widgetID)
		}
	}
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
