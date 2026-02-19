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

func setupMock(t *testing.T) *state.MockKVStore {
	t.Helper()
	mock := state.NewMockKVStore()
	SetStore(mock)
	return mock
}

// seedBinding creates a PCFBinding and its indexes in the mock store.
func seedBinding(t *testing.T, mock *state.MockKVStore, binding models.PCFBinding) {
	t.Helper()
	ctx := context.Background()
	mock.Put(ctx, "bsf-bindings/"+binding.BindingID, binding)
	if binding.UEAddress != "" {
		mock.Put(ctx, "bsf-by-ip/"+binding.UEAddress, binding.BindingID)
	}
	mock.Put(ctx, "bsf-by-supi/"+binding.SUPI+"/"+binding.DNN, binding.BindingID)
}

func TestHandle_BindingDeregister_Success(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_BSF", "true")

	seedBinding(t, mock, models.PCFBinding{
		BindingID:    "bsf-test-1",
		SUPI:         "imsi-001010000000001",
		DNN:          "internet",
		SNSSAI:       models.SNSSAI{SST: 1},
		PCFAddress:   "pcf-001",
		UEAddress:    "10.60.0.1",
		PDUSessionID: "pdu-1",
	})

	body, _ := json.Marshal(BindingDeregisterRequest{BindingID: "bsf-test-1"})
	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var result map[string]string
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["binding_id"] != "bsf-test-1" {
		t.Errorf("binding_id = %s, want bsf-test-1", result["binding_id"])
	}
	if result["status"] != "deregistered" {
		t.Errorf("status = %s, want deregistered", result["status"])
	}

	ctx := context.Background()

	// Verify primary key deleted
	var stored models.PCFBinding
	if err := mock.Get(ctx, "bsf-bindings/bsf-test-1", &stored); err == nil {
		t.Error("primary key should have been deleted")
	}

	// Verify IP index deleted
	var indexID string
	if err := mock.Get(ctx, "bsf-by-ip/10.60.0.1", &indexID); err == nil {
		t.Error("ip index should have been deleted")
	}

	// Verify SUPI+DNN index deleted
	if err := mock.Get(ctx, "bsf-by-supi/imsi-001010000000001/internet", &indexID); err == nil {
		t.Error("supi index should have been deleted")
	}
}

func TestHandle_BindingDeregister_NotFound(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_BSF", "true")

	body, _ := json.Marshal(BindingDeregisterRequest{BindingID: "bsf-nonexistent"})
	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404; body: %s", resp.StatusCode, resp.Body)
	}
}

func TestHandle_BindingDeregister_Disabled(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_BSF", "false")

	body, _ := json.Marshal(BindingDeregisterRequest{BindingID: "bsf-test-1"})
	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var result map[string]string
	json.Unmarshal(resp.Body, &result)
	if result["status"] != "disabled" {
		t.Errorf("status = %s, want disabled", result["status"])
	}
}

func TestHandle_BindingDeregister_InvalidJSON(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_BSF", "true")

	req := handler.Request{Method: "POST", Body: []byte(`{bad`)}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}
