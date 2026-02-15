package function

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/models"
	"github.com/tdinh/serverless5gc/pkg/state"
)

func setup(t *testing.T) *state.MockKVStore {
	t.Helper()
	ms := state.NewMockKVStore()
	SetStore(ms)
	return ms
}

func makeReq(body string) handler.Request {
	return handler.Request{Body: []byte(body)}
}

func TestHandover_Success(t *testing.T) {
	ms := setup(t)

	ue := models.UEContext{
		SUPI:              "imsi-001010000000001",
		RegistrationState: "REGISTERED",
		CmState:           "CONNECTED",
		GnbID:             "gnb-001",
		LastActivity:      time.Now(),
	}
	ms.Put(nil, "ue:imsi-001010000000001", ue)

	resp, err := Handle(makeReq(`{"supi":"imsi-001010000000001","target_gnb_id":"gnb-002"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", resp.StatusCode, http.StatusOK, resp.Body)
	}

	var result HandoverResponse
	json.Unmarshal(resp.Body, &result)
	if result.SourceGnbID != "gnb-001" {
		t.Errorf("SourceGnbID = %q, want %q", result.SourceGnbID, "gnb-001")
	}
	if result.TargetGnbID != "gnb-002" {
		t.Errorf("TargetGnbID = %q, want %q", result.TargetGnbID, "gnb-002")
	}

	// Verify UE context updated.
	var updated models.UEContext
	ms.Get(nil, "ue:imsi-001010000000001", &updated)
	if updated.GnbID != "gnb-002" {
		t.Errorf("GnbID after handover = %q, want %q", updated.GnbID, "gnb-002")
	}
}

func TestHandover_MissingFields(t *testing.T) {
	setup(t)

	resp, _ := Handle(makeReq(`{"supi":"imsi-001010000000001"}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandover_UENotFound(t *testing.T) {
	setup(t)

	resp, _ := Handle(makeReq(`{"supi":"imsi-999","target_gnb_id":"gnb-002"}`))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHandover_NotRegistered(t *testing.T) {
	ms := setup(t)

	ue := models.UEContext{
		SUPI:              "imsi-001010000000001",
		RegistrationState: "DEREGISTERED",
		CmState:           "CONNECTED",
		GnbID:             "gnb-001",
	}
	ms.Put(nil, "ue:imsi-001010000000001", ue)

	resp, _ := Handle(makeReq(`{"supi":"imsi-001010000000001","target_gnb_id":"gnb-002"}`))
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

func TestHandover_NotConnected(t *testing.T) {
	ms := setup(t)

	ue := models.UEContext{
		SUPI:              "imsi-001010000000001",
		RegistrationState: "REGISTERED",
		CmState:           "IDLE",
		GnbID:             "gnb-001",
	}
	ms.Put(nil, "ue:imsi-001010000000001", ue)

	resp, _ := Handle(makeReq(`{"supi":"imsi-001010000000001","target_gnb_id":"gnb-002"}`))
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}
