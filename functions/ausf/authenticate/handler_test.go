package function

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/crypto"
	"github.com/tdinh/serverless5gc/pkg/state"
)

func setupMock(t *testing.T) *state.MockKVStore {
	t.Helper()
	mock := state.NewMockKVStore()
	SetStore(mock)
	return mock
}

func seedAuthVector(t *testing.T, mock *state.MockKVStore, supi string) *crypto.AuthVector {
	t.Helper()
	// Use known test values to generate an auth vector
	k, _ := hex.DecodeString("465b5ce8b199b49faa5f0a2ee238a6bc")
	opc, _ := hex.DecodeString("cd63cb71954a9f4e48a5994e37a02baf")
	sqn, _ := hex.DecodeString("ff9bb4d0b607")
	amf, _ := hex.DecodeString("b9b9")
	randVal, _ := hex.DecodeString("23553cbe9637a89d218ae64dae47bf35")
	snn := "5G:mnc001.mcc001.3gppnetwork.org"

	av, err := crypto.GenerateAuthVectorWithRAND(k, opc, sqn, amf, randVal, snn)
	if err != nil {
		t.Fatalf("generate auth vector: %v", err)
	}

	if err := mock.Put(context.Background(), "auth-vectors/"+supi, av); err != nil {
		t.Fatalf("store auth vector: %v", err)
	}
	return av
}

func TestHandle_AuthenticateSuccess(t *testing.T) {
	mock := setupMock(t)
	supi := "imsi-001010000000001"
	av := seedAuthVector(t, mock, supi)

	// Send correct RES* (matches XRES*)
	reqBody, _ := json.Marshal(AuthRequest{
		SUPI:    supi,
		RESstar: hex.EncodeToString(av.XRES),
	})

	req := handler.Request{Method: "POST", Body: reqBody}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var authResp AuthResponse
	if err := json.Unmarshal(resp.Body, &authResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if authResp.AuthResult != "SUCCESS" {
		t.Errorf("AuthResult = %s, want SUCCESS", authResp.AuthResult)
	}
	if authResp.SUPI != supi {
		t.Errorf("SUPI = %s, want %s", authResp.SUPI, supi)
	}
	if authResp.KAUSF == "" {
		t.Error("KAUSF should be non-empty on success")
	}

	// Verify auth vector was cleaned up
	var storedAV crypto.AuthVector
	if err := mock.Get(context.Background(), "auth-vectors/"+supi, &storedAV); err == nil {
		t.Error("auth vector should have been deleted after successful auth")
	}
}

func TestHandle_AuthenticateFailure(t *testing.T) {
	mock := setupMock(t)
	supi := "imsi-001010000000001"
	seedAuthVector(t, mock, supi)

	// Send wrong RES*
	reqBody, _ := json.Marshal(AuthRequest{
		SUPI:    supi,
		RESstar: hex.EncodeToString(make([]byte, 16)), // all zeros = wrong
	})

	req := handler.Request{Method: "POST", Body: reqBody}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var authResp AuthResponse
	if err := json.Unmarshal(resp.Body, &authResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if authResp.AuthResult != "FAILURE" {
		t.Errorf("AuthResult = %s, want FAILURE", authResp.AuthResult)
	}
	if authResp.KAUSF != "" {
		t.Error("KAUSF should be empty on failure")
	}
}

func TestHandle_AuthenticateNoVector(t *testing.T) {
	setupMock(t)

	reqBody, _ := json.Marshal(AuthRequest{
		SUPI:    "imsi-999999999999999",
		RESstar: hex.EncodeToString(make([]byte, 16)),
	})

	req := handler.Request{Method: "POST", Body: reqBody}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
}

func TestHandle_AuthenticateMissingFields(t *testing.T) {
	setupMock(t)

	tests := []struct {
		name string
		req  AuthRequest
	}{
		{"missing supi", AuthRequest{RESstar: "abcd"}},
		{"missing res_star", AuthRequest{SUPI: "imsi-001010000000001"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.req)
			resp, err := Handle(handler.Request{Method: "POST", Body: body})
			if err != nil {
				t.Fatalf("Handle error: %v", err)
			}
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status %d, want 400", resp.StatusCode)
			}
		})
	}
}
