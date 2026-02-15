package state

import "context"

// KVStore is the interface for key-value stores (Redis, etcd, mocks).
type KVStore interface {
	Put(ctx context.Context, key string, value interface{}) error
	Get(ctx context.Context, key string, dest interface{}) error
	Delete(ctx context.Context, key string) error
}
