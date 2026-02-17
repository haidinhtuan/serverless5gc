package function

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

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

func seedSession(t *testing.T, mock *state.MockKVStore, id string, granted uint64) {
	t.Helper()
	session := models.ChargingSession{
		ChargingID:   id,
		SUPI:         "imsi-001010000000001",
		PDUSessionID: "pdu-1",
		DNN:          "internet",
		SNSSAI:       models.SNSSAI{SST: 1, SD: "010203"},
		ChargingType: "OFFLINE",
		State:        "ACTIVE",
		GrantedUnits: granted,
		CreatedAt:    time.Now(),
		LastUpdated:  time.Now(),
	}
	if err := mock.Put(context.Background(), "charging-sessions/"+id, session); err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func TestHandle_ChargingUpdate_Success(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_CHARGING", "true")

	seedSession(t, mock, "chg-test-1", 1000000)

	body, _ := json.Marshal(ChargingUpdateRequest{
		ChargingID:     "chg-test-1",
		VolumeUplink:   100000,
		VolumeDownlink: 200000,
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var result ChargingUpdateResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.VolumeUplink != 100000 {
		t.Errorf("VolumeUplink = %d, want 100000", result.VolumeUplink)
	}
	if result.VolumeDownlink != 200000 {
		t.Errorf("VolumeDownlink = %d, want 200000", result.VolumeDownlink)
	}
	if result.AdditionalQuotaGranted {
		t.Error("AdditionalQuotaGranted should be false when usage is within limits")
	}
	if result.GrantedUnits != 1000000 {
		t.Errorf("GrantedUnits = %d, want 1000000", result.GrantedUnits)
	}
}

func TestHandle_ChargingUpdate_AdditionalQuota(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_CHARGING", "true")

	seedSession(t, mock, "chg-test-2", 1000000)

	body, _ := json.Marshal(ChargingUpdateRequest{
		ChargingID:     "chg-test-2",
		VolumeUplink:   600000,
		VolumeDownlink: 500000,
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var result ChargingUpdateResponse
	json.Unmarshal(resp.Body, &result)
	if !result.AdditionalQuotaGranted {
		t.Error("AdditionalQuotaGranted should be true when usage exceeds granted units")
	}
	if result.GrantedUnits != 2000000 {
		t.Errorf("GrantedUnits = %d, want 2000000 (original + increment)", result.GrantedUnits)
	}
	if result.VolumeUplink != 600000 {
		t.Errorf("VolumeUplink = %d, want 600000", result.VolumeUplink)
	}
	if result.VolumeDownlink != 500000 {
		t.Errorf("VolumeDownlink = %d, want 500000", result.VolumeDownlink)
	}
}

func TestHandle_ChargingUpdate_NotFound(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_CHARGING", "true")

	body, _ := json.Marshal(ChargingUpdateRequest{
		ChargingID:     "chg-nonexistent",
		VolumeUplink:   100,
		VolumeDownlink: 200,
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404; body: %s", resp.StatusCode, resp.Body)
	}
}

func TestHandle_ChargingUpdate_Disabled(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(ChargingUpdateRequest{
		ChargingID: "chg-test-1",
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

func TestHandle_ChargingUpdate_InvalidJSON(t *testing.T) {
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
