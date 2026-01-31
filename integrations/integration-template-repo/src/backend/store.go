package backend

import "sync"

type CounterStore struct {
	mu    sync.Mutex
	count int
}

func NewCounterStore() *CounterStore {
	return &CounterStore{}
}

func (s *CounterStore) Get() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

func (s *CounterStore) Add(delta int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count += delta
	return s.count
}

func (s *CounterStore) Set(count int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count = count
	return s.count
}
