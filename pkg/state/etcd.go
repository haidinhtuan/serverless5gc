package state

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdStore implements key-value operations backed by etcd.
type EtcdStore struct {
	client *clientv3.Client
}

// NewEtcdStore creates a new EtcdStore connected to the given endpoints.
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

func (e *EtcdStore) Put(ctx context.Context, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	_, err = e.client.Put(ctx, key, string(data))
	return err
}

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

func (e *EtcdStore) Delete(ctx context.Context, key string) error {
	_, err := e.client.Delete(ctx, key)
	return err
}

func (e *EtcdStore) Close() error {
	return e.client.Close()
}
