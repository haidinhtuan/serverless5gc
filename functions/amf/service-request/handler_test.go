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

func TestHandle_ServiceRequest_Success(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	// Pre-populate: registered UE in IDLE state
	ueCtx := models.UEContext{
		SUPI:              "imsi-001010000000001",
		RegistrationState: "REGISTERED",
		CmState:           "IDLE",
	}
	if err := mockStore.Put(context.Background(), "ue:imsi-001010000000001", ueCtx); err != nil {
		t.Fatalf("setup: %v", err)
	}

	body, _ := json.Marshal(ServiceRequest{SUPI: "imsi-001010000000001"})
	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, http.StatusOK, resp.Body)
	}

	var result map[string]string
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["cm_state"] != "CONNECTED" {
		t.Errorf("cm_state = %q, want CONNECTED", result["cm_state"])
	}

	// Verify store was updated
	var updated models.UEContext
	if err := mockStore.Get(context.Background(), "ue:imsi-001010000000001", &updated); err != nil {
		t.Fatalf("get updated context: %v", err)
	}
	if updated.CmState != "CONNECTED" {
		t.Errorf("stored cm_state = %q, want CONNECTED", updated.CmState)
	}
}

func TestHandle_ServiceRequest_NotRegistered(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	// UE exists but is deregistered
	ueCtx := models.UEContext{
		SUPI:              "imsi-001010000000001",
		RegistrationState: "DEREGISTERED",
		CmState:           "IDLE",
	}
	mockStore.Put(context.Background(), "ue:imsi-001010000000001", ueCtx)

	body, _ := json.Marshal(ServiceRequest{SUPI: "imsi-001010000000001"})
	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

func TestHandle_ServiceRequest_NotFound(t *testing.T) {
	SetStore(state.NewMockKVStore())

	body, _ := json.Marshal(ServiceRequest{SUPI: "imsi-unknown"})
	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHandle_ServiceRequest_MissingSUPI(t *testing.T) {
	SetStore(state.NewMockKVStore())

	resp, err := Handle(handler.Request{Body: []byte(`{}`), Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}
