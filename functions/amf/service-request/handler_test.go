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

func TestHandle_ServiceRequest_Success(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	// Pre-populate: registered UE in CM-IDLE state
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

	var result ServiceResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.CmState != "CONNECTED" {
		t.Errorf("cm_state = %q, want CONNECTED", result.CmState)
	}
	if result.NASMessage == "" {
		t.Error("expected NAS Service Accept in response")
	}

	// Verify store was updated with CM-CONNECTED
	var updated models.UEContext
	if err := mockStore.Get(context.Background(), "ue:imsi-001010000000001", &updated); err != nil {
		t.Fatalf("get updated context: %v", err)
	}
	if updated.CmState != "CONNECTED" {
		t.Errorf("stored cm_state = %q, want CONNECTED", updated.CmState)
	}
}

func TestHandle_ServiceRequest_NotRegistered_CauseImplicit(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

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

	// Verify NAS Service Reject with cause code
	var result map[string]string
	json.Unmarshal(resp.Body, &result)
	if result["nas_message"] == "" {
		t.Error("expected nas_message with Service Reject")
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
