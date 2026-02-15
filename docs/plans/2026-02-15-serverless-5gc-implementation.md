# Serverless 5G Core Implementation Plan

**Goal:** Build a serverless 5G core (OpenFaaS, Go) and benchmark its cost efficiency against Open5GS and free5GC for an academic paper.

**Architecture:** Function-per-Procedure pattern where each 3GPP procedure is an OpenFaaS function. ~20 Go functions share state via Redis (UE contexts) and etcd (NRF registry). An SCTP-HTTP proxy bridges the gNB SCTP interface to HTTP. UPF runs as a standard container (go-upf).

**Tech Stack:** Go 1.22+, OpenFaaS on K3s, Redis 7, etcd, UERANSIM, Prometheus/Grafana, IONOS Cloud VMs.

**Key Libraries:**
- `github.com/free5gc/nas` - NAS protocol encoding/decoding
- `github.com/free5gc/ngap` - NGAP protocol (gNB ↔ AMF)
- `github.com/wmnsk/go-pfcp` - PFCP protocol (SMF ↔ UPF)
- `github.com/openfaas/templates-sdk/go-http` - OpenFaaS function handler SDK
- `github.com/redis/go-redis/v9` - Redis client
- `go.etcd.io/etcd/client/v3` - etcd client

**Design doc:** `docs/plans/2026-02-15-serverless-5gc-design.md`

---

## Phase 1: Project Bootstrap & Shared Libraries

### Task 1: Initialize Go Module & Project Structure

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `pkg/state/redis.go`
- Create: `pkg/state/redis_test.go`
- Create: `pkg/state/etcd.go`
- Create: `pkg/state/etcd_test.go`
- Create: `pkg/models/types.go`

**Step 1: Initialize Go module**

```bash
cd /home/tdinh/WorkingSpace/02_Development/serverless5gc
go mod init github.com/tdinh/serverless5gc
```

**Step 2: Create directory structure**

```bash
mkdir -p cmd/sctp-proxy
mkdir -p functions/{amf/{registration,deregistration,service-request,pdu-session-relay,handover,auth-initiate},smf/{pdu-session-create,pdu-session-update,pdu-session-release,n4-session-setup},udm,udr,ausf,nrf,pcf,nssf}
mkdir -p pkg/{state,models,nas,ngap,pfcp,sbi}
mkdir -p deploy/{openfaas,k3s,baselines/{open5gs,free5gc},monitoring,ionos}
mkdir -p eval/{scenarios,scripts,analysis}
```

**Step 3: Create core data models**

Create `pkg/models/types.go` with the fundamental 3GPP data structures used across all NFs:

```go
package models

import "time"

// UEContext represents a UE's state in the AMF, stored in Redis.
type UEContext struct {
    SUPI             string    `json:"supi"`
    GUTI             string    `json:"guti"`
    FiveGTMSI        string    `json:"5g_tmsi"`
    RegistrationState string  `json:"registration_state"` // REGISTERED, DEREGISTERED
    CmState          string    `json:"cm_state"`           // IDLE, CONNECTED
    GnbID            string    `json:"gnb_id"`
    AMFUeNgapID      int64     `json:"amf_ue_ngap_id"`
    RANUeNgapID      int64     `json:"ran_ue_ngap_id"`
    SecurityCtx      *SecurityContext `json:"security_ctx,omitempty"`
    PDUSessions      []string  `json:"pdu_sessions"` // PDU session IDs
    NSSAI            []SNSSAI  `json:"nssai"`
    LastActivity     time.Time `json:"last_activity"`
}

type SecurityContext struct {
    KAMFKey     []byte `json:"kamf_key"`
    NASCount    uint32 `json:"nas_count"`
    AuthStatus  string `json:"auth_status"` // AUTHENTICATED, PENDING
}

type SNSSAI struct {
    SST int32  `json:"sst"`
    SD  string `json:"sd,omitempty"`
}

// PDUSession represents a PDU session in the SMF, stored in Redis.
type PDUSession struct {
    ID            string    `json:"id"`
    SUPI          string    `json:"supi"`
    SNSSAI        SNSSAI    `json:"snssai"`
    DNN           string    `json:"dnn"`
    PDUType       string    `json:"pdu_type"` // IPv4, IPv6, IPv4v6
    UEAddress     string    `json:"ue_address"`
    UPFID         string    `json:"upf_id"`
    State         string    `json:"state"` // ACTIVE, INACTIVE, RELEASED
    QFI           uint8     `json:"qfi"`
    AMBRUL        uint64    `json:"ambr_ul"`
    AMBRDL        uint64    `json:"ambr_dl"`
    CreatedAt     time.Time `json:"created_at"`
}

// NFProfile represents an NF instance in the NRF, stored in etcd.
type NFProfile struct {
    NFInstanceID string   `json:"nf_instance_id"`
    NFType       string   `json:"nf_type"` // AMF, SMF, UDM, etc.
    NFStatus     string   `json:"nf_status"` // REGISTERED, SUSPENDED
    IPv4Addresses []string `json:"ipv4_addresses"`
    NFServices   []NFService `json:"nf_services"`
    PLMN         []PlmnID `json:"plmn_list"`
    HeartbeatTimer int    `json:"heartbeat_timer"`
}

type NFService struct {
    ServiceInstanceID string `json:"service_instance_id"`
    ServiceName       string `json:"service_name"`
    Versions          []string `json:"versions"`
    Scheme            string `json:"scheme"`
    FQDN              string `json:"fqdn"`
}

type PlmnID struct {
    MCC string `json:"mcc"`
    MNC string `json:"mnc"`
}

// SubscriberData represents subscriber info in UDR, stored in Redis.
type SubscriberData struct {
    SUPI               string           `json:"supi"`
    AuthenticationData *AuthData        `json:"auth_data"`
    AccessAndMobility  *AccessMobData   `json:"access_mobility_data"`
    SessionManagement  []SMPolicyData   `json:"session_management"`
}

type AuthData struct {
    AuthMethod     string `json:"auth_method"` // 5G_AKA, EAP_AKA
    PermanentKey   []byte `json:"k"`
    OPc            []byte `json:"opc"`
    AMF            []byte `json:"amf"` // Authentication Management Field (not AMF NF)
    SQN            []byte `json:"sqn"`
}

type AccessMobData struct {
    NSSAI      []SNSSAI `json:"nssai"`
    DefaultDNN string   `json:"default_dnn"`
}

type SMPolicyData struct {
    SNSSAI  SNSSAI `json:"snssai"`
    DNN     string `json:"dnn"`
    QoSRef  int    `json:"qos_ref"`
}
```

**Step 4: Write Redis state store with tests**

Create `pkg/state/redis.go`:

```go
package state

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/redis/go-redis/v9"
)

type RedisStore struct {
    client *redis.Client
}

func NewRedisStore(addr string) *RedisStore {
    return &RedisStore{
        client: redis.NewClient(&redis.Options{Addr: addr}),
    }
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
```

Create `pkg/state/redis_test.go`:

```go
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
```

**Step 5: Write etcd state store with tests**

Create `pkg/state/etcd.go`:

```go
package state

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    clientv3 "go.etcd.io/etcd/client/v3"
)

type EtcdStore struct {
    client *clientv3.Client
}

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
```

Create `pkg/state/etcd_test.go` (uses embedded etcd for testing):

```go
package state

import (
    "context"
    "testing"
)

// EtcdStore tests require a running etcd instance.
// In CI, use testcontainers or embedded etcd.
// For now, these are integration tests tagged with //go:build integration.

//go:build integration

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
```

**Step 6: Install dependencies and run tests**

```bash
go get github.com/redis/go-redis/v9
go get github.com/alicebob/miniredis/v2
go get go.etcd.io/etcd/client/v3
go get github.com/openfaas/templates-sdk/go-http
go get github.com/free5gc/nas
go get github.com/free5gc/ngap
go get github.com/wmnsk/go-pfcp
go mod tidy
```

Run: `go test ./pkg/state/ -v -run TestRedis`
Expected: All 3 Redis tests pass (uses miniredis, no external dependency).

**Step 7: Create Makefile**

```makefile
.PHONY: test test-unit test-integration build lint clean

MODULE := github.com/tdinh/serverless5gc

test-unit:
	go test ./... -v -count=1

test-integration:
	go test ./... -v -count=1 -tags=integration

test: test-unit

build-proxy:
	go build -o bin/sctp-proxy ./cmd/sctp-proxy/

build-functions:
	@for dir in functions/*/; do \
		echo "Building $$dir..."; \
	done

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/
```

**Step 8: Commit**

```bash
git add go.mod go.sum Makefile pkg/ functions/ cmd/ deploy/ eval/
git commit -m "feat: bootstrap project with Go module, state stores, and data models"
```

---

## Phase 2: NRF Functions (Service Discovery Foundation)

### Task 2: Implement NRF Functions

NRF is the foundation - all other NFs register with it and discover each other through it. Build this first.

**Files:**
- Create: `functions/nrf/register/handler.go`
- Create: `functions/nrf/register/handler_test.go`
- Create: `functions/nrf/discover/handler.go`
- Create: `functions/nrf/discover/handler_test.go`
- Create: `functions/nrf/status-notify/handler.go`

**Step 1: Write failing tests for NRF register**

Create `functions/nrf/register/handler_test.go`:

```go
package function

import (
    "encoding/json"
    "net/http"
    "testing"

    handler "github.com/openfaas/templates-sdk/go-http"
    "github.com/tdinh/serverless5gc/pkg/models"
)

func TestHandle_RegisterNF(t *testing.T) {
    profile := models.NFProfile{
        NFInstanceID:  "amf-001",
        NFType:        "AMF",
        NFStatus:      "REGISTERED",
        IPv4Addresses: []string{"10.0.0.1"},
        NFServices: []models.NFService{
            {ServiceName: "namf-comm", Scheme: "http", FQDN: "amf.local"},
        },
    }
    body, _ := json.Marshal(profile)

    req := handler.Request{
        Body:   body,
        Method: "PUT",
        Path:   "/nnrf-nfm/v1/nf-instances/amf-001",
    }

    resp, err := Handle(req)
    if err != nil {
        t.Fatalf("Handle returned error: %v", err)
    }
    if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
        t.Fatalf("got status %d, want 201 or 200", resp.StatusCode)
    }

    var registered models.NFProfile
    if err := json.Unmarshal(resp.Body, &registered); err != nil {
        t.Fatalf("unmarshal response: %v", err)
    }
    if registered.NFInstanceID != "amf-001" {
        t.Fatalf("got ID %s, want amf-001", registered.NFInstanceID)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./functions/nrf/register/ -v`
Expected: FAIL - `Handle` function undefined.

**Step 3: Implement NRF register handler**

Create `functions/nrf/register/handler.go`:

```go
package function

import (
    "encoding/json"
    "fmt"
    "net/http"
    "os"

    handler "github.com/openfaas/templates-sdk/go-http"
    "github.com/tdinh/serverless5gc/pkg/models"
    "github.com/tdinh/serverless5gc/pkg/state"
)

var etcdStore *state.EtcdStore

func init() {
    endpoint := os.Getenv("ETCD_ENDPOINT")
    if endpoint == "" {
        endpoint = "localhost:2379"
    }
    var err error
    etcdStore, err = state.NewEtcdStore([]string{endpoint})
    if err != nil {
        panic(fmt.Sprintf("etcd init: %v", err))
    }
}

func Handle(req handler.Request) (handler.Response, error) {
    var profile models.NFProfile
    if err := json.Unmarshal(req.Body, &profile); err != nil {
        return handler.Response{StatusCode: http.StatusBadRequest,
            Body: []byte(fmt.Sprintf(`{"error":"%s"}`, err))}, nil
    }

    key := fmt.Sprintf("/nrf/nf-instances/%s", profile.NFInstanceID)
    ctx := req.Context()
    if ctx == nil {
        ctx = context.Background()
    }

    if err := etcdStore.Put(ctx, key, profile); err != nil {
        return handler.Response{StatusCode: http.StatusInternalServerError,
            Body: []byte(fmt.Sprintf(`{"error":"%s"}`, err))}, nil
    }

    body, _ := json.Marshal(profile)
    return handler.Response{
        StatusCode: http.StatusCreated,
        Body:       body,
        Header: http.Header{
            "Content-Type": []string{"application/json"},
        },
    }, nil
}
```

Note: The test will need a mock/stub for etcd. Refactor to accept a store interface:

Create `pkg/state/store.go`:

```go
package state

import "context"

// KVStore is the interface for key-value stores (Redis, etcd, mocks).
type KVStore interface {
    Put(ctx context.Context, key string, value interface{}) error
    Get(ctx context.Context, key string, dest interface{}) error
    Delete(ctx context.Context, key string) error
}
```

Then refactor the handler to accept a store via dependency injection (package-level variable for OpenFaaS, overridden in tests).

**Step 4: Implement NRF discover handler**

Create `functions/nrf/discover/handler.go` - queries etcd by NF type prefix:

```go
package function

import (
    "encoding/json"
    "fmt"
    "net/http"

    handler "github.com/openfaas/templates-sdk/go-http"
    "github.com/tdinh/serverless5gc/pkg/models"
    "github.com/tdinh/serverless5gc/pkg/state"
)

func Handle(req handler.Request) (handler.Response, error) {
    nfType := req.QueryStringParameters.Get("target-nf-type")
    if nfType == "" {
        return handler.Response{StatusCode: http.StatusBadRequest,
            Body: []byte(`{"error":"target-nf-type required"}`)}, nil
    }

    // Query etcd for all NF instances, filter by type
    results, err := etcdStore.GetByPrefix(req.Context(), "/nrf/nf-instances/")
    if err != nil {
        return handler.Response{StatusCode: http.StatusInternalServerError,
            Body: []byte(fmt.Sprintf(`{"error":"%s"}`, err))}, nil
    }

    var matched []models.NFProfile
    for _, data := range results {
        var profile models.NFProfile
        if err := json.Unmarshal(data, &profile); err != nil {
            continue
        }
        if profile.NFType == nfType {
            matched = append(matched, profile)
        }
    }

    body, _ := json.Marshal(map[string]interface{}{
        "nfInstances": matched,
    })
    return handler.Response{
        StatusCode: http.StatusOK,
        Body:       body,
        Header:     http.Header{"Content-Type": []string{"application/json"}},
    }, nil
}
```

**Step 5: Run all NRF tests**

Run: `go test ./functions/nrf/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add functions/nrf/ pkg/state/store.go
git commit -m "feat: add NRF register and discover functions"
```

---

## Phase 3: UDM/UDR/AUSF Functions (Authentication & Subscriber Data)

### Task 3: Implement UDR Data Store Functions

**Files:**
- Create: `functions/udr/data-read/handler.go`
- Create: `functions/udr/data-read/handler_test.go`
- Create: `functions/udr/data-write/handler.go`
- Create: `functions/udr/data-write/handler_test.go`

**Step 1: Write failing test for UDR data read**

```go
func TestHandle_ReadSubscriber(t *testing.T) {
    // Pre-populate store with subscriber data
    store := newMockStore()
    sub := models.SubscriberData{
        SUPI: "imsi-001010000000001",
        AuthenticationData: &models.AuthData{
            AuthMethod: "5G_AKA",
            PermanentKey: []byte{0x46, 0x5B, 0x5C, 0xE8, 0xB1, 0x99, 0xB4, 0x9F,
                                  0xAA, 0x5F, 0x0A, 0x2E, 0xE2, 0x38, 0xA6, 0xBC},
        },
    }
    store.Put(context.Background(), "subscribers/imsi-001010000000001", sub)
    SetStore(store)

    req := handler.Request{
        Method: "GET",
        Path:   "/nudr-dr/v1/subscription-data/imsi-001010000000001",
    }

    resp, err := Handle(req)
    if err != nil {
        t.Fatalf("error: %v", err)
    }
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("status %d, want 200", resp.StatusCode)
    }
}
```

**Step 2: Run to verify fail, then implement**

The handler reads subscriber data from Redis by SUPI key.

**Step 3: Implement UDR data write**

Write handler accepts subscriber JSON and stores in Redis.

**Step 4: Run tests, commit**

```bash
go test ./functions/udr/... -v
git add functions/udr/
git commit -m "feat: add UDR data read/write functions"
```

### Task 4: Implement AUSF & UDM Authentication Functions

**Files:**
- Create: `functions/ausf/authenticate/handler.go`
- Create: `functions/ausf/authenticate/handler_test.go`
- Create: `functions/udm/generate-auth-data/handler.go`
- Create: `functions/udm/generate-auth-data/handler_test.go`
- Create: `functions/udm/get-subscriber-data/handler.go`
- Create: `pkg/crypto/milenage.go`
- Create: `pkg/crypto/milenage_test.go`

**Step 1: Implement Milenage crypto (or wrap free5GC's)**

The 5G-AKA authentication requires Milenage algorithm for generating XRES*, AUTN, etc.

Check if `github.com/free5gc/util` provides Milenage. If yes, wrap it. If not, implement the standard algorithm in `pkg/crypto/milenage.go`.

```go
// pkg/crypto/milenage.go
package crypto

// GenerateAuthVector produces a 5G-AKA authentication vector.
// Inputs: K (permanent key), OPc, SQN, AMF, RAND
// Outputs: XRES*, AUTN, RAND, KAUSF
func GenerateAuthVector(k, opc, sqn, amf []byte) (*AuthVector, error) {
    // Use free5gc/util milenage implementation
    // ...
}

type AuthVector struct {
    RAND  []byte `json:"rand"`
    AUTN  []byte `json:"autn"`
    XRES  []byte `json:"xres_star"`
    KAUSF []byte `json:"kausf"`
}
```

**Step 2: Write tests using 3GPP test vectors**

Use test vectors from 3GPP TS 35.208 Annex C to validate Milenage output.

**Step 3: Implement UDM generate-auth-data**

Receives SUPI, looks up subscriber K/OPc from UDR, generates auth vector via Milenage, returns to AUSF.

**Step 4: Implement AUSF authenticate**

Receives UE authentication response (RES*), compares with XRES* from stored auth vector, returns authentication result.

**Step 5: Implement UDM get-subscriber-data**

Returns subscription data (NSSAI, DNN, QoS profiles) for a given SUPI.

**Step 6: Run all auth tests, commit**

```bash
go test ./functions/ausf/... ./functions/udm/... ./pkg/crypto/... -v
git add functions/ausf/ functions/udm/ pkg/crypto/
git commit -m "feat: add AUSF/UDM authentication and subscriber data functions"
```

---

## Phase 4: AMF Functions (Access & Mobility Management)

### Task 5: Implement SCTP-HTTP Proxy

The proxy is the critical bridge between the RAN (SCTP) and serverless functions (HTTP).

**Files:**
- Create: `cmd/sctp-proxy/main.go`
- Create: `cmd/sctp-proxy/proxy.go`
- Create: `cmd/sctp-proxy/proxy_test.go`
- Create: `pkg/ngap/decoder.go`
- Create: `pkg/ngap/decoder_test.go`

**Step 1: Write NGAP message decoder**

Wraps `github.com/free5gc/ngap` to decode NGAP PDUs and determine the target function:

```go
// pkg/ngap/decoder.go
package ngap

import (
    "fmt"

    ngap_message "github.com/free5gc/ngap"
    "github.com/free5gc/ngap/ngapType"
)

// MessageRoute maps an NGAP message to its target OpenFaaS function.
type MessageRoute struct {
    FunctionName string
    ProcedureCode int64
    MessageType  string // "initiating", "successful", "unsuccessful"
}

// RouteNGAP decodes an NGAP PDU and returns the target function name.
func RouteNGAP(data []byte) (*MessageRoute, *ngapType.NGAPPDU, error) {
    pdu, err := ngap_message.Decoder(data)
    if err != nil {
        return nil, nil, fmt.Errorf("ngap decode: %w", err)
    }

    route := &MessageRoute{}
    switch {
    case pdu.InitiatingMessage != nil:
        route.MessageType = "initiating"
        route.ProcedureCode = pdu.InitiatingMessage.ProcedureCode.Value
        switch route.ProcedureCode {
        case ngapType.ProcedureCodeInitialUEMessage:
            route.FunctionName = "amf-initial-registration"
        case ngapType.ProcedureCodeUEContextReleaseRequest:
            route.FunctionName = "amf-deregistration"
        // ... map other procedure codes
        }
    case pdu.SuccessfulOutcome != nil:
        route.MessageType = "successful"
        route.ProcedureCode = pdu.SuccessfulOutcome.ProcedureCode.Value
    }

    return route, pdu, nil
}
```

**Step 2: Write test for NGAP routing**

```go
func TestRouteNGAP_InitialUEMessage(t *testing.T) {
    // Construct a minimal Initial UE Message NGAP PDU
    // Use free5gc/ngap encoder to create test data
    // Verify RouteNGAP returns "amf-initial-registration"
}
```

**Step 3: Implement SCTP proxy**

```go
// cmd/sctp-proxy/proxy.go
package main

import (
    "bytes"
    "fmt"
    "io"
    "log"
    "net"
    "net/http"

    "github.com/ishidawataru/sctp"
    ngapRouter "github.com/tdinh/serverless5gc/pkg/ngap"
)

type SCTPProxy struct {
    listenAddr    string
    openfaasGW    string // e.g., http://gateway.openfaas:8080/function/
    associations  map[string]*sctp.SCTPConn
    httpClient    *http.Client
}

func NewSCTPProxy(listenAddr, openfaasGW string) *SCTPProxy {
    return &SCTPProxy{
        listenAddr:   listenAddr,
        openfaasGW:   openfaasGW,
        associations: make(map[string]*sctp.SCTPConn),
        httpClient:   &http.Client{},
    }
}

func (p *SCTPProxy) Start() error {
    addr, _ := sctp.ResolveSCTPAddr("sctp", p.listenAddr)
    ln, err := sctp.ListenSCTP("sctp", addr)
    if err != nil {
        return fmt.Errorf("sctp listen: %w", err)
    }
    log.Printf("SCTP proxy listening on %s", p.listenAddr)

    for {
        conn, err := ln.AcceptSCTP()
        if err != nil {
            log.Printf("accept: %v", err)
            continue
        }
        go p.handleConnection(conn)
    }
}

func (p *SCTPProxy) handleConnection(conn *sctp.SCTPConn) {
    defer conn.Close()
    buf := make([]byte, 65535)

    for {
        n, _, err := conn.SCTPRead(buf)
        if err != nil {
            if err != io.EOF {
                log.Printf("sctp read: %v", err)
            }
            return
        }

        route, _, err := ngapRouter.RouteNGAP(buf[:n])
        if err != nil {
            log.Printf("ngap route: %v", err)
            continue
        }

        // Forward to OpenFaaS function
        url := fmt.Sprintf("%s%s", p.openfaasGW, route.FunctionName)
        resp, err := p.httpClient.Post(url, "application/octet-stream",
            bytes.NewReader(buf[:n]))
        if err != nil {
            log.Printf("forward to %s: %v", route.FunctionName, err)
            continue
        }

        // Read response and send back via SCTP
        respBody, _ := io.ReadAll(resp.Body)
        resp.Body.Close()

        if len(respBody) > 0 {
            conn.SCTPWrite(respBody, nil)
        }
    }
}
```

**Step 4: Write main.go**

```go
// cmd/sctp-proxy/main.go
package main

import (
    "log"
    "os"
)

func main() {
    listenAddr := os.Getenv("SCTP_LISTEN_ADDR")
    if listenAddr == "" {
        listenAddr = "0.0.0.0:38412"
    }
    openfaasGW := os.Getenv("OPENFAAS_GATEWAY")
    if openfaasGW == "" {
        openfaasGW = "http://gateway.openfaas:8080/function/"
    }

    proxy := NewSCTPProxy(listenAddr, openfaasGW)
    log.Fatal(proxy.Start())
}
```

**Step 5: Build and commit**

```bash
go build -o bin/sctp-proxy ./cmd/sctp-proxy/
git add cmd/sctp-proxy/ pkg/ngap/
git commit -m "feat: add SCTP-HTTP proxy and NGAP routing"
```

### Task 6: Implement AMF Functions

**Files:**
- Create: `functions/amf/registration/handler.go`
- Create: `functions/amf/registration/handler_test.go`
- Create: `functions/amf/deregistration/handler.go`
- Create: `functions/amf/deregistration/handler_test.go`
- Create: `functions/amf/service-request/handler.go`
- Create: `functions/amf/pdu-session-relay/handler.go`
- Create: `functions/amf/auth-initiate/handler.go`
- Create: `pkg/sbi/client.go`

**Step 1: Create SBI client for inter-NF communication**

```go
// pkg/sbi/client.go
package sbi

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
)

// Client calls other NF functions via the OpenFaaS gateway.
type Client struct {
    gateway    string
    httpClient *http.Client
}

func NewClient() *Client {
    gw := os.Getenv("OPENFAAS_GATEWAY")
    if gw == "" {
        gw = "http://gateway.openfaas:8080/function"
    }
    return &Client{gateway: gw, httpClient: &http.Client{}}
}

// CallFunction invokes another OpenFaaS function by name.
func (c *Client) CallFunction(funcName string, payload interface{}, result interface{}) error {
    body, err := json.Marshal(payload)
    if err != nil {
        return err
    }

    url := fmt.Sprintf("%s/%s", c.gateway, funcName)
    resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
    if err != nil {
        return fmt.Errorf("call %s: %w", funcName, err)
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        errBody, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("%s returned %d: %s", funcName, resp.StatusCode, errBody)
    }

    if result != nil {
        return json.NewDecoder(resp.Body).Decode(result)
    }
    return nil
}
```

**Step 2: Implement AMF initial registration**

This is the most complex function - it orchestrates the full UE registration:
1. Decode NAS Registration Request
2. Call `ausf-authenticate` (via SBI client)
3. Call `udm-get-subscriber-data` (via SBI client)
4. Create UE context in Redis
5. Encode NAS Registration Accept
6. Return NGAP response to proxy

```go
// functions/amf/registration/handler.go
package function

import (
    "encoding/json"
    "net/http"

    handler "github.com/openfaas/templates-sdk/go-http"
    "github.com/tdinh/serverless5gc/pkg/models"
    "github.com/tdinh/serverless5gc/pkg/sbi"
    "github.com/tdinh/serverless5gc/pkg/state"
)

func Handle(req handler.Request) (handler.Response, error) {
    // 1. Decode NGAP Initial UE Message containing NAS Registration Request
    // 2. Extract SUPI/SUCI from NAS
    // 3. Call AUSF for authentication
    // 4. Call UDM for subscriber data
    // 5. Create UE context in Redis
    // 6. Build NAS Registration Accept
    // 7. Return NGAP response

    // Implementation follows 3GPP TS 23.502 Section 4.2.2.2
    // Each step calls the relevant function via SBI client

    sbiClient := sbi.NewClient()
    redisStore := state.NewRedisStore(os.Getenv("REDIS_ADDR"))

    // Step 1: Decode NAS from NGAP
    // Use free5gc/nas and free5gc/ngap libraries
    // Extract SUCI or 5G-GUTI

    // Step 2: Authentication
    authReq := map[string]string{"supi": supi}
    var authResult map[string]interface{}
    if err := sbiClient.CallFunction("ausf-authenticate", authReq, &authResult); err != nil {
        return errorResponse(http.StatusInternalServerError, err), nil
    }

    // Step 3: Get subscriber data
    var subData models.SubscriberData
    if err := sbiClient.CallFunction("udm-get-subscriber-data",
        map[string]string{"supi": supi}, &subData); err != nil {
        return errorResponse(http.StatusInternalServerError, err), nil
    }

    // Step 4: Create UE context
    ueCtx := models.UEContext{
        SUPI:              supi,
        RegistrationState: "REGISTERED",
        CmState:           "CONNECTED",
        NSSAI:             subData.AccessAndMobility.NSSAI,
    }
    redisStore.Set(req.Context(), "ue:"+supi, ueCtx, 0)

    // Step 5: Build response
    // Encode NAS Registration Accept using free5gc/nas
    // Wrap in NGAP DownlinkNASTransport

    body, _ := json.Marshal(map[string]string{
        "status": "registered",
        "supi":   supi,
    })
    return handler.Response{StatusCode: http.StatusOK, Body: body}, nil
}
```

**Step 3: Implement remaining AMF functions**

Each follows the same pattern:
- `amf-deregistration`: Reads UE context, marks DEREGISTERED, deletes from Redis
- `amf-service-request`: Updates CM state from IDLE to CONNECTED
- `amf-pdu-session-relay`: Forwards PDU session request to SMF via SBI client
- `amf-auth-initiate`: Initiates authentication, calls AUSF

**Step 4: Write tests for each AMF function**

Tests use mock SBI client and mock Redis store.

**Step 5: Run tests, commit**

```bash
go test ./functions/amf/... -v
git add functions/amf/ pkg/sbi/
git commit -m "feat: add AMF registration, deregistration, and service request functions"
```

---

## Phase 5: SMF Functions (Session Management)

### Task 7: Implement SMF Functions

**Files:**
- Create: `functions/smf/pdu-session-create/handler.go`
- Create: `functions/smf/pdu-session-create/handler_test.go`
- Create: `functions/smf/pdu-session-update/handler.go`
- Create: `functions/smf/pdu-session-release/handler.go`
- Create: `functions/smf/n4-session-setup/handler.go`
- Create: `pkg/pfcp/client.go`

**Step 1: Implement PFCP client for UPF communication**

```go
// pkg/pfcp/client.go
package pfcp

import (
    "net"

    "github.com/wmnsk/go-pfcp/ie"
    "github.com/wmnsk/go-pfcp/message"
)

type Client struct {
    conn    *net.UDPConn
    upfAddr *net.UDPAddr
    seq     uint32
}

func NewClient(upfAddr string) (*Client, error) {
    addr, err := net.ResolveUDPAddr("udp", upfAddr)
    if err != nil {
        return nil, err
    }
    conn, err := net.DialUDP("udp", nil, addr)
    if err != nil {
        return nil, err
    }
    return &Client{conn: conn, upfAddr: addr}, nil
}

// EstablishSession sends PFCP Session Establishment Request to UPF.
func (c *Client) EstablishSession(seid uint64, ueIP string, teid uint32) error {
    c.seq++
    msg := message.NewSessionEstablishmentRequest(
        0, // MP flag
        0, // S flag
        seid,
        c.seq,
        0, // priority
        ie.NewCreatePDR(
            ie.NewPDRID(1),
            ie.NewPrecedence(100),
            ie.NewPDI(
                ie.NewSourceInterface(ie.SrcInterfaceAccess),
                ie.NewFTEID(0x01, teid, net.ParseIP(ueIP), nil, 0),
            ),
        ),
        ie.NewCreateFAR(
            ie.NewFARID(1),
            ie.NewApplyAction(0x02), // Forward
            ie.NewForwardingParameters(
                ie.NewDestinationInterface(ie.DstInterfaceCore),
            ),
        ),
    )

    b := make([]byte, msg.MarshalLen())
    if err := msg.MarshalTo(b); err != nil {
        return err
    }
    _, err := c.conn.Write(b)
    return err
}
```

**Step 2: Implement SMF PDU session create**

The handler:
1. Receives CreateSMContext from AMF (via SBI)
2. Allocates UE IP address (from pool)
3. Calls PCF for policy (`pcf-policy-create` via SBI)
4. Calls UPF via PFCP to set up user plane session
5. Stores PDU session in Redis
6. Returns SM context to AMF

**Step 3: Implement PDU session update and release**

- Update: Modifies QoS, re-sends PFCP Session Modification
- Release: Sends PFCP Session Deletion to UPF, removes from Redis

**Step 4: Tests and commit**

```bash
go test ./functions/smf/... ./pkg/pfcp/... -v
git add functions/smf/ pkg/pfcp/
git commit -m "feat: add SMF PDU session create/update/release functions"
```

---

## Phase 6: PCF & NSSF Functions

### Task 8: Implement PCF and NSSF Functions

**Files:**
- Create: `functions/pcf/policy-create/handler.go`
- Create: `functions/pcf/policy-get/handler.go`
- Create: `functions/nssf/slice-select/handler.go`

These are simpler functions - PCF returns static/configured policy rules, NSSF performs slice selection based on NSSAI.

**Step 1: Implement PCF policy-create**

Returns default QoS policy for a given SNSSAI/DNN combination. Reads policy config from Redis.

**Step 2: Implement NSSF slice-select**

Matches requested NSSAI against configured slices and returns allowed NSSAI.

**Step 3: Tests and commit**

```bash
go test ./functions/pcf/... ./functions/nssf/... -v
git add functions/pcf/ functions/nssf/
git commit -m "feat: add PCF policy and NSSF slice selection functions"
```

---

## Phase 7: OpenFaaS Deployment Configuration

### Task 9: Create OpenFaaS Function Definitions

**Files:**
- Create: `deploy/openfaas/stack.yml`
- Create: `deploy/openfaas/Dockerfile.template`
- Create: `functions/amf/registration/Dockerfile`

**Step 1: Create stack.yml**

```yaml
# deploy/openfaas/stack.yml
version: 1.0
provider:
  name: openfaas
  gateway: http://127.0.0.1:8080

functions:
  # NRF
  nrf-register:
    lang: dockerfile
    handler: ./functions/nrf/register
    image: serverless5gc/nrf-register:latest
    environment:
      ETCD_ENDPOINT: etcd:2379

  nrf-discover:
    lang: dockerfile
    handler: ./functions/nrf/discover
    image: serverless5gc/nrf-discover:latest
    environment:
      ETCD_ENDPOINT: etcd:2379

  # AMF
  amf-initial-registration:
    lang: dockerfile
    handler: ./functions/amf/registration
    image: serverless5gc/amf-initial-registration:latest
    environment:
      REDIS_ADDR: redis:6379
      OPENFAAS_GATEWAY: http://gateway.openfaas:8080/function

  amf-deregistration:
    lang: dockerfile
    handler: ./functions/amf/deregistration
    image: serverless5gc/amf-deregistration:latest
    environment:
      REDIS_ADDR: redis:6379

  amf-service-request:
    lang: dockerfile
    handler: ./functions/amf/service-request
    image: serverless5gc/amf-service-request:latest
    environment:
      REDIS_ADDR: redis:6379

  amf-pdu-session-relay:
    lang: dockerfile
    handler: ./functions/amf/pdu-session-relay
    image: serverless5gc/amf-pdu-session-relay:latest
    environment:
      REDIS_ADDR: redis:6379
      OPENFAAS_GATEWAY: http://gateway.openfaas:8080/function

  # SMF
  smf-pdu-session-create:
    lang: dockerfile
    handler: ./functions/smf/pdu-session-create
    image: serverless5gc/smf-pdu-session-create:latest
    environment:
      REDIS_ADDR: redis:6379
      UPF_PFCP_ADDR: upf:8805
      OPENFAAS_GATEWAY: http://gateway.openfaas:8080/function

  smf-pdu-session-update:
    lang: dockerfile
    handler: ./functions/smf/pdu-session-update
    image: serverless5gc/smf-pdu-session-update:latest
    environment:
      REDIS_ADDR: redis:6379
      UPF_PFCP_ADDR: upf:8805

  smf-pdu-session-release:
    lang: dockerfile
    handler: ./functions/smf/pdu-session-release
    image: serverless5gc/smf-pdu-session-release:latest
    environment:
      REDIS_ADDR: redis:6379
      UPF_PFCP_ADDR: upf:8805

  # UDM/UDR/AUSF
  ausf-authenticate:
    lang: dockerfile
    handler: ./functions/ausf/authenticate
    image: serverless5gc/ausf-authenticate:latest
    environment:
      REDIS_ADDR: redis:6379
      OPENFAAS_GATEWAY: http://gateway.openfaas:8080/function

  udm-generate-auth-data:
    lang: dockerfile
    handler: ./functions/udm/generate-auth-data
    image: serverless5gc/udm-generate-auth-data:latest
    environment:
      OPENFAAS_GATEWAY: http://gateway.openfaas:8080/function

  udm-get-subscriber-data:
    lang: dockerfile
    handler: ./functions/udm/get-subscriber-data
    image: serverless5gc/udm-get-subscriber-data:latest
    environment:
      REDIS_ADDR: redis:6379

  udr-data-read:
    lang: dockerfile
    handler: ./functions/udr/data-read
    image: serverless5gc/udr-data-read:latest
    environment:
      REDIS_ADDR: redis:6379

  udr-data-write:
    lang: dockerfile
    handler: ./functions/udr/data-write
    image: serverless5gc/udr-data-write:latest
    environment:
      REDIS_ADDR: redis:6379

  # PCF/NSSF
  pcf-policy-create:
    lang: dockerfile
    handler: ./functions/pcf/policy-create
    image: serverless5gc/pcf-policy-create:latest
    environment:
      REDIS_ADDR: redis:6379

  pcf-policy-get:
    lang: dockerfile
    handler: ./functions/pcf/policy-get
    image: serverless5gc/pcf-policy-get:latest
    environment:
      REDIS_ADDR: redis:6379

  nssf-slice-select:
    lang: dockerfile
    handler: ./functions/nssf/slice-select
    image: serverless5gc/nssf-slice-select:latest
    environment:
      REDIS_ADDR: redis:6379
```

**Step 2: Create shared Dockerfile template for Go functions**

```dockerfile
# deploy/openfaas/Dockerfile.template
FROM golang:1.22-alpine AS builder
WORKDIR /go/src/handler
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG FUNCTION_PATH
RUN CGO_ENABLED=0 go build -o /handler ${FUNCTION_PATH}

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /handler /handler
EXPOSE 8080
CMD ["/handler"]
```

**Step 3: Commit**

```bash
git add deploy/openfaas/
git commit -m "feat: add OpenFaaS stack.yml and function Dockerfiles"
```

---

## Phase 8: UPF & Supporting Infrastructure

### Task 10: Set Up UPF Container and Docker Compose

**Files:**
- Create: `deploy/k3s/upf-deployment.yaml`
- Create: `deploy/k3s/redis-deployment.yaml`
- Create: `deploy/k3s/etcd-deployment.yaml`
- Create: `deploy/k3s/sctp-proxy-deployment.yaml`

**Step 1: Create UPF Kubernetes manifest**

Using free5GC's go-upf as a container with GTP-U and PFCP configuration:

```yaml
# deploy/k3s/upf-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: upf
  labels:
    app: upf
spec:
  replicas: 1
  selector:
    matchLabels:
      app: upf
  template:
    metadata:
      labels:
        app: upf
    spec:
      containers:
      - name: upf
        image: free5gc/upf:latest
        securityContext:
          capabilities:
            add: ["NET_ADMIN"]
        ports:
        - containerPort: 8805
          protocol: UDP
          name: pfcp
        - containerPort: 2152
          protocol: UDP
          name: gtpu
        volumeMounts:
        - name: upf-config
          mountPath: /free5gc/config/
      volumes:
      - name: upf-config
        configMap:
          name: upf-config
---
apiVersion: v1
kind: Service
metadata:
  name: upf
spec:
  selector:
    app: upf
  ports:
  - name: pfcp
    port: 8805
    protocol: UDP
  - name: gtpu
    port: 2152
    protocol: UDP
```

**Step 2: Create Redis and etcd deployments**

Standard Redis 7 and etcd 3.5 Kubernetes manifests with PersistentVolumeClaims.

**Step 3: Create SCTP proxy deployment**

```yaml
# deploy/k3s/sctp-proxy-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sctp-proxy
spec:
  replicas: 1
  selector:
    matchLabels:
      app: sctp-proxy
  template:
    metadata:
      labels:
        app: sctp-proxy
    spec:
      containers:
      - name: sctp-proxy
        image: serverless5gc/sctp-proxy:latest
        ports:
        - containerPort: 38412
          protocol: SCTP
        env:
        - name: OPENFAAS_GATEWAY
          value: "http://gateway.openfaas:8080/function/"
        - name: SCTP_LISTEN_ADDR
          value: "0.0.0.0:38412"
---
apiVersion: v1
kind: Service
metadata:
  name: sctp-proxy
spec:
  type: NodePort
  selector:
    app: sctp-proxy
  ports:
  - port: 38412
    protocol: SCTP
    nodePort: 38412
```

**Step 4: Commit**

```bash
git add deploy/k3s/
git commit -m "feat: add K3s manifests for UPF, Redis, etcd, and SCTP proxy"
```

---

## Phase 9: Baseline Deployments

### Task 11: Set Up Open5GS Baseline

**Files:**
- Create: `deploy/baselines/open5gs/docker-compose.yml`
- Create: `deploy/baselines/open5gs/config/`

**Step 1: Create Open5GS Docker Compose**

Use official Open5GS Docker images with all NFs:

```yaml
# deploy/baselines/open5gs/docker-compose.yml
version: '3.8'

services:
  mongodb:
    image: mongo:6.0
    volumes:
      - mongo_data:/data/db

  nrf:
    image: gradiant/open5gs:2.7.0
    command: open5gs-nrfd
    depends_on: [mongodb]
    volumes:
      - ./config/nrf.yaml:/etc/open5gs/nrf.yaml

  amf:
    image: gradiant/open5gs:2.7.0
    command: open5gs-amfd
    depends_on: [nrf]
    ports:
      - "38412:38412/sctp"
    volumes:
      - ./config/amf.yaml:/etc/open5gs/amf.yaml

  smf:
    image: gradiant/open5gs:2.7.0
    command: open5gs-smfd
    depends_on: [nrf]
    volumes:
      - ./config/smf.yaml:/etc/open5gs/smf.yaml

  upf:
    image: gradiant/open5gs:2.7.0
    command: open5gs-upfd
    privileged: true
    depends_on: [smf]
    volumes:
      - ./config/upf.yaml:/etc/open5gs/upf.yaml

  udm:
    image: gradiant/open5gs:2.7.0
    command: open5gs-udmd
    depends_on: [nrf]

  udr:
    image: gradiant/open5gs:2.7.0
    command: open5gs-udrd
    depends_on: [mongodb]

  ausf:
    image: gradiant/open5gs:2.7.0
    command: open5gs-ausfd
    depends_on: [nrf]

  pcf:
    image: gradiant/open5gs:2.7.0
    command: open5gs-pcfd
    depends_on: [mongodb]

  nssf:
    image: gradiant/open5gs:2.7.0
    command: open5gs-nssfd
    depends_on: [nrf]

volumes:
  mongo_data:
```

**Step 2: Create configuration files**

Standard Open5GS YAML configs for each NF, tailored to the test PLMN (001/01).

**Step 3: Commit**

```bash
git add deploy/baselines/open5gs/
git commit -m "feat: add Open5GS baseline Docker Compose deployment"
```

### Task 12: Set Up free5GC Baseline

**Files:**
- Create: `deploy/baselines/free5gc/docker-compose.yml`
- Create: `deploy/baselines/free5gc/config/`

**Step 1: Create free5GC Docker Compose**

Similar to Open5GS but using free5GC images. Refer to free5GC's official docker-compose as a starting point.

**Step 2: Configuration files and commit**

```bash
git add deploy/baselines/free5gc/
git commit -m "feat: add free5GC baseline Docker Compose deployment"
```

---

## Phase 10: Monitoring & Metrics Collection

### Task 13: Set Up Prometheus, Grafana, and Custom Metrics

**Files:**
- Create: `deploy/monitoring/prometheus.yml`
- Create: `deploy/monitoring/grafana/dashboards/cost-comparison.json`
- Create: `deploy/monitoring/docker-compose.yml`
- Create: `eval/scripts/cost-exporter/main.go`

**Step 1: Create Prometheus config**

```yaml
# deploy/monitoring/prometheus.yml
global:
  scrape_interval: 5s
  evaluation_interval: 5s

scrape_configs:
  # OpenFaaS gateway metrics
  - job_name: 'openfaas'
    static_configs:
      - targets: ['VM1_IP:8080']
    metrics_path: /metrics

  # cAdvisor for container metrics (all VMs)
  - job_name: 'cadvisor-serverless'
    static_configs:
      - targets: ['VM1_IP:8081']

  - job_name: 'cadvisor-open5gs'
    static_configs:
      - targets: ['VM2_IP:8081']

  - job_name: 'cadvisor-free5gc'
    static_configs:
      - targets: ['VM3_IP:8081']

  # Node exporter for host-level metrics
  - job_name: 'node-serverless'
    static_configs:
      - targets: ['VM1_IP:9100']

  - job_name: 'node-open5gs'
    static_configs:
      - targets: ['VM2_IP:9100']

  - job_name: 'node-free5gc'
    static_configs:
      - targets: ['VM3_IP:9100']

  # Custom cost exporter
  - job_name: 'cost-exporter'
    static_configs:
      - targets: ['VM5_IP:9200']
```

**Step 2: Create custom cost metrics exporter**

```go
// eval/scripts/cost-exporter/main.go
package main

import (
    "fmt"
    "log"
    "net/http"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    functionCost = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "serverless5gc_function_cost_usd",
            Help: "Projected cost in USD based on AWS Lambda pricing",
        },
        []string{"function_name", "pricing_model"},
    )

    totalCostServerless = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "serverless5gc_total_cost_serverless_usd",
        Help: "Total projected cost for serverless deployment",
    })

    totalCostTraditional = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "serverless5gc_total_cost_traditional_usd",
            Help: "Total projected cost for traditional deployment",
        },
        []string{"baseline"}, // "open5gs" or "free5gc"
    )
)

func init() {
    prometheus.MustRegister(functionCost, totalCostServerless, totalCostTraditional)
}

// AWS Lambda pricing (as of 2025):
// $0.0000166667 per GB-second (x86)
// $0.20 per 1M requests
const (
    lambdaPricePerGBSecond = 0.0000166667
    lambdaPricePerRequest  = 0.0000002 // $0.20 / 1M
    // AWS Fargate pricing for comparison:
    fargatePricePerVCPUHour = 0.04048
    fargatePricePerGBHour   = 0.004445
)

func main() {
    // This exporter queries Prometheus for OpenFaaS metrics
    // and calculates projected costs using cloud pricing
    http.Handle("/metrics", promhttp.Handler())
    log.Println("Cost exporter listening on :9200")
    log.Fatal(http.ListenAndServe(":9200", nil))
}
```

**Step 3: Create Grafana dashboard JSON**

Pre-built dashboard with panels:
- Per-function invocation count and duration
- Serverless vs traditional cost over time
- CPU/memory utilization comparison
- Cold start latency histogram
- Registration and PDU session setup latency

**Step 4: Commit**

```bash
git add deploy/monitoring/ eval/scripts/cost-exporter/
git commit -m "feat: add Prometheus, Grafana, and custom cost metrics exporter"
```

---

## Phase 11: IONOS Cloud Infrastructure

### Task 14: Create IONOS VM Provisioning Scripts

**Files:**
- Create: `deploy/ionos/provision.sh`
- Create: `deploy/ionos/setup-serverless.sh`
- Create: `deploy/ionos/setup-open5gs.sh`
- Create: `deploy/ionos/setup-free5gc.sh`
- Create: `deploy/ionos/setup-loadgen.sh`
- Create: `deploy/ionos/setup-monitoring.sh`
- Create: `deploy/ionos/teardown.sh`

**Step 1: Create VM provisioning script**

```bash
#!/bin/bash
# deploy/ionos/provision.sh
# Provisions 5 VMs on IONOS Cloud using ionosctl

set -euo pipefail

DATACENTER_NAME="serverless5gc-eval"
LOCATION="de/fra"  # Frankfurt

# Create datacenter
ionosctl datacenter create --name "$DATACENTER_NAME" --location "$LOCATION" --wait-for-request

DC_ID=$(ionosctl datacenter list --no-headers -o json | jq -r '.items[] | select(.properties.name=="'"$DATACENTER_NAME"'") | .id')

# Create LAN
ionosctl lan create --datacenter-id "$DC_ID" --name "internal" --public=false --wait-for-request
LAN_ID=$(ionosctl lan list --datacenter-id "$DC_ID" --no-headers -o json | jq -r '.items[0].id')

# VM specs
declare -A VM_SPECS=(
    ["serverless5gc"]="8:16384"   # 8 vCPU, 16GB
    ["open5gs"]="8:16384"
    ["free5gc"]="8:16384"
    ["loadgen"]="4:8192"          # 4 vCPU, 8GB
    ["monitoring"]="4:8192"
)

for VM_NAME in "${!VM_SPECS[@]}"; do
    IFS=':' read -r CORES RAM <<< "${VM_SPECS[$VM_NAME]}"
    echo "Creating VM: $VM_NAME ($CORES vCPU, $RAM MB RAM)"

    ionosctl server create \
        --datacenter-id "$DC_ID" \
        --name "$VM_NAME" \
        --cores "$CORES" \
        --ram "$RAM" \
        --image-alias "ubuntu:latest" \
        --ssh-key-path ~/.ssh/id_rsa.pub \
        --wait-for-request

    # Attach to LAN
    SERVER_ID=$(ionosctl server list --datacenter-id "$DC_ID" --no-headers -o json | \
        jq -r '.items[] | select(.properties.name=="'"$VM_NAME"'") | .id')

    ionosctl nic create \
        --datacenter-id "$DC_ID" \
        --server-id "$SERVER_ID" \
        --lan-id "$LAN_ID" \
        --name "${VM_NAME}-nic" \
        --wait-for-request
done

echo "All VMs provisioned. Use 'ionosctl server list --datacenter-id $DC_ID' to see IPs."
```

**Step 2: Create per-VM setup scripts**

`setup-serverless.sh`: Installs K3s, OpenFaaS (via Helm), deploys Redis/etcd/UPF/SCTP-proxy, deploys all functions.

`setup-open5gs.sh`: Installs Docker, pulls Open5GS images, starts docker-compose.

`setup-free5gc.sh`: Installs Docker, pulls free5GC images, starts docker-compose.

`setup-loadgen.sh`: Installs UERANSIM, configures gNB/UE profiles for each scenario.

`setup-monitoring.sh`: Installs Prometheus + Grafana + cAdvisor + node-exporter + cost-exporter.

**Step 3: Create teardown script**

```bash
#!/bin/bash
# deploy/ionos/teardown.sh
DC_ID=$(ionosctl datacenter list --no-headers -o json | \
    jq -r '.items[] | select(.properties.name=="serverless5gc-eval") | .id')

if [ -n "$DC_ID" ]; then
    echo "Deleting datacenter $DC_ID..."
    ionosctl datacenter delete --datacenter-id "$DC_ID" --wait-for-request --force
    echo "Done."
else
    echo "Datacenter not found."
fi
```

**Step 4: Commit**

```bash
git add deploy/ionos/
git commit -m "feat: add IONOS Cloud VM provisioning and setup scripts"
```

---

## Phase 12: Evaluation & Benchmarking

### Task 15: Create Load Generation Scenarios

**Files:**
- Create: `eval/scenarios/low.yaml`
- Create: `eval/scenarios/medium.yaml`
- Create: `eval/scenarios/high.yaml`
- Create: `eval/scenarios/idle.yaml`
- Create: `eval/scenarios/burst.yaml`
- Create: `eval/scripts/run-scenario.sh`

**Step 1: Create UERANSIM scenario configs**

Each scenario YAML specifies:
- Number of UEs to simulate
- Registration rate (UEs per second)
- PDU session parameters
- Test duration
- PLMN and slice info

```yaml
# eval/scenarios/low.yaml
scenario: low
description: "IoT/rural - 100 UEs, 1 reg/sec"
duration_minutes: 30
runs: 3

ueransim:
  gnb:
    mcc: "001"
    mnc: "01"
    tac: 1
    link_ip: "LOADGEN_IP"
    ngap_ip: "TARGET_AMF_IP"
    gtp_ip: "LOADGEN_IP"

  ue_profile:
    count: 100
    registration_rate_per_sec: 1
    pdu_sessions_per_ue: 1
    imsi_start: "001010000000001"
    key: "465B5CE8B199B49FAA5F0A2EE238A6BC"
    opc: "E8ED289DEBA952E4283B54E88E6183CA"
    dnn: "internet"
    snssai:
      sst: 1
      sd: "010203"
```

**Step 2: Create run-scenario.sh**

```bash
#!/bin/bash
# eval/scripts/run-scenario.sh
# Usage: ./run-scenario.sh <scenario> <target> [run_number]
# target: serverless | open5gs | free5gc

SCENARIO=$1
TARGET=$2
RUN=${3:-1}

SCENARIO_FILE="eval/scenarios/${SCENARIO}.yaml"
RESULTS_DIR="eval/results/${TARGET}/${SCENARIO}/run${RUN}"

mkdir -p "$RESULTS_DIR"

echo "=== Starting scenario: $SCENARIO on $TARGET (run $RUN) ==="
echo "$(date -Iseconds)" > "$RESULTS_DIR/start_time"

# 1. Reset Prometheus metrics (snapshot current state)
curl -X POST http://MONITORING_IP:9090/api/v1/admin/tsdb/snapshot

# 2. Start UERANSIM with scenario config
# ... (generate UERANSIM config from scenario YAML, start gnb + ues)

# 3. Wait for scenario duration
DURATION=$(yq '.duration_minutes' "$SCENARIO_FILE")
echo "Running for $DURATION minutes..."
sleep $((DURATION * 60))

# 4. Stop UERANSIM
# ... (kill ue and gnb processes)

echo "$(date -Iseconds)" > "$RESULTS_DIR/end_time"

# 5. Collect metrics
# Export Prometheus data for the test window
START=$(cat "$RESULTS_DIR/start_time")
END=$(cat "$RESULTS_DIR/end_time")

# Query key metrics
for METRIC in \
    "gateway_function_invocation_total" \
    "gateway_functions_seconds_sum" \
    "container_cpu_usage_seconds_total" \
    "container_memory_usage_bytes"; do
    curl -s "http://MONITORING_IP:9090/api/v1/query_range?query=${METRIC}&start=${START}&end=${END}&step=5s" \
        > "$RESULTS_DIR/${METRIC}.json"
done

echo "=== Scenario complete. Results in $RESULTS_DIR ==="
```

**Step 3: Commit**

```bash
git add eval/
git commit -m "feat: add evaluation scenarios and load generation scripts"
```

### Task 16: Create Data Analysis and Chart Generation

**Files:**
- Create: `eval/analysis/analyze.py`
- Create: `eval/analysis/requirements.txt`
- Create: `eval/analysis/charts.py`

**Step 1: Create analysis script**

```python
# eval/analysis/analyze.py
"""
Analyze evaluation results and compute cost comparisons.

Reads Prometheus metric exports from eval/results/ and produces:
- Cost comparison tables (CSV)
- Latency percentile summaries
- Resource utilization summaries
"""
import json
import csv
import sys
from pathlib import Path
from dataclasses import dataclass

# AWS pricing constants for projected cost
LAMBDA_PRICE_PER_GB_SEC = 0.0000166667
LAMBDA_PRICE_PER_REQUEST = 0.0000002
FARGATE_PRICE_PER_VCPU_HR = 0.04048
FARGATE_PRICE_PER_GB_HR = 0.004445

@dataclass
class ScenarioResult:
    scenario: str
    target: str
    run: int
    total_cpu_seconds: float
    total_memory_mb_seconds: float
    total_invocations: int
    avg_latency_ms: float
    p99_latency_ms: float
    projected_cost_usd: float

def compute_serverless_cost(invocations, avg_duration_s, memory_mb):
    gb_seconds = (memory_mb / 1024) * avg_duration_s * invocations
    compute_cost = gb_seconds * LAMBDA_PRICE_PER_GB_SEC
    request_cost = invocations * LAMBDA_PRICE_PER_REQUEST
    return compute_cost + request_cost

def compute_traditional_cost(cpu_seconds, memory_mb, duration_hours):
    vcpu_hours = (cpu_seconds / 3600)
    gb_hours = (memory_mb / 1024) * duration_hours
    return vcpu_hours * FARGATE_PRICE_PER_VCPU_HR + gb_hours * FARGATE_PRICE_PER_GB_HR

def main():
    results_dir = Path("eval/results")
    # ... load and analyze results
    # Output: eval/results/summary.csv

if __name__ == "__main__":
    main()
```

**Step 2: Create chart generation**

```python
# eval/analysis/charts.py
"""Generate paper-ready charts from analysis results."""
import matplotlib.pyplot as plt
import pandas as pd

def plot_cost_comparison(summary_csv, output_dir):
    """Bar chart: cost per scenario across all three systems."""
    df = pd.read_csv(summary_csv)
    # Group by scenario, plot serverless vs open5gs vs free5gc

def plot_cost_crossover(summary_csv, output_dir):
    """Line chart: cost vs UE count, showing crossover point."""

def plot_latency_comparison(summary_csv, output_dir):
    """Box plot: registration latency distribution per system."""

def plot_resource_utilization(summary_csv, output_dir):
    """Stacked area: CPU/memory over time for each system."""

def plot_cold_start_histogram(metrics_dir, output_dir):
    """Histogram: function cold start times."""

if __name__ == "__main__":
    plot_cost_comparison("eval/results/summary.csv", "eval/results/charts/")
    plot_cost_crossover("eval/results/summary.csv", "eval/results/charts/")
    plot_latency_comparison("eval/results/summary.csv", "eval/results/charts/")
    plot_resource_utilization("eval/results/summary.csv", "eval/results/charts/")
    plot_cold_start_histogram("eval/results/", "eval/results/charts/")
```

**Step 3: Create requirements.txt**

```
matplotlib>=3.8
pandas>=2.1
numpy>=1.25
```

**Step 4: Commit**

```bash
git add eval/analysis/
git commit -m "feat: add data analysis and chart generation scripts"
```

---

## Phase 13: Integration Testing & End-to-End Validation

### Task 17: Local Integration Test (Docker Compose)

**Files:**
- Create: `docker-compose.test.yml`
- Create: `test/integration/registration_test.go`
- Create: `test/integration/pdu_session_test.go`

**Step 1: Create local test Docker Compose**

Runs all components locally for integration testing before deploying to IONOS:
- Redis + etcd
- OpenFaaS (faasd for simplicity)
- All functions
- SCTP proxy
- UPF

**Step 2: Write integration tests**

Test the full UE registration flow end-to-end:
1. Register subscriber in UDR
2. Send NAS Registration Request via SCTP proxy
3. Verify UE context created in Redis
4. Establish PDU session
5. Verify PFCP session on UPF
6. Deregister UE

**Step 3: Run and commit**

```bash
docker compose -f docker-compose.test.yml up -d
go test ./test/integration/ -v -tags=integration
docker compose -f docker-compose.test.yml down
git add docker-compose.test.yml test/
git commit -m "feat: add integration tests for full UE registration and PDU session flow"
```

---

## Phase 14: Run Evaluation

### Task 18: Execute Full Evaluation Campaign

This is the final execution phase - no code to write, just running the evaluation.

**Step 1: Provision IONOS VMs**

```bash
./deploy/ionos/provision.sh
```

**Step 2: Set up all environments**

Run in parallel on different VMs:
```bash
ssh vm1 'bash -s' < deploy/ionos/setup-serverless.sh
ssh vm2 'bash -s' < deploy/ionos/setup-open5gs.sh
ssh vm3 'bash -s' < deploy/ionos/setup-free5gc.sh
ssh vm4 'bash -s' < deploy/ionos/setup-loadgen.sh
ssh vm5 'bash -s' < deploy/ionos/setup-monitoring.sh
```

**Step 3: Run all scenarios (3 runs each, 5 scenarios, 3 targets = 45 runs)**

```bash
for SCENARIO in idle low medium high burst; do
    for TARGET in serverless open5gs free5gc; do
        for RUN in 1 2 3; do
            ./eval/scripts/run-scenario.sh $SCENARIO $TARGET $RUN
        done
    done
done
```

**Step 4: Analyze results**

```bash
python eval/analysis/analyze.py
python eval/analysis/charts.py
```

**Step 5: Commit results**

```bash
git add eval/results/
git commit -m "data: add evaluation results and analysis charts"
```

**Step 6: Tear down IONOS VMs**

```bash
./deploy/ionos/teardown.sh
```

---

## Dependency Graph

```
Task 1 (Bootstrap)
  ├── Task 2 (NRF) ─────────────────────────────────────┐
  ├── Task 3 (UDR) ──┐                                  │
  │                   ├── Task 4 (AUSF/UDM) ──┐          │
  ├── Task 5 (SCTP Proxy) ──┐                 │          │
  │                          ├── Task 6 (AMF) ─┤          │
  │                          │                 ├── Task 8 (PCF/NSSF)
  │                          │                 │          │
  │                          │        Task 7 (SMF) ───────┤
  │                          │                            │
  │                          │            Task 9 (OpenFaaS Stack) ──┐
  │                          │                            │         │
  │                          │            Task 10 (K3s Manifests) ──┤
  │                          │                                     │
  ├── Task 11 (Open5GS Baseline) ─────────────────────────────────┤
  ├── Task 12 (free5GC Baseline) ─────────────────────────────────┤
  │                                                               │
  ├── Task 13 (Monitoring) ───────────────────────────────────────┤
  ├── Task 14 (IONOS Provisioning) ───────────────────────────────┤
  │                                                               │
  ├── Task 15 (Load Scenarios) ───────────────────────────────────┤
  ├── Task 16 (Analysis Scripts) ─────────────────────────────────┤
  │                                                               │
  │                                     Task 17 (Integration Test)│
  │                                                               │
  └───────────────────────── Task 18 (Run Evaluation) ────────────┘
```

## Parallelizable Work

These groups can be developed in parallel:

- **Group A** (core NFs): Tasks 2-8 (sequential within group)
- **Group B** (infrastructure): Tasks 9-10, 14 (can start after Task 1)
- **Group C** (baselines): Tasks 11-12 (independent, can start after Task 1)
- **Group D** (evaluation tooling): Tasks 13, 15-16 (can start after Task 1)
