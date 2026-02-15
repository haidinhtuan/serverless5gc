//go:build integration

package state

import (
	"context"
	"testing"
)

// EtcdStore tests require a running etcd instance.
// In CI, use testcontainers or embedded etcd.
// These are integration tests tagged with //go:build integration.

func setupTestEtcd(t *testing.T) (*EtcdStore, func()) {
	t.Helper()
	store, err := NewEtcdStore([]string{"localhost:2379"})
	if err != nil {
		t.Skipf("etcd not available: %v", err)
	}
	return store, func() { store.Close() }
}

func TestEtcdStore_PutAndGet(t *testing.T) {
	store, cleanup := setupTestEtcd(t)
	defer cleanup()

	ctx := context.Background()
	input := map[string]string{"name": "test-nf"}

	if err := store.Put(ctx, "/test/key1", input); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	var output map[string]string
	if err := store.Get(ctx, "/test/key1", &output); err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if output["name"] != "test-nf" {
		t.Fatalf("got %v, want test-nf", output["name"])
	}

	// Cleanup
	store.Delete(ctx, "/test/key1")
}

func TestEtcdStore_GetByPrefix(t *testing.T) {
	store, cleanup := setupTestEtcd(t)
	defer cleanup()

	ctx := context.Background()
	store.Put(ctx, "/nrf/nf/amf-1", map[string]string{"type": "AMF"})
	store.Put(ctx, "/nrf/nf/smf-1", map[string]string{"type": "SMF"})

	results, err := store.GetByPrefix(ctx, "/nrf/nf/")
	if err != nil {
		t.Fatalf("GetByPrefix failed: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("got %d results, want >= 2", len(results))
	}

	// Cleanup
	store.Delete(ctx, "/nrf/nf/amf-1")
	store.Delete(ctx, "/nrf/nf/smf-1")
}
