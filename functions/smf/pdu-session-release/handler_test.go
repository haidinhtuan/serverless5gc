package function

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/models"
	"github.com/tdinh/serverless5gc/pkg/state"
)

type mockPFCP struct {
	deletions []uint64
}

func (m *mockPFCP) DeleteSession(seid uint64) error {
	m.deletions = append(m.deletions, seid)
	return nil
}

func setup(t *testing.T) (*state.MockKVStore, *mockPFCP) {
	t.Helper()
	store := state.NewMockKVStore()
	SetStore(store)
	pfcpMock := &mockPFCP{}
	SetPFCP(pfcpMock)
	return store, pfcpMock
}

func seedSession(t *testing.T, store *state.MockKVStore) {
	t.Helper()
	session := models.PDUSession{
		ID:        "pdu-imsi-001010000000001-1",
		SUPI:      "imsi-001010000000001",
		SNSSAI:    models.SNSSAI{SST: 1, SD: "010203"},
		DNN:       "internet",
		PDUType:   "IPv4",
		UEAddress: "10.45.0.1",
		State:     "ACTIVE",
		QFI:       9,
		AMBRUL:    1000000,
		AMBRDL:    5000000,
		CreatedAt: time.Now(),
	}
	if err := store.Put(context.Background(), "pdu-sessions/pdu-imsi-001010000000001-1", session); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	// Track IP allocation in Redis (mirrors pdu-session-create behavior)
	if err := store.Put(context.Background(), "ip-pool/allocated/10.45.0.1", "10.45.0.1"); err != nil {
		t.Fatalf("seed IP allocation: %v", err)
	}
}

func TestHandle_ReleasePDUSession(t *testing.T) {
	store, pfcpMock := setup(t)
	seedSession(t, store)

	body, _ := json.Marshal(ReleaseSMContextRequest{
		SessionID: "pdu-imsi-001010000000001-1",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var result map[string]string
	json.Unmarshal(resp.Body, &result)
	if result["state"] != "RELEASED" {
		t.Errorf("state = %s, want RELEASED", result["state"])
	}

	// Verify PFCP deletion was sent
	if len(pfcpMock.deletions) != 1 {
		t.Fatalf("expected 1 PFCP deletion, got %d", len(pfcpMock.deletions))
	}
	if pfcpMock.deletions[0] != 1 {
		t.Errorf("PFCP SEID = %d, want 1", pfcpMock.deletions[0])
	}

	// Verify session removed from store
	var session models.PDUSession
	err = store.Get(context.Background(), "pdu-sessions/pdu-imsi-001010000000001-1", &session)
	if err == nil {
		t.Error("session should be deleted from store")
	}

	// Verify UE IP address released from pool (TS 29.244 Section 5.21)
	var allocatedIP string
	err = store.Get(context.Background(), "ip-pool/allocated/10.45.0.1", &allocatedIP)
	if err == nil {
		t.Error("IP 10.45.0.1 should be released from pool after session release")
	}
}

func TestHandle_ReleasePDUSession_NotFound(t *testing.T) {
	setup(t)

	body, _ := json.Marshal(ReleaseSMContextRequest{
		SessionID: "pdu-nonexistent-1",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
}

func TestHandle_ReleasePDUSession_MissingSessionID(t *testing.T) {
	setup(t)

	body, _ := json.Marshal(ReleaseSMContextRequest{})
	req := handler.Request{Method: "POST", Body: body}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestHandle_ReleasePDUSession_InvalidJSON(t *testing.T) {
	setup(t)

	req := handler.Request{Method: "POST", Body: []byte(`{bad`)}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}
