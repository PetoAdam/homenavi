package cache

import (
	"context"
	stdErrors "errors"
	"time"

	"github.com/PetoAdam/homenavi/shared/redisx"
	"github.com/redis/go-redis/v9"
)

var ErrNotFound = stdErrors.New("cache value not found")

type RefreshTokenRotationStatus uint8

const (
	RefreshTokenRotated RefreshTokenRotationStatus = iota
	RefreshTokenMissing
	RefreshTokenReplayed
	RefreshTokenFamilyRevoked
)

type Store interface {
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, keys ...string) error
	TTL(ctx context.Context, key string) (time.Duration, error)
	Increment(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	GetDelete(ctx context.Context, key string) (string, error)
	RotateRefreshToken(ctx context.Context, currentKey, consumedKey, replacementKey, familyKeyPrefix string) (string, RefreshTokenRotationStatus, error)
	Close() error
}

type RedisStore struct {
	client redis.UniversalClient
}

func NewRedisStore(cfg redisx.Config) (*RedisStore, error) {
	client, err := redisx.Connect(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	return &RedisStore{client: client}, nil
}

func (s *RedisStore) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *RedisStore) Get(ctx context.Context, key string) (string, error) {
	value, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if stdErrors.Is(err, redis.Nil) {
			return "", ErrNotFound
		}
		return "", err
	}
	return value, nil
}

func (s *RedisStore) Delete(ctx context.Context, keys ...string) error {
	return s.client.Del(ctx, keys...).Err()
}

func (s *RedisStore) TTL(ctx context.Context, key string) (time.Duration, error) {
	return s.client.TTL(ctx, key).Result()
}

func (s *RedisStore) Increment(ctx context.Context, key string) (int64, error) {
	return s.client.Incr(ctx, key).Result()
}

func (s *RedisStore) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return s.client.Expire(ctx, key, ttl).Err()
}

func (s *RedisStore) GetDelete(ctx context.Context, key string) (string, error) {
	value, err := s.client.GetDel(ctx, key).Result()
	if err != nil {
		if stdErrors.Is(err, redis.Nil) {
			return "", ErrNotFound
		}
		return "", err
	}
	return value, nil
}

var rotateRefreshTokenScript = redis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current then
	local consumedRecord = redis.call('GET', KEYS[2])
	if consumedRecord then
		local consumed = cjson.decode(consumedRecord)
		local replayedFamilyKey = ARGV[1] .. consumed.family_id
    local familyTTL = redis.call('PTTL', replayedFamilyKey)
    if familyTTL > 0 then
      redis.call('SET', replayedFamilyKey, 'revoked', 'PX', familyTTL)
    end
		return {2, consumedRecord}
  end
  return {1, ''}
end

local record = cjson.decode(current)
local familyKey = ARGV[1] .. record.family_id
if redis.call('GET', familyKey) ~= 'active' then
  return {3, ''}
end

local tokenTTL = redis.call('PTTL', KEYS[1])
if tokenTTL <= 0 then
  return {1, ''}
end

redis.call('DEL', KEYS[1])
redis.call('SET', KEYS[2], current, 'PX', tokenTTL)
redis.call('SET', KEYS[3], current, 'PX', tokenTTL)
return {0, current}
`)

// RotateRefreshToken atomically consumes a refresh token, records its family for
// replay detection, and stores a replacement token record with the same expiry.
func (s *RedisStore) RotateRefreshToken(ctx context.Context, currentKey, consumedKey, replacementKey, familyKeyPrefix string) (string, RefreshTokenRotationStatus, error) {
	result, err := rotateRefreshTokenScript.Run(ctx, s.client, []string{currentKey, consumedKey, replacementKey}, familyKeyPrefix).Result()
	if err != nil {
		return "", RefreshTokenMissing, err
	}
	values, ok := result.([]interface{})
	if !ok || len(values) != 2 {
		return "", RefreshTokenMissing, stdErrors.New("unexpected refresh token rotation result")
	}
	statusCode, ok := values[0].(int64)
	if !ok {
		return "", RefreshTokenMissing, stdErrors.New("invalid refresh token rotation status")
	}
	record, ok := values[1].(string)
	if !ok {
		return "", RefreshTokenMissing, stdErrors.New("invalid refresh token rotation record")
	}
	return record, RefreshTokenRotationStatus(statusCode), nil
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}
