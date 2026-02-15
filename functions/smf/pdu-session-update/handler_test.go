package function

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/models"
	"github.com/tdinh/serverless5gc/pkg/pfcp"
	"github.com/tdinh/serverless5gc/pkg/state"
)

type mockPFCP struct {
	modifications []pfcpMod
}

type pfcpMod struct {
	SEID   uint64
	Params pfcp.ModifyParams
}

func (m *mockPFCP) ModifySession(seid uint64, params pfcp.ModifyParams) error {
	m.modifications = append(m.modifications, pfcpMod{SEID: seid, Params: params})
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
}

func TestHandle_UpdatePDUSession_QoS(t *testing.T) {
	store, pfcpMock := setup(t)
	seedSession(t, store)

	body, _ := json.Marshal(UpdateSMContextRequest{
		SessionID: "pdu-imsi-001010000000001-1",
		AMBRUL:    2000000,
		AMBRDL:    10000000,
		QFI:       5,
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var updated models.PDUSession
	if err := json.Unmarshal(resp.Body, &updated); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if updated.AMBRUL != 2000000 {
		t.Errorf("AMBRUL = %d, want 2000000", updated.AMBRUL)
	}
	if updated.AMBRDL != 10000000 {
		t.Errorf("AMBRDL = %d, want 10000000", updated.AMBRDL)
	}
	if updated.QFI != 5 {
		t.Errorf("QFI = %d, want 5", updated.QFI)
	}

	// Verify PFCP modification was sent
	if len(pfcpMock.modifications) != 1 {
		t.Fatalf("expected 1 PFCP modification, got %d", len(pfcpMock.modifications))
	}
	mod := pfcpMock.modifications[0]
	if mod.SEID != 1 {
		t.Errorf("PFCP SEID = %d, want 1", mod.SEID)
	}
	if mod.Params.QFI != 5 {
		t.Errorf("PFCP QFI = %d, want 5", mod.Params.QFI)
	}

	// Verify store was updated
	var stored models.PDUSession
	store.Get(context.Background(), "pdu-sessions/pdu-imsi-001010000000001-1", &stored)
	if stored.AMBRUL != 2000000 {
		t.Errorf("stored AMBRUL = %d, want 2000000", stored.AMBRUL)
	}
}

func TestHandle_UpdatePDUSession_NotFound(t *testing.T) {
	setup(t)

	body, _ := json.Marshal(UpdateSMContextRequest{
		SessionID: "pdu-nonexistent-1",
		AMBRUL:    2000000,
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

func TestHandle_UpdatePDUSession_MissingSessionID(t *testing.T) {
	setup(t)

	body, _ := json.Marshal(UpdateSMContextRequest{AMBRUL: 2000000})
	req := handler.Request{Method: "POST", Body: body}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestHandle_UpdatePDUSession_InvalidJSON(t *testing.T) {
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

func TestExtractSEID(t *testing.T) {
	tests := []struct {
		input string
		want  uint64
	}{
		{"pdu-imsi-001010000000001-42", 42},
		{"pdu-imsi-001010000000001-1", 1},
		{"unknown", 0},
	}
	for _, tt := range tests {
		got := extractSEID(tt.input)
		if got != tt.want {
			t.Errorf("extractSEID(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
