package commandstate

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStoreSharesExclusiveCommandAndExpiryClaim(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	entry := Entry{DeviceID: "zigbee/one", Correlation: "corr-1", StartedAt: 100, ExpiresAt: 200}
	accepted, err := store.Put(ctx, entry, true)
	if err != nil || !accepted {
		t.Fatalf("first exclusive command accepted=%v err=%v", accepted, err)
	}
	accepted, err = store.Put(ctx, Entry{DeviceID: entry.DeviceID, Correlation: "corr-2", ExpiresAt: 200}, true)
	if err != nil || accepted {
		t.Fatalf("second exclusive command accepted=%v err=%v", accepted, err)
	}
	got, ok, err := store.Get(ctx, entry.DeviceID, "")
	if err != nil || !ok || got.Correlation != entry.Correlation {
		t.Fatalf("cross-replica lookup got=%#v ok=%v err=%v", got, ok, err)
	}
	expired, err := store.ClaimExpired(ctx, time.UnixMilli(200))
	if err != nil || len(expired) != 1 || expired[0].Correlation != entry.Correlation {
		t.Fatalf("expiry claim got=%#v err=%v", expired, err)
	}
	expired, err = store.ClaimExpired(ctx, time.UnixMilli(200))
	if err != nil || len(expired) != 0 {
		t.Fatalf("duplicate expiry claim got=%#v err=%v", expired, err)
	}
}
