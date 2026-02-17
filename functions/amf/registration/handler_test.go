package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/crypto"
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

func setupSuccessMocks() (*state.MockKVStore, *mockSBI) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	// Pre-populate auth vector in store (simulating what UDM generate-auth-data does)
	testXRES := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	testKAUSF := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	mockStore.Put(context.Background(), "auth-vectors/imsi-001010000000001", &crypto.AuthVector{
		RAND:  []byte{0x00},
		AUTN:  []byte{0x00},
		XRES:  testXRES,
		KAUSF: testKAUSF,
	})

	mock := newMockSBI()
	mock.responses["amf-auth-initiate"] = map[string]string{
		"auth_type": "5G_AKA",
		"rand":      "00112233445566778899aabbccddeeff",
		"autn":      "ffeeddccbbaa99887766554433221100",
		"supi":      "imsi-001010000000001",
	}
	mock.responses["ausf-authenticate"] = map[string]string{
		"auth_result": "SUCCESS",
		"supi":        "imsi-001010000000001",
		"kausf":       "aabbccdd",
	}
	mock.responses["udm-get-subscriber-data"] = models.SubscriberData{
		SUPI: "imsi-001010000000001",
		AccessAndMobility: &models.AccessMobData{
			NSSAI:      []models.SNSSAI{{SST: 1, SD: "010203"}},
			DefaultDNN: "internet",
		},
	}
	SetSBI(mock)
	return mockStore, mock
}

func TestHandle_Registration_Success(t *testing.T) {
	mockStore, mock := setupSuccessMocks()

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

	// Verify 3GPP call chain order: auth-initiate -> AUSF -> UDM(get) -> UDM(register)
	expectedCalls := []string{"amf-auth-initiate", "ausf-authenticate", "udm-get-subscriber-data", "udm-registration"}
	if len(mock.calls) != len(expectedCalls) {
		t.Fatalf("SBI calls = %v, want %v", mock.calls, expectedCalls)
	}
	for i, want := range expectedCalls {
		if mock.calls[i] != want {
			t.Errorf("SBI call[%d] = %q, want %q", i, mock.calls[i], want)
		}
	}
}

func TestHandle_Registration_Success_StateTransitionTimestamps(t *testing.T) {
	mockStore, _ := setupSuccessMocks()

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
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var ueCtx models.UEContext
	if err := mockStore.Get(context.Background(), "ue:imsi-001010000000001", &ueCtx); err != nil {
		t.Fatalf("UE context not found: %v", err)
	}

	// State machine must set RegistrationTime (RM-DEREGISTERED -> RM-REGISTERED)
	if ueCtx.RegistrationTime.IsZero() {
		t.Error("RegistrationTime should be set by state machine transition")
	}

	// LastActivity should also be populated
	if ueCtx.LastActivity.IsZero() {
		t.Error("LastActivity should be set")
	}
}

func TestHandle_Registration_AuthFailure_ProblemDetails(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	// Pre-populate auth vector
	mockStore.Put(context.Background(), "auth-vectors/imsi-001010000000001", &crypto.AuthVector{
		RAND: []byte{0x00}, AUTN: []byte{0x00},
		XRES: []byte{0x01, 0x02, 0x03, 0x04}, KAUSF: []byte{0xaa},
	})

	mock := newMockSBI()
	mock.responses["amf-auth-initiate"] = map[string]string{
		"auth_type": "5G_AKA", "rand": "00", "autn": "00", "supi": "imsi-001010000000001",
	}
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

	// Error responses must use ProblemDetails format per TS 29.571
	var pd models.ProblemDetails
	if err := json.Unmarshal(resp.Body, &pd); err != nil {
		t.Fatalf("response should be ProblemDetails JSON: %v", err)
	}
	if pd.Status != http.StatusForbidden {
		t.Errorf("ProblemDetails.Status = %d, want %d", pd.Status, http.StatusForbidden)
	}
	if pd.Title != "Forbidden" {
		t.Errorf("ProblemDetails.Title = %q, want %q", pd.Title, "Forbidden")
	}
	// Cause should reference the 5GMM cause (Cause #3: Illegal UE)
	if pd.Cause != "ILLEGAL_UE" {
		t.Errorf("ProblemDetails.Cause = %q, want %q", pd.Cause, "ILLEGAL_UE")
	}
	// Detail should include human-readable cause string
	if pd.Detail == "" {
		t.Error("ProblemDetails.Detail should not be empty")
	}
}

func TestHandle_Registration_AuthFailure_NASReject(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	mockStore.Put(context.Background(), "auth-vectors/imsi-001010000000001", &crypto.AuthVector{
		RAND: []byte{0x00}, AUTN: []byte{0x00},
		XRES: []byte{0x01, 0x02, 0x03, 0x04}, KAUSF: []byte{0xaa},
	})

	mock := newMockSBI()
	mock.responses["amf-auth-initiate"] = map[string]string{
		"auth_type": "5G_AKA", "rand": "00", "autn": "00", "supi": "imsi-001010000000001",
	}
	mock.responses["ausf-authenticate"] = map[string]string{
		"auth_result": "FAILURE",
		"supi":        "imsi-001010000000001",
	}
	SetSBI(mock)

	body, _ := json.Marshal(RegistrationRequest{SUPI: "imsi-001010000000001"})
	resp, _ := Handle(handler.Request{Body: body, Method: "POST"})

	// Must still include NAS Registration Reject in response header
	nasHex := resp.Header.Get("X-Nas-Message")
	if nasHex == "" {
		t.Error("expected X-NAS-Message header with Registration Reject")
	}
}

func TestHandle_Registration_MissingSUPI_ProblemDetails(t *testing.T) {
	SetStore(state.NewMockKVStore())
	SetSBI(newMockSBI())

	resp, err := Handle(handler.Request{Body: []byte(`{}`), Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	// Error must be ProblemDetails
	var pd models.ProblemDetails
	if err := json.Unmarshal(resp.Body, &pd); err != nil {
		t.Fatalf("response should be ProblemDetails JSON: %v", err)
	}
	if pd.Status != http.StatusBadRequest {
		t.Errorf("ProblemDetails.Status = %d, want %d", pd.Status, http.StatusBadRequest)
	}
	if pd.Title != "Bad Request" {
		t.Errorf("ProblemDetails.Title = %q, want %q", pd.Title, "Bad Request")
	}

	// Must include NAS reject header
	nasHex := resp.Header.Get("X-Nas-Message")
	if nasHex == "" {
		t.Error("expected X-NAS-Message header with Registration Reject")
	}
}

func TestHandle_Registration_InvalidJSON_ProblemDetails(t *testing.T) {
	SetStore(state.NewMockKVStore())
	SetSBI(newMockSBI())

	resp, err := Handle(handler.Request{Body: []byte(`not json`), Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var pd models.ProblemDetails
	if err := json.Unmarshal(resp.Body, &pd); err != nil {
		t.Fatalf("response should be ProblemDetails JSON: %v", err)
	}
	if pd.Status != http.StatusBadRequest {
		t.Errorf("ProblemDetails.Status = %d, want %d", pd.Status, http.StatusBadRequest)
	}
	if pd.Title != "Bad Request" {
		t.Errorf("ProblemDetails.Title = %q, want %q", pd.Title, "Bad Request")
	}
}

func TestHandle_Registration_SkipAuth(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	mock := newMockSBI()
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
		SkipAuth:    true,
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
	if !regResp.SecurityActivated {
		t.Error("security_activated = false, want true (auth done externally by proxy)")
	}

	// No auth-related SBI calls should have been made
	for _, call := range mock.calls {
		if call == "amf-auth-initiate" || call == "ausf-authenticate" {
			t.Errorf("unexpected auth call %q when skip_auth=true", call)
		}
	}

	// Verify UE context was still created correctly
	var ueCtx models.UEContext
	if err := mockStore.Get(context.Background(), "ue:imsi-001010000000001", &ueCtx); err != nil {
		t.Fatalf("UE context not found: %v", err)
	}
	if ueCtx.RegistrationState != "REGISTERED" {
		t.Errorf("registration_state = %q, want REGISTERED", ueCtx.RegistrationState)
	}
	if ueCtx.SecurityCtx == nil {
		t.Fatal("SecurityCtx is nil")
	}
	if !ueCtx.SecurityCtx.SecurityActivated {
		t.Error("security_activated = false in stored context")
	}
}

func TestHandle_Registration_AMFUeNgapIDUnique(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	// Pre-populate auth vectors for both UEs
	for _, supi := range []string{"imsi-1", "imsi-2"} {
		mockStore.Put(context.Background(), "auth-vectors/"+supi, &crypto.AuthVector{
			RAND: []byte{0x00}, AUTN: []byte{0x00},
			XRES: []byte{0x01, 0x02}, KAUSF: []byte{0xaa},
		})
	}

	mock := newMockSBI()
	mock.responses["amf-auth-initiate"] = map[string]string{"auth_type": "5G_AKA", "rand": "00", "autn": "00", "supi": "imsi-1"}
	mock.responses["ausf-authenticate"] = map[string]string{"auth_result": "SUCCESS", "kausf": "aa"}
	mock.responses["udm-get-subscriber-data"] = models.SubscriberData{SUPI: "imsi-1"}
	SetSBI(mock)

	// Register two UEs and verify they get different AMF-UE-NGAP-IDs
	body1, _ := json.Marshal(RegistrationRequest{SUPI: "imsi-1", RANUeNgapID: 1})
	Handle(handler.Request{Body: body1, Method: "POST"})

	mock2 := newMockSBI()
	mock2.responses["amf-auth-initiate"] = map[string]string{"auth_type": "5G_AKA", "rand": "00", "autn": "00", "supi": "imsi-2"}
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

func TestHandle_Registration_AUSFError_ProblemDetails(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	// Pre-populate auth vector so auth-initiate + store.Get succeed
	mockStore.Put(context.Background(), "auth-vectors/imsi-001010000000001", &crypto.AuthVector{
		RAND: []byte{0x00}, AUTN: []byte{0x00},
		XRES: []byte{0x01, 0x02}, KAUSF: []byte{0xaa},
	})

	mock := newMockSBI()
	mock.responses["amf-auth-initiate"] = map[string]string{
		"auth_type": "5G_AKA", "rand": "00", "autn": "00", "supi": "imsi-001010000000001",
	}
	mock.errors["ausf-authenticate"] = fmt.Errorf("connection refused")
	SetSBI(mock)

	body, _ := json.Marshal(RegistrationRequest{SUPI: "imsi-001010000000001"})
	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}

	var pd models.ProblemDetails
	if err := json.Unmarshal(resp.Body, &pd); err != nil {
		t.Fatalf("response should be ProblemDetails JSON: %v", err)
	}
	if pd.Status != http.StatusInternalServerError {
		t.Errorf("ProblemDetails.Status = %d, want %d", pd.Status, http.StatusInternalServerError)
	}
	if pd.Title != "Internal Server Error" {
		t.Errorf("ProblemDetails.Title = %q, want %q", pd.Title, "Internal Server Error")
	}
}
