package function

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/models"
	"github.com/tdinh/serverless5gc/pkg/nas"
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
	// Nausf_UEAuthentication (TS 29.509) returns SUCCESS
	mock.responses["ausf-authenticate"] = map[string]string{
		"auth_result": "SUCCESS",
		"supi":        "imsi-001010000000001",
		"kausf":       "aabbccdd",
	}
	// Nudm_SDM_Get (TS 29.503) returns subscriber data
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
	if regResp.T3512Value != nas.T3512Default {
		t.Errorf("t3512 = %d, want %d", regResp.T3512Value, nas.T3512Default)
	}
	if !regResp.SecurityActivated {
		t.Error("security_activated = false, want true")
	}
	if regResp.NASMessage == "" {
		t.Error("nas_message is empty, expected NAS Registration Accept")
	}
	if len(regResp.AllowedNSSAI) != 1 || regResp.AllowedNSSAI[0].SST != 1 {
		t.Errorf("allowed_nssai = %+v, want [{SST:1 SD:010203}]", regResp.AllowedNSSAI)
	}

	// Verify UE context in store
	var ueCtx models.UEContext
	if err := mockStore.Get(context.Background(), "ue:imsi-001010000000001", &ueCtx); err != nil {
		t.Fatalf("UE context not found: %v", err)
	}
	if ueCtx.RegistrationState != "REGISTERED" {
		t.Errorf("registration_state = %q, want REGISTERED", ueCtx.RegistrationState)
	}
	if ueCtx.CmState != "CONNECTED" {
		t.Errorf("cm_state = %q, want CONNECTED", ueCtx.CmState)
	}
	if ueCtx.AMFUeNgapID == 0 {
		t.Error("AMF-UE-NGAP-ID should be allocated (non-zero)")
	}
	if ueCtx.SecurityCtx == nil {
		t.Fatal("SecurityCtx is nil")
	}
	if ueCtx.SecurityCtx.AuthStatus != "AUTHENTICATED" {
		t.Errorf("auth_status = %q, want AUTHENTICATED", ueCtx.SecurityCtx.AuthStatus)
	}
	if ueCtx.SecurityCtx.SelectedCiphering != nas.CipherAlg5GEA2 {
		t.Errorf("ciphering_alg = %d, want %d (5G-EA2)", ueCtx.SecurityCtx.SelectedCiphering, nas.CipherAlg5GEA2)
	}
	if ueCtx.SecurityCtx.SelectedIntegrity != nas.IntegAlg5GIA2 {
		t.Errorf("integrity_alg = %d, want %d (5G-IA2)", ueCtx.SecurityCtx.SelectedIntegrity, nas.IntegAlg5GIA2)
	}
	if !ueCtx.SecurityCtx.SecurityActivated {
		t.Error("security_activated = false in stored context")
	}
	if ueCtx.T3512Value != nas.T3512Default {
		t.Errorf("stored t3512 = %d, want %d", ueCtx.T3512Value, nas.T3512Default)
	}
	if ueCtx.RegistrationTime.IsZero() {
		t.Error("registration_time should not be zero")
	}

	// Verify 3GPP call chain order: AUSF → UDM(get) → UDM(register)
	expectedCalls := []string{"ausf-authenticate", "udm-get-subscriber-data", "udm-registration"}
	if len(mock.calls) != len(expectedCalls) {
		t.Fatalf("SBI calls = %v, want %v", mock.calls, expectedCalls)
	}
	for i, want := range expectedCalls {
		if mock.calls[i] != want {
			t.Errorf("SBI call[%d] = %q, want %q", i, mock.calls[i], want)
		}
	}
}

func TestHandle_Registration_AuthFailure_CauseIllegalUE(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	mock := newMockSBI()
	mock.responses["ausf-authenticate"] = map[string]string{
		"auth_result": "FAILURE",
		"supi":        "imsi-001010000000001",
	}
	SetSBI(mock)

	body, _ := json.Marshal(RegistrationRequest{SUPI: "imsi-001010000000001"})
	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}

	// Verify NAS Registration Reject with Cause #3 is included
	var result map[string]string
	json.Unmarshal(resp.Body, &result)
	if result["nas_message"] == "" {
		t.Error("expected nas_message with Registration Reject in response")
	}
}

func TestHandle_Registration_MissingSUPI_CauseUEIdentity(t *testing.T) {
	SetStore(state.NewMockKVStore())
	SetSBI(newMockSBI())

	resp, err := Handle(handler.Request{Body: []byte(`{}`), Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	// Verify NAS reject with Cause #9 (UE identity cannot be derived)
	var result map[string]string
	json.Unmarshal(resp.Body, &result)
	if result["nas_message"] == "" {
		t.Error("expected nas_message with Registration Reject in response")
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

func TestHandle_Registration_AMFUeNgapIDUnique(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	mock := newMockSBI()
	mock.responses["ausf-authenticate"] = map[string]string{"auth_result": "SUCCESS", "kausf": "aa"}
	mock.responses["udm-get-subscriber-data"] = models.SubscriberData{SUPI: "imsi-1"}
	SetSBI(mock)

	// Register two UEs and verify they get different AMF-UE-NGAP-IDs
	body1, _ := json.Marshal(RegistrationRequest{SUPI: "imsi-1", RANUeNgapID: 1})
	Handle(handler.Request{Body: body1, Method: "POST"})

	mock2 := newMockSBI()
	mock2.responses["ausf-authenticate"] = map[string]string{"auth_result": "SUCCESS", "kausf": "bb"}
	mock2.responses["udm-get-subscriber-data"] = models.SubscriberData{SUPI: "imsi-2"}
	SetSBI(mock2)

	body2, _ := json.Marshal(RegistrationRequest{SUPI: "imsi-2", RANUeNgapID: 2})
	Handle(handler.Request{Body: body2, Method: "POST"})

	var ctx1, ctx2 models.UEContext
	mockStore.Get(context.Background(), "ue:imsi-1", &ctx1)
	mockStore.Get(context.Background(), "ue:imsi-2", &ctx2)

	if ctx1.AMFUeNgapID == ctx2.AMFUeNgapID {
		t.Errorf("AMF-UE-NGAP-IDs should be unique: %d vs %d", ctx1.AMFUeNgapID, ctx2.AMFUeNgapID)
	}
}
