package state

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdStore implements KVStore backed by etcd. It is used exclusively for the
// NRF service registry, storing NF instance profiles (NFProfile) and enabling
// service discovery through key-prefix queries (GetByPrefix). Using etcd rather
// than a dedicated NRF database provides strong consistency guarantees and a
// native watch mechanism for reactive NF registration without polling.
type EtcdStore struct {
	client *clientv3.Client
}

// NewEtcdStore creates a new EtcdStore connected to the given endpoints
// (e.g., []string{"localhost:2379"}). Returns an error if the initial
// connection cannot be established within the 5-second dial timeout.
func NewEtcdStore(endpoints []string) (*EtcdStore, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return &EtcdStore{client: cli}, nil
}

// Put stores a JSON-serialized value at the given key.
func (e *EtcdStore) Put(ctx context.Context, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	_, err = e.client.Put(ctx, key, string(data))
	return err
}

// Get retrieves and JSON-deserializes a value. Returns "key %s not found" for missing keys.
func (e *EtcdStore) Get(ctx context.Context, key string, dest interface{}) error {
	resp, err := e.client.Get(ctx, key)
	if err != nil {
		return err
	}
	if len(resp.Kvs) == 0 {
		return fmt.Errorf("key %s not found", key)
	}
	return json.Unmarshal(resp.Kvs[0].Value, dest)
}

// GetByPrefix retrieves all values whose keys start with the given prefix.
// This is used by NRF discovery to find all NF instances of a given type
// (e.g., prefix "nf-instances/AMF/" returns all registered AMF profiles).
func (e *EtcdStore) GetByPrefix(ctx context.Context, prefix string) ([][]byte, error) {
	resp, err := e.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}
	var results [][]byte
	for _, kv := range resp.Kvs {
		results = append(results, kv.Value)
	}
	return results, nil
}

// Delete removes a key from etcd.
func (e *EtcdStore) Delete(ctx context.Context, key string) error {
	_, err := e.client.Delete(ctx, key)
	return err
}

// Close releases the etcd client connection.
func (e *EtcdStore) Close() error {
	return e.client.Close()
}
