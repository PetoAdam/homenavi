package commandstate

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PetoAdam/homenavi/shared/hdp"
	"github.com/redis/go-redis/v9"
)

func TestRedisTraceMetadataExpiryAndClaim(t *testing.T) {
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	address := os.Getenv("HOMENAVI_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("isolated Redis test address is not configured")
	}
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = client.Close() })
	store := &RedisStore{client: client}
	corr := "trace-test-" + time.Now().Format("20060102150405.000000000")
	deviceID := "test/" + corr
	entry := Entry{DeviceID: deviceID, Correlation: corr, Expected: map[string]any{"empty_array": []any{}}, StartedAt: time.Now().UnixMilli(), ExpiresAt: time.Now().Add(-time.Second).UnixMilli(), Trace: &hdp.TraceContext{Traceparent: "00-11111111111111111111111111111111-2222222222222222-01"}}
	if ok, err := store.Put(ctx, entry, false); err != nil || !ok {
		t.Fatalf("store pending command: %v", err)
	}
	ttl, err := client.PTTL(ctx, traceKey(corr)).Result()
	if err != nil || ttl <= 0 || ttl > 15*time.Minute {
		t.Fatalf("trace must have a bounded TTL: %s %v", ttl, err)
	}
	payload, err := client.Get(ctx, corrKey(corr)).Result()
	if err != nil || strings.Contains(payload, "traceparent") {
		t.Fatalf("lifecycle record must not contain trace metadata: %s %v", payload, err)
	}
	loaded, ok, err := store.Get(ctx, deviceID, corr)
	if err != nil || !ok || loaded.Trace == nil || loaded.Trace.Traceparent != entry.Trace.Traceparent {
		t.Fatalf("trace not restored: %#v %v", loaded.Trace, err)
	}
	expired, err := store.ClaimExpired(ctx, time.Now())
	if err != nil || len(expired) != 1 || expired[0].Trace == nil || expired[0].Trace.Traceparent != entry.Trace.Traceparent {
		t.Fatalf("claim did not include trace: %#v %v", expired, err)
	}
	if _, ok := expired[0].Expected["empty_array"].([]any); !ok {
		t.Fatal("claim changed an empty array in the lifecycle payload")
	}
	if count, err := client.Exists(ctx, traceKey(corr)).Result(); err != nil || count != 0 {
		t.Fatalf("claimed trace key still exists: %d %v", count, err)
	}
	if ok, err := store.Put(ctx, entry, false); err != nil || !ok {
		t.Fatalf("restore command: %v", err)
	}
	if err := client.Del(ctx, traceKey(corr)).Err(); err != nil {
		t.Fatal(err)
	}
	expired, err = store.ClaimExpired(ctx, time.Now())
	if err != nil || len(expired) != 1 || expired[0].Trace != nil {
		t.Fatalf("claim without trace metadata failed: %#v %v", expired, err)
	}
	entry.StartedAt = time.Now().Add(time.Hour).UnixMilli()
	if ok, err := store.Put(ctx, entry, false); err != nil || !ok {
		t.Fatalf("store command with future timestamp: %v", err)
	}
	ttl, err = client.PTTL(ctx, traceKey(corr)).Result()
	if err != nil || ttl <= 0 || ttl > 15*time.Minute {
		t.Fatalf("future timestamp extended trace retention: %s %v", ttl, err)
	}
	if _, ok, err := store.Remove(ctx, deviceID, corr); err != nil || !ok {
		t.Fatalf("remove command: %v", err)
	}
	if count, err := client.Exists(ctx, traceKey(corr)).Result(); err != nil || count != 0 {
		t.Fatalf("removed trace key still exists: %d %v", count, err)
	}
}

func TestRedisTracePermissionFailureDoesNotBreakCommands(t *testing.T) {
	t.Setenv("HOMENAVI_MQTT_TRACE_ENABLED", "true")
	address := os.Getenv("HOMENAVI_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("isolated Redis test address is not configured")
	}
	ctx := context.Background()
	admin := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = admin.Close() })
	user := "trace-denied-" + time.Now().Format("150405.000000000")
	if err := admin.Do(ctx, "ACL", "SETUSER", user, "on", ">test-password", "+@all", "~"+corrKey("*"), "~"+deviceKey("*"), "~"+expiresKey).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Do(ctx, "ACL", "DELUSER", user).Err() })
	client := redis.NewClient(&redis.Options{Addr: address, Username: user, Password: "test-password"})
	defer client.Close()
	store := &RedisStore{client: client}
	entry := Entry{DeviceID: user, Correlation: user, StartedAt: time.Now().UnixMilli(), ExpiresAt: time.Now().Add(-time.Second).UnixMilli(), Trace: &hdp.TraceContext{Traceparent: "00-11111111111111111111111111111111-2222222222222222-01"}}
	if ok, err := store.Put(ctx, entry, true); err != nil || !ok {
		t.Fatalf("trace permission blocked command: accepted=%v err=%v", ok, err)
	}
	if _, ok, err := store.Get(ctx, entry.DeviceID, entry.Correlation); err != nil || !ok {
		t.Fatalf("trace permission blocked lookup: found=%v err=%v", ok, err)
	}
	if expired, err := store.ClaimExpired(ctx, time.Now()); err != nil || len(expired) != 1 {
		t.Fatalf("trace permission blocked expiry: entries=%v err=%v", expired, err)
	}
}

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
