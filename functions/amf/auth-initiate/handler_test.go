package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/haidinhtuan/serverless5gc/pkg/state"
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

func TestHandle_AuthInitiate_Success(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	mock := newMockSBI()
	mock.responses["udm-generate-auth-data"] = map[string]string{
		"auth_type": "5G_AKA",
		"rand":      "aabbccdd",
		"autn":      "11223344",
		"xres_star": "deadbeef",
		"kausf":     "cafebabe",
	}
	SetSBI(mock)

	body, _ := json.Marshal(AuthInitiateRequest{
		SUPI:               "imsi-001010000000001",
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
	})

	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusOK, resp.Body)
	}

	var authResp AuthInitiateResponse
	if err := json.Unmarshal(resp.Body, &authResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if authResp.AuthType != "5G_AKA" {
		t.Errorf("auth_type = %q, want 5G_AKA", authResp.AuthType)
	}
	if authResp.RAND != "aabbccdd" {
		t.Errorf("rand = %q, want aabbccdd", authResp.RAND)
	}
	if authResp.AUTN != "11223344" {
		t.Errorf("autn = %q, want 11223344", authResp.AUTN)
	}
	if authResp.SUPI != "imsi-001010000000001" {
		t.Errorf("supi = %q, want imsi-001010000000001", authResp.SUPI)
	}

	// Verify XRES* was stored for later verification
	var pending map[string]string
	if err := mockStore.Get(context.Background(), "auth-pending:imsi-001010000000001", &pending); err != nil {
		t.Fatalf("auth-pending not stored: %v", err)
	}
	if pending["xres_star"] != "deadbeef" {
		t.Errorf("stored xres_star = %q, want deadbeef", pending["xres_star"])
	}

	// Verify UDM was called
	if len(mock.calls) != 1 || mock.calls[0] != "udm-generate-auth-data" {
		t.Errorf("SBI calls = %v, want [udm-generate-auth-data]", mock.calls)
	}
}

func TestHandle_AuthInitiate_DefaultServingNetwork(t *testing.T) {
	SetStore(state.NewMockKVStore())

	mock := newMockSBI()
	mock.responses["udm-generate-auth-data"] = map[string]string{
		"auth_type": "5G_AKA",
		"rand":      "aa",
		"autn":      "bb",
	}
	SetSBI(mock)

	body, _ := json.Marshal(AuthInitiateRequest{SUPI: "imsi-001010000000001"})
	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestHandle_AuthInitiate_UDMError(t *testing.T) {
	SetStore(state.NewMockKVStore())

	mock := newMockSBI()
	mock.errors["udm-generate-auth-data"] = fmt.Errorf("udm unavailable")
	SetSBI(mock)

	body, _ := json.Marshal(AuthInitiateRequest{SUPI: "imsi-001010000000001"})
	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

func TestHandle_AuthInitiate_MissingSUPI(t *testing.T) {
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

func TestHandle_AuthInitiate_InvalidJSON(t *testing.T) {
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
