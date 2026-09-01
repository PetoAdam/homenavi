package commandstate

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/redisx"
	"github.com/redis/go-redis/v9"
)

const (
	redisPrefix = "homenavi:device-hub:command:"
	expiresKey  = redisPrefix + "expires"
)

type RedisStore struct{ client redis.UniversalClient }

func NewRedisStore(ctx context.Context, cfg redisx.Config) (*RedisStore, error) {
	client, err := redisx.Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &RedisStore{client: client}, nil
}

func corrKey(correlation string) string { return redisPrefix + "corr:" + correlation }
func deviceKey(deviceID string) string  { return redisPrefix + "device:" + deviceID }

func (s *RedisStore) Put(ctx context.Context, entry Entry, exclusive bool) (bool, error) {
	if s == nil || s.client == nil || strings.TrimSpace(entry.DeviceID) == "" || strings.TrimSpace(entry.Correlation) == "" {
		return false, nil
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return false, err
	}
	result, err := s.client.Eval(ctx, `
local existing = redis.call('GET', KEYS[1])
if ARGV[1] == '1' and existing and existing ~= ARGV[2] then return 0 end
if existing and existing ~= ARGV[2] then redis.call('DEL', KEYS[2] .. existing) end
redis.call('SET', KEYS[1], ARGV[2])
redis.call('SET', KEYS[2] .. ARGV[2], ARGV[3])
redis.call('ZADD', KEYS[3], ARGV[4], ARGV[2])
return 1`, []string{deviceKey(entry.DeviceID), corrKey(""), expiresKey}, boolArg(exclusive), entry.Correlation, payload, entry.ExpiresAt).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func boolArg(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func (s *RedisStore) Get(ctx context.Context, deviceID, correlation string) (Entry, bool, error) {
	if s == nil || s.client == nil {
		return Entry{}, false, nil
	}
	if correlation == "" && deviceID != "" {
		var err error
		correlation, err = s.client.Get(ctx, deviceKey(deviceID)).Result()
		if err == redis.Nil {
			return Entry{}, false, nil
		}
		if err != nil {
			return Entry{}, false, err
		}
	}
	payload, err := s.client.Get(ctx, corrKey(correlation)).Bytes()
	if err == redis.Nil {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, err
	}
	var entry Entry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return Entry{}, false, err
	}
	if deviceID != "" && entry.DeviceID != deviceID {
		return Entry{}, false, nil
	}
	return entry, true, nil
}

func (s *RedisStore) Remove(ctx context.Context, deviceID, correlation string) (Entry, bool, error) {
	entry, ok, err := s.Get(ctx, deviceID, correlation)
	if err != nil || !ok {
		return Entry{}, false, err
	}
	result, err := s.client.Eval(ctx, `
local payload = redis.call('GET', KEYS[1])
if not payload then return nil end
if redis.call('GET', KEYS[2]) == ARGV[1] then redis.call('DEL', KEYS[2]) end
redis.call('DEL', KEYS[1]); redis.call('ZREM', KEYS[3], ARGV[1]); return payload`, []string{corrKey(entry.Correlation), deviceKey(entry.DeviceID), expiresKey}, entry.Correlation).Result()
	if err != nil {
		return Entry{}, false, err
	}
	return entry, result != nil, nil
}

func (s *RedisStore) ClaimExpired(ctx context.Context, before time.Time) ([]Entry, error) {
	values, err := s.client.Eval(ctx, `
local corr = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
local out = {}
for _, id in ipairs(corr) do
 local key = ARGV[2] .. id
 local payload = redis.call('GET', key)
 if payload then
  local obj = cjson.decode(payload)
  if redis.call('GET', ARGV[3] .. obj.device_id) == id then redis.call('DEL', ARGV[3] .. obj.device_id) end
  redis.call('DEL', key); table.insert(out, payload)
 end
 redis.call('ZREM', KEYS[1], id)
end
return out`, []string{expiresKey}, before.UnixMilli(), corrKey(""), deviceKey("")).StringSlice()
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(values))
	for _, payload := range values {
		var entry Entry
		if json.Unmarshal([]byte(payload), &entry) == nil {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (s *RedisStore) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

var _ Store = (*RedisStore)(nil)
