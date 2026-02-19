package state

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// MockKVStore is an in-memory KVStore for unit testing. It stores JSON-serialized
// values in a map protected by a read-write mutex. All function handler tests
// use this to avoid requiring a running Redis or etcd instance.
type MockKVStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// NewMockKVStore creates a new empty in-memory store.
func NewMockKVStore() *MockKVStore {
	return &MockKVStore{data: make(map[string][]byte)}
}

// Put stores a JSON-serialized value.
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

// Get retrieves and deserializes a value. Returns "key %s not found" for missing keys.
func (m *MockKVStore) Get(_ context.Context, key string, dest interface{}) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.data[key]
	if !ok {
		return fmt.Errorf("key %s not found", key)
	}
	return json.Unmarshal(data, dest)
}

// Delete removes a key from the store.
func (m *MockKVStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}
