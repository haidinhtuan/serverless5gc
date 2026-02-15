package state

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore implements key-value operations backed by Redis.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a new RedisStore connected to the given address.
func NewRedisStore(addr string) *RedisStore {
	return &RedisStore{
		client: redis.NewClient(&redis.Options{Addr: addr}),
	}
}

// Put implements KVStore by storing the value with no expiration.
func (r *RedisStore) Put(ctx context.Context, key string, value interface{}) error {
	return r.Set(ctx, key, value, 0)
}

func (r *RedisStore) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return r.client.Set(ctx, key, data, ttl).Err()
}

func (r *RedisStore) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return fmt.Errorf("get %s: %w", key, err)
	}
	return json.Unmarshal(data, dest)
}

func (r *RedisStore) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *RedisStore) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	return n > 0, err
}

func (r *RedisStore) Close() error {
	return r.client.Close()
}
