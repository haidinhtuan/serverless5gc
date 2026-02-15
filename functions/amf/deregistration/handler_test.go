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

func TestHandle_Deregistration_Success(t *testing.T) {
	mockStore := state.NewMockKVStore()
	SetStore(mockStore)

	// Pre-populate with a registered UE context
	ueCtx := models.UEContext{
		SUPI:              "imsi-001010000000001",
		RegistrationState: "REGISTERED",
		CmState:           "CONNECTED",
	}
	if err := mockStore.Put(context.Background(), "ue:imsi-001010000000001", ueCtx); err != nil {
		t.Fatalf("pre-populate store: %v", err)
	}

	body, _ := json.Marshal(DeregistrationRequest{SUPI: "imsi-001010000000001"})
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
	if result["status"] != "deregistered" {
		t.Errorf("status = %q, want deregistered", result["status"])
	}

	// Verify UE context was deleted
	var check models.UEContext
	if err := mockStore.Get(context.Background(), "ue:imsi-001010000000001", &check); err == nil {
		t.Fatal("expected UE context to be deleted from store")
	}
}

func TestHandle_Deregistration_NotFound(t *testing.T) {
	SetStore(state.NewMockKVStore())

	body, _ := json.Marshal(DeregistrationRequest{SUPI: "imsi-999"})
	resp, err := Handle(handler.Request{Body: body, Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestHandle_Deregistration_MissingSUPI(t *testing.T) {
	SetStore(state.NewMockKVStore())

	resp, err := Handle(handler.Request{Body: []byte(`{}`), Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandle_Deregistration_InvalidJSON(t *testing.T) {
	SetStore(state.NewMockKVStore())

	resp, err := Handle(handler.Request{Body: []byte(`bad`), Method: "POST"})
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}
