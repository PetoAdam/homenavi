package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	dbinfra "github.com/PetoAdam/homenavi/entity-registry-service/internal/infra/db"
	"github.com/PetoAdam/homenavi/shared/hdp"
	"github.com/PetoAdam/homenavi/shared/mqttx"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testStore struct {
	revision int64
	items    []dbinfra.DiscoveryMetadata
	calls    int
}

func (store *testStore) ApplyDiscoverySnapshot(_ context.Context, revision int64, items []dbinfra.DiscoveryMetadata) error {
	store.revision, store.items = revision, items
	store.calls++
	return nil
}

func TestReconcilePagesAndRejectsChangedRevision(t *testing.T) {
	for _, changed := range []bool{false, true} {
		store := &testStore{}
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Query().Get("cursor") == "" {
				_, _ = fmt.Fprint(writer, `{"devices":[{"device_id":"thread/one","source_id":"00000000-0000-0000-0000-000000000001","capabilities":[]}],"revision":4,"next_cursor":"next"}`)
				return
			}
			if request.URL.Query().Get("revision") != "4" {
				t.Error("revision not propagated")
			}
			if changed {
				writer.WriteHeader(http.StatusConflict)
				return
			}
			_, _ = fmt.Fprint(writer, `{"devices":[{"device_id":"thread/two","source_id":"00000000-0000-0000-0000-000000000002","capabilities":[]}],"revision":4,"next_cursor":""}`)
		}))
		err := Reconcile(context.Background(), store, server.URL, server.Client())
		server.Close()
		if changed {
			if err == nil || store.calls != 0 {
				t.Fatalf("changed scan err=%v calls=%d", err, store.calls)
			}
		} else if err != nil || store.calls != 1 || store.revision != 4 || len(store.items) != 2 {
			t.Fatalf("err=%v store=%+v", err, store)
		}
	}
}

func TestReconcileRejectsInvalidOrUnavailableFeeds(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"unavailable", http.StatusServiceUnavailable, ""},
		{"invalid json", http.StatusOK, "{"},
		{"missing revision", http.StatusOK, `{"devices":[]}`},
		{"negative revision", http.StatusOK, `{"devices":[],"revision":-1}`},
		{"missing devices", http.StatusOK, `{"revision":1}`},
		{"missing source identity", http.StatusOK, `{"devices":[{"device_id":"thread/one","capabilities":[]}],"revision":1}`},
		{"duplicate source identity", http.StatusOK, `{"devices":[{"device_id":"thread/one","source_id":"00000000-0000-0000-0000-000000000001","capabilities":[]},{"device_id":"thread/two","source_id":"00000000-0000-0000-0000-000000000001","capabilities":[]}],"revision":1}`},
		{"empty cursor page", http.StatusOK, `{"devices":[],"revision":1,"next_cursor":"next"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &testStore{}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = fmt.Fprint(writer, test.body)
			}))
			defer server.Close()
			if err := Reconcile(context.Background(), store, server.URL, server.Client()); err == nil || store.calls != 0 {
				t.Fatalf("err=%v calls=%d", err, store.calls)
			}
		})
	}
}

func TestReconcileRejectsRepeatedCursorAndRevisionMismatch(t *testing.T) {
	for _, test := range []struct {
		name   string
		second string
	}{
		{"repeated cursor", `{"devices":[{"device_id":"thread/two","source_id":"00000000-0000-0000-0000-000000000002","capabilities":[]}],"revision":4,"next_cursor":"next"}`},
		{"revision mismatch", `{"devices":[{"device_id":"thread/two","source_id":"00000000-0000-0000-0000-000000000002","capabilities":[]}],"revision":5,"next_cursor":""}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &testStore{}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Query().Get("cursor") == "" {
					_, _ = fmt.Fprint(writer, `{"devices":[{"device_id":"thread/one","source_id":"00000000-0000-0000-0000-000000000001","capabilities":[]}],"revision":4,"next_cursor":"next"}`)
				} else {
					_, _ = fmt.Fprint(writer, test.second)
				}
			}))
			defer server.Close()
			if err := Reconcile(context.Background(), store, server.URL, server.Client()); err == nil || store.calls != 0 {
				t.Fatalf("err=%v calls=%d", err, store.calls)
			}
		})
	}
}

func TestReconcileRetriesOversizedPage(t *testing.T) {
	store := &testStore{}
	var limits []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		limit := request.URL.Query().Get("limit")
		limits = append(limits, limit)
		if limit == "100" {
			writer.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		_, _ = fmt.Fprint(writer, `{"devices":[{"device_id":"thread/one","source_id":"00000000-0000-0000-0000-000000000001","capabilities":[]}],"revision":4,"next_cursor":""}`)
	}))
	defer server.Close()
	if err := Reconcile(context.Background(), store, server.URL, server.Client()); err != nil {
		t.Fatal(err)
	}
	if len(limits) != 2 || limits[0] != "100" || limits[1] != "50" || store.calls != 1 {
		t.Fatalf("limits=%v calls=%d", limits, store.calls)
	}
}

type notifyingStore struct{ calls chan struct{} }

func (store *notifyingStore) ApplyDiscoverySnapshot(_ context.Context, _ int64, _ []dbinfra.DiscoveryMetadata) error {
	store.calls <- struct{}{}
	return nil
}

func TestStartRefreshesOnRetainedMetadata(t *testing.T) {
	brokerURL := os.Getenv("DISCOVERY_TEST_MQTT_URL")
	if brokerURL == "" {
		t.Skip("DISCOVERY_TEST_MQTT_URL not set")
	}
	client, err := mqttx.Connect(mqttx.Options{BrokerURL: brokerURL, BrokerKind: mqttx.BrokerKindMosquitto, ClientIDPrefix: "discovery-test-publisher"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(250)
	if err := client.PublishWith(hdp.MetadataPrefix+"thread/test", []byte(`{"device_id":"thread/test"}`), true); err != nil {
		t.Fatal(err)
	}
	defer client.PublishWith(hdp.MetadataPrefix+"thread/test", nil, true)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(writer, `{"devices":[],"revision":1,"next_cursor":""}`)
	}))
	defer server.Close()
	store := &notifyingStore{calls: make(chan struct{}, 10)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	Start(ctx, store, server.URL, server.Client(), mqttx.Config{BrokerURL: brokerURL, BrokerKind: mqttx.BrokerKindMosquitto}, "")
	for index := 0; index < 2; index++ {
		select {
		case <-store.calls:
		case <-time.After(5 * time.Second):
			t.Fatalf("received only %d reconciliations", index)
		}
	}
	if err := client.Publish(hdp.EventPrefix+"thread/test", []byte(`{"type":"event","event":"device_removed"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.calls:
	case <-time.After(5 * time.Second):
		t.Fatal("removal did not trigger reconciliation")
	}
}

func TestReconcileProjectionRecoversMissedChanges(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := dbinfra.New(database)
	if err != nil {
		t.Fatal(err)
	}
	items := make([]dbinfra.DiscoveryMetadata, 151)
	for index := range items {
		sourceID := uuid.New()
		items[index] = dbinfra.DiscoveryMetadata{DeviceID: fmt.Sprintf("thread/device-%03d", index), SourceID: &sourceID, DeviceType: "sensor", Capabilities: json.RawMessage(`[]`)}
	}
	revision := int64(7)
	interrupt := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		start := 0
		if request.URL.Query().Get("cursor") != "" {
			if interrupt {
				writer.WriteHeader(http.StatusConflict)
				return
			}
			start = 100
		}
		end := start + 100
		if end > len(items) {
			end = len(items)
		}
		next := ""
		if end < len(items) {
			next = "next"
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"devices": items[start:end], "revision": revision, "next_cursor": next})
	}))
	defer server.Close()
	ctx := context.Background()
	if err := Reconcile(ctx, repo, server.URL, server.Client()); err != nil {
		t.Fatal(err)
	}
	first, err := repo.SearchDiscovery(ctx, dbinfra.DiscoveryFilter{Name: "device-000", Limit: 25})
	if err != nil || len(first.Devices) != 1 {
		t.Fatalf("initial page=%+v err=%v", first, err)
	}
	newID := uuid.New()
	items = append(items[1:], dbinfra.DiscoveryMetadata{DeviceID: "thread/new-device", SourceID: &newID, DeviceType: "thermostat", Capabilities: json.RawMessage(`[{"id":"temperature"}]`)})
	revision++
	interrupt = true
	if err := Reconcile(ctx, repo, server.URL, server.Client()); err == nil {
		t.Fatal("interrupted scan succeeded")
	}
	stillPresent, err := repo.SearchDiscovery(ctx, dbinfra.DiscoveryFilter{Name: "device-000", Limit: 25})
	if err != nil || len(stillPresent.Devices) != 1 {
		t.Fatalf("interrupted scan replaced projection: %+v %v", stillPresent, err)
	}
	interrupt = false
	if err := Reconcile(ctx, repo, server.URL, server.Client()); err != nil {
		t.Fatal(err)
	}
	removed, err := repo.SearchDiscovery(ctx, dbinfra.DiscoveryFilter{Name: "device-000", Limit: 25})
	if err != nil || len(removed.Devices) != 0 {
		t.Fatalf("removed page=%+v err=%v", removed, err)
	}
	added, err := repo.SearchDiscovery(ctx, dbinfra.DiscoveryFilter{DeviceType: "thermostat", Capability: "temperature", Limit: 25})
	if err != nil || len(added.Devices) != 1 || added.Devices[0].DeviceID != "thread/new-device" {
		t.Fatalf("added page=%+v err=%v", added, err)
	}
}
