package adapterstate

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

type Entry struct {
	AdapterID string          `json:"adapter_id"`
	Protocol  string          `json:"protocol"`
	Status    string          `json:"status"`
	Reason    string          `json:"reason"`
	Version   string          `json:"version"`
	LastSeen  time.Time       `json:"last_seen"`
	Pairing   json.RawMessage `json:"pairing,omitempty"`
}

type Store interface {
	Get(ctx context.Context, adapterID string) (Entry, bool, error)
	Put(ctx context.Context, entry Entry, ttl time.Duration) error
	List(ctx context.Context) ([]Entry, error)
	Close() error
}

type MemoryStore struct {
	mu      sync.RWMutex
	entries map[string]memoryEntry
}

type memoryEntry struct {
	entry     Entry
	expiresAt time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: make(map[string]memoryEntry)}
}

func (s *MemoryStore) Get(_ context.Context, adapterID string) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[adapterID]
	if !ok || !entry.expiresAt.After(time.Now()) {
		delete(s.entries, adapterID)
		return Entry{}, false, nil
	}
	return entry.entry, true, nil
}

func (s *MemoryStore) Put(_ context.Context, entry Entry, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	s.mu.Lock()
	s.entries[entry.AdapterID] = memoryEntry{entry: entry, expiresAt: time.Now().Add(ttl)}
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) List(_ context.Context) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	entries := make([]Entry, 0, len(s.entries))
	for adapterID, stored := range s.entries {
		if !stored.expiresAt.After(now) {
			delete(s.entries, adapterID)
			continue
		}
		entries = append(entries, stored.entry)
	}
	return entries, nil
}

func (s *MemoryStore) Close() error {
	return nil
}

var _ Store = (*MemoryStore)(nil)
