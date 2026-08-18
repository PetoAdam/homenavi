package commandstate

import (
	"context"
	"sync"
	"time"
)

type Entry struct {
	DeviceID    string         `json:"device_id"`
	Correlation string         `json:"correlation"`
	Expected    map[string]any `json:"expected,omitempty"`
	Baseline    map[string]any `json:"baseline,omitempty"`
	StartedAt   int64          `json:"started_at"`
	LastStatus  string         `json:"last_status,omitempty"`
	ExpiresAt   int64          `json:"expires_at"`
}

type Store interface {
	Put(ctx context.Context, entry Entry, exclusive bool) (bool, error)
	Get(ctx context.Context, deviceID, correlation string) (Entry, bool, error)
	Remove(ctx context.Context, deviceID, correlation string) (Entry, bool, error)
	ClaimExpired(ctx context.Context, before time.Time) ([]Entry, error)
	Close() error
}

type MemoryStore struct {
	mu       sync.Mutex
	byCorr   map[string]Entry
	byDevice map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{byCorr: map[string]Entry{}, byDevice: map[string]string{}}
}

func (s *MemoryStore) Put(_ context.Context, entry Entry, exclusive bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.byDevice[entry.DeviceID]; exclusive && existing != "" && existing != entry.Correlation {
		return false, nil
	}
	if previous := s.byDevice[entry.DeviceID]; previous != "" && previous != entry.Correlation {
		delete(s.byCorr, previous)
	}
	s.byCorr[entry.Correlation] = entry
	s.byDevice[entry.DeviceID] = entry.Correlation
	return true, nil
}

func (s *MemoryStore) Get(_ context.Context, deviceID, correlation string) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if correlation == "" {
		correlation = s.byDevice[deviceID]
	}
	entry, ok := s.byCorr[correlation]
	if !ok || (deviceID != "" && entry.DeviceID != deviceID) {
		return Entry{}, false, nil
	}
	return entry, true, nil
}

func (s *MemoryStore) Remove(_ context.Context, deviceID, correlation string) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if correlation == "" {
		correlation = s.byDevice[deviceID]
	}
	entry, ok := s.byCorr[correlation]
	if !ok || (deviceID != "" && entry.DeviceID != deviceID) {
		return Entry{}, false, nil
	}
	delete(s.byCorr, correlation)
	if s.byDevice[entry.DeviceID] == correlation {
		delete(s.byDevice, entry.DeviceID)
	}
	return entry, true, nil
}

func (s *MemoryStore) ClaimExpired(ctx context.Context, before time.Time) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	expired := make([]Entry, 0)
	for correlation, entry := range s.byCorr {
		if entry.ExpiresAt > before.UnixMilli() {
			continue
		}
		delete(s.byCorr, correlation)
		if s.byDevice[entry.DeviceID] == correlation {
			delete(s.byDevice, entry.DeviceID)
		}
		expired = append(expired, entry)
	}
	return expired, nil
}

func (s *MemoryStore) Close() error { return nil }

var _ Store = (*MemoryStore)(nil)
