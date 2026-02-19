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

func TestHandle_BindingDiscover_ByIP(t *testing.T) {
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

	body, _ := json.Marshal(BindingDiscoverRequest{UEAddress: "10.60.0.1"})
	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var binding models.PCFBinding
	if err := json.Unmarshal(resp.Body, &binding); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if binding.BindingID != "bsf-test-1" {
		t.Errorf("BindingID = %s, want bsf-test-1", binding.BindingID)
	}
	if binding.PCFAddress != "pcf-001" {
		t.Errorf("PCFAddress = %s, want pcf-001", binding.PCFAddress)
	}
}

func TestHandle_BindingDiscover_BySUPI(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_BSF", "true")

	seedBinding(t, mock, models.PCFBinding{
		BindingID:    "bsf-test-2",
		SUPI:         "imsi-001010000000002",
		DNN:          "enterprise",
		SNSSAI:       models.SNSSAI{SST: 1, SD: "010203"},
		PCFAddress:   "pcf-002",
		PDUSessionID: "pdu-2",
	})

	body, _ := json.Marshal(BindingDiscoverRequest{SUPI: "imsi-001010000000002", DNN: "enterprise"})
	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var binding models.PCFBinding
	json.Unmarshal(resp.Body, &binding)
	if binding.BindingID != "bsf-test-2" {
		t.Errorf("BindingID = %s, want bsf-test-2", binding.BindingID)
	}
	if binding.DNN != "enterprise" {
		t.Errorf("DNN = %s, want enterprise", binding.DNN)
	}
}

func TestHandle_BindingDiscover_ByIPNotFound(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_BSF", "true")

	body, _ := json.Marshal(BindingDiscoverRequest{UEAddress: "10.60.0.99"})
	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404; body: %s", resp.StatusCode, resp.Body)
	}
}

func TestHandle_BindingDiscover_BySUPINotFound(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_BSF", "true")

	body, _ := json.Marshal(BindingDiscoverRequest{SUPI: "imsi-999999999999999", DNN: "internet"})
	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404; body: %s", resp.StatusCode, resp.Body)
	}
}

func TestHandle_BindingDiscover_NoParams(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_BSF", "true")

	body, _ := json.Marshal(BindingDiscoverRequest{})
	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400; body: %s", resp.StatusCode, resp.Body)
	}
}

func TestHandle_BindingDiscover_Disabled(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_BSF", "false")

	body, _ := json.Marshal(BindingDiscoverRequest{UEAddress: "10.60.0.1"})
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

func TestHandle_BindingDiscover_InvalidJSON(t *testing.T) {
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
