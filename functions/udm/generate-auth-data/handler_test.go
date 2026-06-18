package function

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/haidinhtuan/serverless5gc/pkg/crypto"
	"github.com/haidinhtuan/serverless5gc/pkg/models"
	"github.com/haidinhtuan/serverless5gc/pkg/state"
)

func setupMock(t *testing.T) *state.MockKVStore {
	t.Helper()
	mock := state.NewMockKVStore()
	SetStore(mock)
	return mock
}

func seedSubscriber(t *testing.T, mock *state.MockKVStore) {
	t.Helper()
	k, _ := hex.DecodeString("465b5ce8b199b49faa5f0a2ee238a6bc")
	opc, _ := hex.DecodeString("cd63cb71954a9f4e48a5994e37a02baf")
	sub := models.SubscriberData{
		SUPI: "imsi-001010000000001",
		AuthenticationData: &models.AuthData{
			AuthMethod:   "5G_AKA",
			PermanentKey: k,
			OPc:          opc,
			AMF:          []byte{0xB9, 0xB9},
			SQN:          []byte{0xFF, 0x9B, 0xB4, 0xD0, 0xB6, 0x07},
		},
		AccessAndMobility: &models.AccessMobData{
			NSSAI:      []models.SNSSAI{{SST: 1, SD: "010203"}},
			DefaultDNN: "internet",
		},
	}
	if err := mock.Put(context.Background(), "subscribers/imsi-001010000000001", sub); err != nil {
		t.Fatalf("seed subscriber: %v", err)
	}
}

func TestHandle_GenerateAuthData(t *testing.T) {
	mock := setupMock(t)
	seedSubscriber(t, mock)

	reqBody, _ := json.Marshal(AuthDataRequest{
		SUPI:               "imsi-001010000000001",
		ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org",
	})

	req := handler.Request{Method: "POST", Body: reqBody}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var authResp AuthDataResponse
	if err := json.Unmarshal(resp.Body, &authResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if authResp.AuthType != "5G_AKA" {
		t.Errorf("AuthType = %s, want 5G_AKA", authResp.AuthType)
	}

	// Verify RAND is 16 bytes (32 hex chars)
	randBytes, err := hex.DecodeString(authResp.RAND)
	if err != nil || len(randBytes) != 16 {
		t.Errorf("RAND invalid: %s (len=%d)", authResp.RAND, len(randBytes))
	}

	// Verify AUTN is 16 bytes
	autnBytes, err := hex.DecodeString(authResp.AUTN)
	if err != nil || len(autnBytes) != 16 {
		t.Errorf("AUTN invalid: %s (len=%d)", authResp.AUTN, len(autnBytes))
	}

	// Verify XRES* is 16 bytes
	xresBytes, err := hex.DecodeString(authResp.XREStar)
	if err != nil || len(xresBytes) != 16 {
		t.Errorf("XREStar invalid: %s (len=%d)", authResp.XREStar, len(xresBytes))
	}

	// Verify KAUSF is 32 bytes
	kausfBytes, err := hex.DecodeString(authResp.KAUSF)
	if err != nil || len(kausfBytes) != 32 {
		t.Errorf("KAUSF invalid: %s (len=%d)", authResp.KAUSF, len(kausfBytes))
	}

	// Verify auth vector was stored for AUSF
	var storedAV crypto.AuthVector
	if err := mock.Get(context.Background(), "auth-vectors/imsi-001010000000001", &storedAV); err != nil {
		t.Fatalf("auth vector not stored: %v", err)
	}
	if len(storedAV.XRES) != 16 {
		t.Errorf("stored XRES* length = %d, want 16", len(storedAV.XRES))
	}
}

func sqnUint48(b []byte) uint64 {
	var v uint64
	for _, x := range b {
		v = v<<8 | uint64(x)
	}
	return v & ((1 << 48) - 1)
}

func persistedSQN(t *testing.T, mock *state.MockKVStore, supi string) []byte {
	t.Helper()
	var sub models.SubscriberData
	if err := mock.Get(context.Background(), "subscribers/"+supi, &sub); err != nil {
		t.Fatalf("read subscriber: %v", err)
	}
	if sub.AuthenticationData == nil {
		t.Fatalf("subscriber lost auth data after Handle")
	}
	return sub.AuthenticationData.SQN
}

func callGenerate(t *testing.T, supi string) {
	t.Helper()
	reqBody, _ := json.Marshal(AuthDataRequest{SUPI: supi, ServingNetworkName: "5G:mnc001.mcc001.3gppnetwork.org"})
	resp, err := Handle(handler.Request{Method: "POST", Body: reqBody})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}
}

// TestHandle_AdvancesAndPersistsSQN guards 5G-AKA sequence-number management.
// A USIM (UERANSIM uses indBitLen=5) rejects an AUTN whose SEQ (= SQN>>5) is not
// greater than the highest SEQ it has accepted; for a fresh UE that means SEQ
// must be >= 1. The UDM must advance SEQ before generating each vector and
// persist it, otherwise the AUTN carries SEQ 0 and registration fails with
// "SQN out of range".
func TestHandle_AdvancesAndPersistsSQN(t *testing.T) {
	const supi = "imsi-001010000000077"
	mock := setupMock(t)
	k, _ := hex.DecodeString("465b5ce8b199b49faa5f0a2ee238a6bc")
	opc, _ := hex.DecodeString("cd63cb71954a9f4e48a5994e37a02baf")
	sub := models.SubscriberData{
		SUPI: supi,
		AuthenticationData: &models.AuthData{
			AuthMethod:   "5G_AKA",
			PermanentKey: k,
			OPc:          opc,
			AMF:          []byte{0x80, 0x00},
			SQN:          []byte{0, 0, 0, 0, 0, 0}, // fresh subscriber
		},
	}
	if err := mock.Put(context.Background(), "subscribers/"+supi, sub); err != nil {
		t.Fatalf("seed: %v", err)
	}

	callGenerate(t, supi)
	seq1 := sqnUint48(persistedSQN(t, mock, supi)) >> 5
	if seq1 < 1 {
		t.Fatalf("after first auth SEQ=%d, want >=1 (USIM rejects SEQ 0 as 'SQN out of range')", seq1)
	}

	callGenerate(t, supi)
	seq2 := sqnUint48(persistedSQN(t, mock, supi)) >> 5
	if seq2 <= seq1 {
		t.Fatalf("SEQ did not advance across vectors: %d -> %d", seq1, seq2)
	}
}

func TestHandle_GenerateAuthData_SubscriberNotFound(t *testing.T) {
	setupMock(t)

	reqBody, _ := json.Marshal(AuthDataRequest{SUPI: "imsi-999999999999999"})
	req := handler.Request{Method: "POST", Body: reqBody}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
}

func TestHandle_GenerateAuthData_MissingSUPI(t *testing.T) {
	setupMock(t)

	reqBody, _ := json.Marshal(AuthDataRequest{})
	req := handler.Request{Method: "POST", Body: reqBody}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}
