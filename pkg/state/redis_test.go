package state

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

type testStruct struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func setupTestRedis(t *testing.T) (*RedisStore, func()) {
	t.Helper()
	mr := miniredis.RunT(t)
	store := NewRedisStore(mr.Addr())
	return store, func() { store.Close(); mr.Close() }
}

func TestRedisStore_SetAndGet(t *testing.T) {
	store, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	input := testStruct{Name: "test", Value: 42}

	if err := store.Set(ctx, "key1", input, 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	var output testStruct
	if err := store.Get(ctx, "key1", &output); err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if output.Name != "test" || output.Value != 42 {
		t.Fatalf("got %+v, want %+v", output, input)
	}
}

func TestRedisStore_Delete(t *testing.T) {
	store, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()
	store.Set(ctx, "key1", testStruct{Name: "x"}, 0)

	if err := store.Delete(ctx, "key1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	exists, _ := store.Exists(ctx, "key1")
	if exists {
		t.Fatal("key should not exist after delete")
	}
}

func TestRedisStore_Exists(t *testing.T) {
	store, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()

	exists, _ := store.Exists(ctx, "nokey")
	if exists {
		t.Fatal("nonexistent key should return false")
	}

	store.Set(ctx, "k", "v", time.Minute)
	exists, _ = store.Exists(ctx, "k")
	if !exists {
		t.Fatal("existing key should return true")
	}
}
