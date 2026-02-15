package function

import (
	"encoding/json"
	"net/http"
	"testing"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/state"
)

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

func setup(t *testing.T) (*state.MockKVStore, *mockPFCP) {
	t.Helper()
	store := state.NewMockKVStore()
	SetStore(store)
	pfcpMock := &mockPFCP{}
	SetPFCP(pfcpMock)
	return store, pfcpMock
}

func TestHandle_N4SessionSetup(t *testing.T) {
	_, pfcpMock := setup(t)

	body, _ := json.Marshal(N4SetupRequest{
		SEID: 42,
		UEIP: "10.45.0.5",
		TEID: 100,
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var n4Resp N4SetupResponse
	if err := json.Unmarshal(resp.Body, &n4Resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if n4Resp.SEID != 42 {
		t.Errorf("SEID = %d, want 42", n4Resp.SEID)
	}
	if n4Resp.Status != "ESTABLISHED" {
		t.Errorf("Status = %s, want ESTABLISHED", n4Resp.Status)
	}

	// Verify PFCP session was established
	if len(pfcpMock.sessions) != 1 {
		t.Fatalf("expected 1 PFCP session, got %d", len(pfcpMock.sessions))
	}
	s := pfcpMock.sessions[0]
	if s.SEID != 42 {
		t.Errorf("PFCP SEID = %d, want 42", s.SEID)
	}
	if s.UEIP != "10.45.0.5" {
		t.Errorf("PFCP UEIP = %s, want 10.45.0.5", s.UEIP)
	}
	if s.TEID != 100 {
		t.Errorf("PFCP TEID = %d, want 100", s.TEID)
	}
}

func TestHandle_N4SessionSetup_MissingUEIP(t *testing.T) {
	setup(t)

	body, _ := json.Marshal(N4SetupRequest{SEID: 1})
	req := handler.Request{Method: "POST", Body: body}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestHandle_N4SessionSetup_MissingSEID(t *testing.T) {
	setup(t)

	body, _ := json.Marshal(N4SetupRequest{UEIP: "10.45.0.1"})
	req := handler.Request{Method: "POST", Body: body}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestHandle_N4SessionSetup_InvalidJSON(t *testing.T) {
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
