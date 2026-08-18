package http

import (
	"testing"
	"time"

	adapterstate "github.com/PetoAdam/homenavi/device-hub/internal/infra/adapterstate"
)

func TestAdapterRegistrySharesPresenceAcrossReplicas(t *testing.T) {
	store := adapterstate.NewMemoryStore()
	writer := newAdapterRegistry(store, time.Minute)
	reader := newAdapterRegistry(store, time.Minute)

	writer.upsertFromHello([]byte(`{
		"adapter_id":"zigbee-primary",
		"protocol":"zigbee",
		"version":"1.2.3",
		"pairing":{"schema_version":"1.0","supported":true,"supports_interview":true}
	}`))

	integrations := reader.integrationsSnapshot()
	if len(integrations) != 1 || integrations[0].Protocol != "zigbee" || integrations[0].Status != "active" {
		t.Fatalf("expected shared active zigbee adapter, got %#v", integrations)
	}
	configs := reader.pairingConfigsSnapshot()
	if len(configs) != 1 || !configs[0].Supported || !configs[0].SupportsInterview {
		t.Fatalf("expected shared pairing configuration, got %#v", configs)
	}
}
