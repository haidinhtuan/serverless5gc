// Package state provides the key-value storage abstraction used by all serverless
// 5GC functions to externalize UE context, PDU session data, and authentication
// vectors. This package implements the 3GPP UDSF (Unstructured Data Storage
// Function, TS 29.598) concept: functions are stateless, and all per-UE state
// lives in an external store accessible via the KVStore interface.
//
// Three implementations are provided:
//   - RedisStore: production store for transient UE/session state (sub-ms latency)
//   - EtcdStore: production store for NRF service registry (strong consistency)
//   - MockKVStore: in-memory store for unit testing without external dependencies
package state

import "context"

// KVStore defines the minimal key-value operations required by 5GC functions.
// All values are JSON-serialized before storage and deserialized on retrieval.
// Implementations must be safe for concurrent use by multiple goroutines.
type KVStore interface {
	// Put stores a JSON-serialized value at the given key, overwriting any existing value.
	Put(ctx context.Context, key string, value interface{}) error
	// Get retrieves the value at key and JSON-deserializes it into dest.
	// Returns an error containing "not found" if the key does not exist.
	Get(ctx context.Context, key string, dest interface{}) error
	// Delete removes the key. No error is returned if the key does not exist.
	Delete(ctx context.Context, key string) error
}
