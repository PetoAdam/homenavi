package store

import (
	"context"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type StateCache struct{ rdb *redis.Client }

func NewStateCache(rdb *redis.Client) *StateCache { return &StateCache{rdb: rdb} }

func key(id string) string { return "device:state:" + id }

func (c *StateCache) Set(ctx context.Context, id string, stateJSON []byte) error {
	return c.rdb.Set(ctx, key(id), stateJSON, 24*time.Hour).Err()
}

func (c *StateCache) Get(ctx context.Context, id string) ([]byte, error) {
	b, err := c.rdb.Get(ctx, key(id)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	return b, err
}

func (c *StateCache) Delete(ctx context.Context, id string) error {
	return c.rdb.Del(ctx, key(id)).Err()
}

func (c *StateCache) RemoveAllExcept(ctx context.Context, keepIDs []string) ([]string, error) {
	keep := make(map[string]struct{}, len(keepIDs))
	for _, id := range keepIDs {
		if id == "" {
			continue
		}
		keep[id] = struct{}{}
	}
	iter := c.rdb.Scan(ctx, 0, key("*"), 100).Iterator()
	var removed []string
	for iter.Next(ctx) {
		full := iter.Val()
		if !strings.HasPrefix(full, "device:state:") {
			continue
		}
		id := strings.TrimPrefix(full, "device:state:")
		if _, ok := keep[id]; ok {
			continue
		}
		if err := c.rdb.Del(ctx, full).Err(); err != nil {
			return removed, err
		}
		removed = append(removed, id)
	}
	if err := iter.Err(); err != nil {
		return removed, err
	}
	return removed, nil
}
