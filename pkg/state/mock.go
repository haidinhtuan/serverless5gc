package state

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// MockKVStore is an in-memory KVStore for testing.
type MockKVStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// NewMockKVStore creates a new in-memory mock store.
func NewMockKVStore() *MockKVStore {
	return &MockKVStore{data: make(map[string][]byte)}
}

func (m *MockKVStore) Put(_ context.Context, key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	m.data[key] = data
	return nil
}

func (m *MockKVStore) Get(_ context.Context, key string, dest interface{}) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.data[key]
	if !ok {
		return fmt.Errorf("key %s not found", key)
	}
	return json.Unmarshal(data, dest)
}

func (m *MockKVStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}
