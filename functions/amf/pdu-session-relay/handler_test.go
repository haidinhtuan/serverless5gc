package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/models"
	"github.com/tdinh/serverless5gc/pkg/state"
)

type mockSBI struct {
	responses map[string]interface{}
	errors    map[string]error
	calls     []string
}

func newMockSBI() *mockSBI {
	return &mockSBI{
		responses: make(map[string]interface{}),
		errors:    make(map[string]error),
	}
}

func (m *mockSBI) CallFunction(funcName string, payload interface{}, result interface{}) error {
	m.calls = append(m.calls, funcName)
	if err, ok := m.errors[funcName]; ok {
		return err
	}
	if resp, ok := m.responses[funcName]; ok {
		data, _ := json.Marshal(resp)
		return json.Unmarshal(data, result)
	}
	return nil
}

func TestHandle_PDUSessionRelay_Success(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	// Pre-populate registered UE
	ueCtx := models.UEContext{
		SUPI:              "imsi-001010000000001",
		RegistrationState: "REGISTERED",
		CmState:           "CONNECTED",
	}
	mockStore.Put(context.Background(), "ue:imsi-001010000000001", ueCtx)

	mock := newMockSBI()
	mock.responses["smf-pdu-session-create"] = map[string]string{
		"status":     "created",
		"session_id": "pdu-session-001",
	}
	SetSBI(mock)

	body, _ := json.Marshal(PDUSessionRelayRequest{
		SUPI:   "imsi-001010000000001",
		SNSSAI: models.SNSSAI{SST: 1, SD: "010203"},
		DNN:    "internet",
	})

	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusOK, resp.Body)
	}

	var result PDUSessionRelayResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.SessionID != "pdu-session-001" {
		t.Errorf("session_id = %q, want pdu-session-001", result.SessionID)
	}

	// Verify SMF was called
	if len(mock.calls) != 1 || mock.calls[0] != "smf-pdu-session-create" {
		t.Errorf("SBI calls = %v, want [smf-pdu-session-create]", mock.calls)
	}
}

func TestHandle_PDUSessionRelay_UENotRegistered(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	ueCtx := models.UEContext{
		SUPI:              "imsi-001010000000001",
		RegistrationState: "DEREGISTERED",
	}
	mockStore.Put(context.Background(), "ue:imsi-001010000000001", ueCtx)
	SetSBI(newMockSBI())

	body, _ := json.Marshal(PDUSessionRelayRequest{SUPI: "imsi-001010000000001", DNN: "internet"})
	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

func TestHandle_PDUSessionRelay_UENotFound(t *testing.T) {
	SetStore(state.NewMockKVStore())
	SetSBI(newMockSBI())

	body, _ := json.Marshal(PDUSessionRelayRequest{SUPI: "imsi-unknown", DNN: "internet"})
	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHandle_PDUSessionRelay_SMFError(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	ueCtx := models.UEContext{
		SUPI:              "imsi-001010000000001",
		RegistrationState: "REGISTERED",
		CmState:           "CONNECTED",
	}
	mockStore.Put(context.Background(), "ue:imsi-001010000000001", ueCtx)

	mock := newMockSBI()
	mock.errors["smf-pdu-session-create"] = fmt.Errorf("smf unavailable")
	SetSBI(mock)

	body, _ := json.Marshal(PDUSessionRelayRequest{SUPI: "imsi-001010000000001", DNN: "internet"})
	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
}

func TestHandle_PDUSessionRelay_MissingSUPI(t *testing.T) {
	SetStore(state.NewMockKVStore())
	SetSBI(newMockSBI())

	resp, err := Handle(handler.Request{Body: []byte(`{"dnn":"internet"}`), Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}
