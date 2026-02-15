package function

import (
	"context"
	"encoding/json"
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

func TestHandle_Registration_Success(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	mock := newMockSBI()
	mock.responses["ausf-authenticate"] = map[string]string{
		"auth_result": "SUCCESS",
		"supi":        "imsi-001010000000001",
		"kausf":       "aabb",
	}
	mock.responses["udm-get-subscriber-data"] = models.SubscriberData{
		SUPI: "imsi-001010000000001",
		AccessAndMobility: &models.AccessMobData{
			NSSAI:      []models.SNSSAI{{SST: 1, SD: "010203"}},
			DefaultDNN: "internet",
		},
	}
	SetSBI(mock)

	body, _ := json.Marshal(RegistrationRequest{
		SUPI:        "imsi-001010000000001",
		RANUeNgapID: 1,
		GnbID:       "gnb-001",
	})

	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusOK, resp.Body)
	}

	var regResp RegistrationResponse
	if err := json.Unmarshal(resp.Body, &regResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if regResp.Status != "registered" {
		t.Errorf("status = %q, want %q", regResp.Status, "registered")
	}
	if regResp.SUPI != "imsi-001010000000001" {
		t.Errorf("supi = %q, want %q", regResp.SUPI, "imsi-001010000000001")
	}

	// Verify UE context was stored
	var ueCtx models.UEContext
	if err := mockStore.Get(context.Background(), "ue:imsi-001010000000001", &ueCtx); err != nil {
		t.Fatalf("UE context not found in store: %v", err)
	}
	if ueCtx.RegistrationState != "REGISTERED" {
		t.Errorf("registration_state = %q, want REGISTERED", ueCtx.RegistrationState)
	}
	if ueCtx.CmState != "CONNECTED" {
		t.Errorf("cm_state = %q, want CONNECTED", ueCtx.CmState)
	}
	if len(ueCtx.NSSAI) != 1 || ueCtx.NSSAI[0].SST != 1 {
		t.Errorf("NSSAI = %+v, want [{SST:1 SD:010203}]", ueCtx.NSSAI)
	}

	// Verify SBI calls were made
	if len(mock.calls) != 2 {
		t.Fatalf("expected 2 SBI calls, got %d", len(mock.calls))
	}
	if mock.calls[0] != "ausf-authenticate" {
		t.Errorf("first SBI call = %q, want ausf-authenticate", mock.calls[0])
	}
	if mock.calls[1] != "udm-get-subscriber-data" {
		t.Errorf("second SBI call = %q, want udm-get-subscriber-data", mock.calls[1])
	}
}

func TestHandle_Registration_AuthFailure(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	mock := newMockSBI()
	mock.responses["ausf-authenticate"] = map[string]string{
		"auth_result": "FAILURE",
		"supi":        "imsi-001010000000001",
	}
	SetSBI(mock)

	body, _ := json.Marshal(RegistrationRequest{
		SUPI: "imsi-001010000000001",
	})

	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestHandle_Registration_MissingSUPI(t *testing.T) {
	SetStore(state.NewMockKVStore())
	SetSBI(newMockSBI())

	resp, err := Handle(handler.Request{Body: []byte(`{}`), Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandle_Registration_InvalidJSON(t *testing.T) {
	SetStore(state.NewMockKVStore())
	SetSBI(newMockSBI())

	resp, err := Handle(handler.Request{Body: []byte(`not json`), Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}
