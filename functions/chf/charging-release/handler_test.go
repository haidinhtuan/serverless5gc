package function

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

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

func seedSession(t *testing.T, mock *state.MockKVStore, id string) {
	t.Helper()
	session := models.ChargingSession{
		ChargingID:     id,
		SUPI:           "imsi-001010000000001",
		PDUSessionID:   "pdu-1",
		DNN:            "internet",
		SNSSAI:         models.SNSSAI{SST: 1, SD: "010203"},
		ChargingType:   "OFFLINE",
		State:          "ACTIVE",
		VolumeUplink:   500000,
		VolumeDownlink: 300000,
		GrantedUnits:   1000000,
		CreatedAt:      time.Now().Add(-10 * time.Minute),
		LastUpdated:    time.Now(),
	}
	if err := mock.Put(context.Background(), "charging-sessions/"+id, session); err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func TestHandle_ChargingRelease_Success(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_CHARGING", "true")

	seedSession(t, mock, "chg-test-1")

	body, _ := json.Marshal(ChargingReleaseRequest{
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

	var cdr models.ChargingDataRecord
	if err := json.Unmarshal(resp.Body, &cdr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cdr.RecordID != "cdr-chg-test-1" {
		t.Errorf("RecordID = %s, want cdr-chg-test-1", cdr.RecordID)
	}
	if cdr.ChargingID != "chg-test-1" {
		t.Errorf("ChargingID = %s, want chg-test-1", cdr.ChargingID)
	}
	if cdr.SUPI != "imsi-001010000000001" {
		t.Errorf("SUPI = %s, want imsi-001010000000001", cdr.SUPI)
	}
	if cdr.VolumeUplink != 500000 {
		t.Errorf("VolumeUplink = %d, want 500000", cdr.VolumeUplink)
	}
	if cdr.VolumeDownlink != 300000 {
		t.Errorf("VolumeDownlink = %d, want 300000", cdr.VolumeDownlink)
	}
	if cdr.Duration < 0 {
		t.Errorf("Duration = %d, should be non-negative", cdr.Duration)
	}

	// Verify CDR was stored
	var storedCDR models.ChargingDataRecord
	if err := mock.Get(context.Background(), "charging-records/cdr-chg-test-1", &storedCDR); err != nil {
		t.Fatalf("CDR not stored: %v", err)
	}
	if storedCDR.RecordID != "cdr-chg-test-1" {
		t.Errorf("stored RecordID = %s, want cdr-chg-test-1", storedCDR.RecordID)
	}

	// Verify charging session was deleted
	var deleted models.ChargingSession
	if err := mock.Get(context.Background(), "charging-sessions/chg-test-1", &deleted); err == nil {
		t.Error("charging session should have been deleted")
	}
}

func TestHandle_ChargingRelease_NotFound(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_CHARGING", "true")

	body, _ := json.Marshal(ChargingReleaseRequest{
		ChargingID: "chg-nonexistent",
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

func TestHandle_ChargingRelease_Disabled(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(ChargingReleaseRequest{
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

func TestHandle_ChargingRelease_InvalidJSON(t *testing.T) {
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
