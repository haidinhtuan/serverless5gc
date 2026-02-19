package state

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore implements KVStore backed by Redis. It is used for all transient
// 5GC state: UE contexts (ue:{supi}), PDU sessions (pdu:{session-id}),
// authentication vectors (auth-vectors/{supi}), charging sessions, slice
// counters, and BSF bindings. Redis's single-threaded execution model provides
// atomic individual commands without external locks, and WATCH/MULTI/EXEC is
// available for multi-key transactions.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a new RedisStore connected to the given address
// (e.g., "localhost:6379"). The connection is established lazily on first use.
func NewRedisStore(addr string) *RedisStore {
	return &RedisStore{
		client: redis.NewClient(&redis.Options{Addr: addr}),
	}
}

// Put stores a JSON-serialized value with no expiration (TTL=0).
func (r *RedisStore) Put(ctx context.Context, key string, value interface{}) error {
	return r.Set(ctx, key, value, 0)
}

// Set stores a JSON-serialized value with an optional TTL. Pass ttl=0 for no expiration.
func (r *RedisStore) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return r.client.Set(ctx, key, data, ttl).Err()
}

// Get retrieves and JSON-deserializes a value. Returns "key %s not found"
// when the key does not exist (redis.Nil is translated to a descriptive error).
func (r *RedisStore) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("key %s not found", key)
		}
		return fmt.Errorf("get %s: %w", key, err)
	}
	return json.Unmarshal(data, dest)
}

// Delete removes a key. No error is returned if the key does not exist.
func (r *RedisStore) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// Exists checks whether a key exists in Redis.
func (r *RedisStore) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	return n > 0, err
}

// Close releases the Redis connection.
func (r *RedisStore) Close() error {
	return r.client.Close()
}
