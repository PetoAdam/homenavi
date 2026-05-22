package cachex

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/PetoAdam/homenavi/shared/redisx"
	"github.com/redis/go-redis/v9"
)

var ErrCacheMiss = errors.New("cache value not found")

type JSONStore struct {
	client redis.UniversalClient
}

func NewJSONStore(ctx context.Context, cfg redisx.Config) (*JSONStore, error) {
	client, err := redisx.Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &JSONStore{client: client}, nil
}

func (s *JSONStore) Get(ctx context.Context, key string, dst any) error {
	if s == nil || s.client == nil {
		return ErrCacheMiss
	}
	value, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrCacheMiss
		}
		return err
	}
	if err := json.Unmarshal(value, dst); err != nil {
		return err
	}
	return nil
}

func (s *JSONStore) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if s == nil || s.client == nil {
		return nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key, payload, ttl).Err()
}

func (s *JSONStore) Delete(ctx context.Context, keys ...string) error {
	if s == nil || s.client == nil || len(keys) == 0 {
		return nil
	}
	return s.client.Del(ctx, keys...).Err()
}

func (s *JSONStore) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}
