package cache

import (
	"context"
	stdErrors "errors"
	"time"

	"github.com/PetoAdam/homenavi/shared/redisx"
	"github.com/redis/go-redis/v9"
)

var ErrNotFound = stdErrors.New("cache value not found")

type Store interface {
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, keys ...string) error
	TTL(ctx context.Context, key string) (time.Duration, error)
	Increment(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	GetDelete(ctx context.Context, key string) (string, error)
	Close() error
}

type RedisStore struct {
	client redis.UniversalClient
}

func NewRedisStore(cfg redisx.Config) (*RedisStore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &RedisStore{client: redis.NewUniversalClient(cfg.UniversalOptions())}, nil
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

func (s *RedisStore) Close() error {
	return s.client.Close()
}
