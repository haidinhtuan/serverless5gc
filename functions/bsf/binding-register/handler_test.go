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

func setupMock(t *testing.T) *state.MockKVStore {
	t.Helper()
	mock := state.NewMockKVStore()
	SetStore(mock)
	return mock
}

func TestHandle_BindingRegister_Success(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_BSF", "true")

	body, _ := json.Marshal(BindingRegisterRequest{
		SUPI:         "imsi-001010000000001",
		DNN:          "internet",
		SNSSAI:       models.SNSSAI{SST: 1, SD: "010203"},
		PCFAddress:   "pcf-001",
		PDUSessionID: "pdu-1",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	var binding models.PCFBinding
	if err := json.Unmarshal(resp.Body, &binding); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if binding.BindingID == "" {
		t.Error("BindingID should not be empty")
	}
	if binding.SUPI != "imsi-001010000000001" {
		t.Errorf("SUPI = %s, want imsi-001010000000001", binding.SUPI)
	}
	if binding.DNN != "internet" {
		t.Errorf("DNN = %s, want internet", binding.DNN)
	}

	// Verify primary key stored
	var stored models.PCFBinding
	if err := mock.Get(context.Background(), "bsf-bindings/"+binding.BindingID, &stored); err != nil {
		t.Fatalf("primary key not stored: %v", err)
	}
	if stored.BindingID != binding.BindingID {
		t.Errorf("stored BindingID = %s, want %s", stored.BindingID, binding.BindingID)
	}

	// Verify SUPI+DNN index stored
	var indexID string
	if err := mock.Get(context.Background(), "bsf-by-supi/imsi-001010000000001/internet", &indexID); err != nil {
		t.Fatalf("supi index not stored: %v", err)
	}
	if indexID != binding.BindingID {
		t.Errorf("supi index = %s, want %s", indexID, binding.BindingID)
	}
}

func TestHandle_BindingRegister_WithUEAddress(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_BSF", "true")

	body, _ := json.Marshal(BindingRegisterRequest{
		SUPI:         "imsi-001010000000002",
		DNN:          "internet",
		SNSSAI:       models.SNSSAI{SST: 1},
		PCFAddress:   "pcf-002",
		UEAddress:    "10.60.0.1",
		PDUSessionID: "pdu-2",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	var binding models.PCFBinding
	json.Unmarshal(resp.Body, &binding)

	// Verify IP index stored
	var indexID string
	if err := mock.Get(context.Background(), "bsf-by-ip/10.60.0.1", &indexID); err != nil {
		t.Fatalf("ip index not stored: %v", err)
	}
	if indexID != binding.BindingID {
		t.Errorf("ip index = %s, want %s", indexID, binding.BindingID)
	}
}

func TestHandle_BindingRegister_DefaultDNN(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_BSF", "true")

	body, _ := json.Marshal(BindingRegisterRequest{
		SUPI:         "imsi-001010000000003",
		SNSSAI:       models.SNSSAI{SST: 1},
		PCFAddress:   "pcf-001",
		PDUSessionID: "pdu-3",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	var binding models.PCFBinding
	json.Unmarshal(resp.Body, &binding)
	if binding.DNN != "internet" {
		t.Errorf("DNN = %s, want internet (default)", binding.DNN)
	}
}

func TestHandle_BindingRegister_Disabled(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_BSF", "false")

	body, _ := json.Marshal(BindingRegisterRequest{
		SUPI:         "imsi-001010000000001",
		PDUSessionID: "pdu-1",
	})

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

func TestHandle_BindingRegister_MissingSUPI(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_BSF", "true")

	body, _ := json.Marshal(BindingRegisterRequest{
		PDUSessionID: "pdu-1",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400; body: %s", resp.StatusCode, resp.Body)
	}
}

func TestHandle_BindingRegister_InvalidJSON(t *testing.T) {
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
