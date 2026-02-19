package function

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/haidinhtuan/serverless5gc/pkg/models"
	"github.com/haidinhtuan/serverless5gc/pkg/state"
)

type mockSBI struct {
	calls []sbiCall
	resp  map[string]interface{}
}

type sbiCall struct {
	FuncName string
	Payload  interface{}
}

func (m *mockSBI) CallFunction(funcName string, payload interface{}, result interface{}) error {
	m.calls = append(m.calls, sbiCall{FuncName: funcName, Payload: payload})
	if resp, ok := m.resp[funcName]; ok {
		data, _ := json.Marshal(resp)
		return json.Unmarshal(data, result)
	}
	return nil
}

type mockPFCP struct {
	sessions []pfcpSession
}

type pfcpSession struct {
	SEID uint64
	UEIP string
	TEID uint32
}

func (m *mockPFCP) EstablishSession(seid uint64, ueIP string, teid uint32) error {
	m.sessions = append(m.sessions, pfcpSession{SEID: seid, UEIP: ueIP, TEID: teid})
	return nil
}

func setup(t *testing.T) (*state.MockKVStore, *mockSBI, *mockPFCP) {
	t.Helper()
	ResetIPPool()
	store := state.NewMockKVStore()
	SetStore(store)
	// PCF returns SmPolicyDecision per TS 29.512
	sbi := &mockSBI{resp: map[string]interface{}{
		"pcf-policy-create": SmPolicyDecision{
			PolicyID: "pol-1",
			QFI:      1,       // TS 23.501 Section 5.7: default QoS flow
			AMBRUL:   1000000, // 1 Mbps
			AMBRDL:   5000000, // 5 Mbps
			FiveQI:   9,       // best effort internet
		},
	}}
	SetSBI(sbi)
	pfcpMock := &mockPFCP{}
	SetPFCP(pfcpMock)
	return store, sbi, pfcpMock
}

func TestHandle_CreatePDUSession(t *testing.T) {
	store, sbi, pfcpMock := setup(t)

	body, _ := json.Marshal(CreateSMContextRequest{
		SUPI:   "imsi-001010000000001",
		SNSSAI: models.SNSSAI{SST: 1, SD: "010203"},
		DNN:    "internet",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	var smResp CreateSMContextResponse
	if err := json.Unmarshal(resp.Body, &smResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if smResp.SessionID == "" {
		t.Error("session ID should not be empty")
	}
	if smResp.UEAddress != "10.45.0.1" {
		t.Errorf("UEAddress = %s, want 10.45.0.1", smResp.UEAddress)
	}
	if smResp.State != "ACTIVE" {
		t.Errorf("State = %s, want ACTIVE", smResp.State)
	}
	// TS 23.501 Section 5.7: default QFI=1 for best effort
	if smResp.QFI != 1 {
		t.Errorf("QFI = %d, want 1 (default QoS flow)", smResp.QFI)
	}
	if smResp.DNN != "internet" {
		t.Errorf("DNN = %s, want internet", smResp.DNN)
	}

	// Verify PCF was called (Npcf_SMPolicyControl_Create, TS 29.512)
	if len(sbi.calls) != 1 {
		t.Fatalf("expected 1 SBI call, got %d", len(sbi.calls))
	}
	if sbi.calls[0].FuncName != "pcf-policy-create" {
		t.Errorf("SBI call = %s, want pcf-policy-create", sbi.calls[0].FuncName)
	}

	// Verify PFCP session was established (N4, TS 29.244)
	if len(pfcpMock.sessions) != 1 {
		t.Fatalf("expected 1 PFCP session, got %d", len(pfcpMock.sessions))
	}
	if pfcpMock.sessions[0].UEIP != "10.45.0.1" {
		t.Errorf("PFCP UEIP = %s, want 10.45.0.1", pfcpMock.sessions[0].UEIP)
	}

	// Verify session stored in Redis
	var stored models.PDUSession
	if err := store.Get(context.Background(), "pdu-sessions/"+smResp.SessionID, &stored); err != nil {
		t.Fatalf("session not in store: %v", err)
	}
	if stored.SUPI != "imsi-001010000000001" {
		t.Errorf("stored SUPI = %s, want imsi-001010000000001", stored.SUPI)
	}
	if stored.State != "ACTIVE" {
		t.Errorf("stored State = %s, want ACTIVE", stored.State)
	}

	// Verify IP was recorded in Redis pool
	var allocatedIP string
	if err := store.Get(context.Background(), "ip-pool/allocated/10.45.0.1", &allocatedIP); err != nil {
		t.Fatal("allocated IP not tracked in Redis")
	}
}

func TestHandle_CreatePDUSession_NoDuplicateIPs(t *testing.T) {
	setup(t)

	seen := make(map[string]bool)
	for i := 0; i < 3; i++ {
		body, _ := json.Marshal(CreateSMContextRequest{
			SUPI:   "imsi-001010000000001",
			SNSSAI: models.SNSSAI{SST: 1, SD: "010203"},
			DNN:    "internet",
		})
		req := handler.Request{Method: "POST", Body: body}
		resp, err := Handle(req)
		if err != nil {
			t.Fatalf("Handle[%d] error: %v", i, err)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Handle[%d] status %d; body: %s", i, resp.StatusCode, resp.Body)
		}

		var smResp CreateSMContextResponse
		json.Unmarshal(resp.Body, &smResp)
		if seen[smResp.UEAddress] {
			t.Errorf("duplicate IP allocated: %s", smResp.UEAddress)
		}
		seen[smResp.UEAddress] = true
	}
	if len(seen) != 3 {
		t.Errorf("expected 3 unique IPs, got %d", len(seen))
	}
}

func TestHandle_CreatePDUSession_SubscriptionAMBROverride(t *testing.T) {
	store, _, _ := setup(t)

	body, _ := json.Marshal(CreateSMContextRequest{
		SUPI:          "imsi-001010000000001",
		SNSSAI:        models.SNSSAI{SST: 1, SD: "010203"},
		DNN:           "internet",
		SessionAMBRUL: 9999999,
		SessionAMBRDL: 8888888,
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, _ := Handle(req)

	var smResp CreateSMContextResponse
	json.Unmarshal(resp.Body, &smResp)

	if smResp.AMBRUL != 9999999 {
		t.Errorf("AMBRUL = %d, want 9999999 (subscription override)", smResp.AMBRUL)
	}
	if smResp.AMBRDL != 8888888 {
		t.Errorf("AMBRDL = %d, want 8888888 (subscription override)", smResp.AMBRDL)
	}

	// Verify stored values match
	var stored models.PDUSession
	store.Get(context.Background(), "pdu-sessions/"+smResp.SessionID, &stored)
	if stored.AMBRUL != 9999999 {
		t.Errorf("stored AMBRUL = %d, want 9999999", stored.AMBRUL)
	}
}

func TestHandle_CreatePDUSession_MissingSUPI(t *testing.T) {
	setup(t)

	body, _ := json.Marshal(CreateSMContextRequest{DNN: "internet"})
	req := handler.Request{Method: "POST", Body: body}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestHandle_CreatePDUSession_InvalidJSON(t *testing.T) {
	setup(t)

	req := handler.Request{Method: "POST", Body: []byte(`{invalid`)}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestHandle_CreatePDUSession_DefaultDNN(t *testing.T) {
	store, _, _ := setup(t)

	body, _ := json.Marshal(CreateSMContextRequest{
		SUPI:   "imsi-001010000000001",
		SNSSAI: models.SNSSAI{SST: 1},
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	var smResp CreateSMContextResponse
	json.Unmarshal(resp.Body, &smResp)

	var stored models.PDUSession
	store.Get(context.Background(), "pdu-sessions/"+smResp.SessionID, &stored)
	if stored.DNN != "internet" {
		t.Errorf("default DNN = %s, want internet", stored.DNN)
	}
	if stored.PDUType != "IPv4" {
		t.Errorf("default PDUType = %s, want IPv4", stored.PDUType)
	}
}
