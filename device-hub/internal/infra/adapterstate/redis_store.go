package adapterstate

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/PetoAdam/homenavi/shared/redisx"
	"github.com/redis/go-redis/v9"
)

const redisKey = "homenavi:device-hub:adapter-state"

type RedisStore struct {
	client redis.UniversalClient
}

func NewRedisStore(ctx context.Context, cfg redisx.Config) (*RedisStore, error) {
	client, err := redisx.Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &RedisStore{client: client}, nil
}

func (s *RedisStore) Get(ctx context.Context, adapterID string) (Entry, bool, error) {
	if s == nil || s.client == nil || strings.TrimSpace(adapterID) == "" {
		return Entry{}, false, nil
	}
	payload, err := s.client.HGet(ctx, redisKey, adapterID).Bytes()
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
	return entry, !entry.LastSeen.IsZero(), nil
}

func (s *RedisStore) Put(ctx context.Context, entry Entry, ttl time.Duration) error {
	if s == nil || s.client == nil || strings.TrimSpace(entry.AdapterID) == "" || ttl <= 0 {
		return nil
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	pipeline := s.client.TxPipeline()
	pipeline.HSet(ctx, redisKey, entry.AdapterID, payload)
	pipeline.Expire(ctx, redisKey, ttl)
	_, err = pipeline.Exec(ctx)
	return err
}

func (s *RedisStore) List(ctx context.Context) ([]Entry, error) {
	if s == nil || s.client == nil {
		return nil, nil
	}
	values, err := s.client.HVals(ctx, redisKey).Result()
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
