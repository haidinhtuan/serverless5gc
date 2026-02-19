package function

import (
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

func TestHandle_ChargingCreate_Success(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_CHARGING", "true")

	body, _ := json.Marshal(ChargingCreateRequest{
		SUPI:         "imsi-001010000000001",
		PDUSessionID: "pdu-1",
		DNN:          "internet",
		SNSSAI:       models.SNSSAI{SST: 1, SD: "010203"},
		ChargingType: "OFFLINE",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	var session models.ChargingSession
	if err := json.Unmarshal(resp.Body, &session); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if session.ChargingID == "" {
		t.Error("ChargingID should not be empty")
	}
	if session.SUPI != "imsi-001010000000001" {
		t.Errorf("SUPI = %s, want imsi-001010000000001", session.SUPI)
	}
	if session.State != "ACTIVE" {
		t.Errorf("State = %s, want ACTIVE", session.State)
	}
	if session.GrantedUnits != 1000000 {
		t.Errorf("GrantedUnits = %d, want 1000000", session.GrantedUnits)
	}
	if session.ChargingType != "OFFLINE" {
		t.Errorf("ChargingType = %s, want OFFLINE", session.ChargingType)
	}
}

func TestHandle_ChargingCreate_DefaultOffline(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_CHARGING", "true")

	body, _ := json.Marshal(ChargingCreateRequest{
		SUPI:         "imsi-001010000000001",
		PDUSessionID: "pdu-1",
		DNN:          "internet",
		SNSSAI:       models.SNSSAI{SST: 1},
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	var session models.ChargingSession
	json.Unmarshal(resp.Body, &session)
	if session.ChargingType != "OFFLINE" {
		t.Errorf("ChargingType = %s, want OFFLINE (default)", session.ChargingType)
	}
}

func TestHandle_ChargingCreate_Online(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_CHARGING", "true")

	body, _ := json.Marshal(ChargingCreateRequest{
		SUPI:         "imsi-001010000000001",
		PDUSessionID: "pdu-2",
		DNN:          "internet",
		SNSSAI:       models.SNSSAI{SST: 1},
		ChargingType: "ONLINE",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	var session models.ChargingSession
	json.Unmarshal(resp.Body, &session)
	if session.ChargingType != "ONLINE" {
		t.Errorf("ChargingType = %s, want ONLINE", session.ChargingType)
	}
}

func TestHandle_ChargingCreate_Disabled(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(ChargingCreateRequest{
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

func TestHandle_ChargingCreate_MissingSUPI(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_CHARGING", "true")

	body, _ := json.Marshal(ChargingCreateRequest{
		PDUSessionID: "pdu-1",
		DNN:          "internet",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestHandle_ChargingCreate_InvalidJSON(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_CHARGING", "true")

	req := handler.Request{Method: "POST", Body: []byte(`{bad`)}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}
