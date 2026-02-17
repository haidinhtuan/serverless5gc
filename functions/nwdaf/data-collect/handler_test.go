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

func TestHandle_DataCollect_NFLoad(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(DataCollectRequest{
		CollectType:  "NF_LOAD",
		NFInstanceID: "amf-001",
		NFType:       "AMF",
		CPUUsage:     55.3,
		MemUsage:     70.1,
		NfLoad:       62,
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var info models.NFLoadInfo
	if err := json.Unmarshal(resp.Body, &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if info.NFInstanceID != "amf-001" {
		t.Errorf("nf_instance_id = %q, want amf-001", info.NFInstanceID)
	}
	if info.NFType != "AMF" {
		t.Errorf("nf_type = %q, want AMF", info.NFType)
	}
	if info.CPUUsage != 55.3 {
		t.Errorf("cpu_usage = %f, want 55.3", info.CPUUsage)
	}
	if info.MemUsage != 70.1 {
		t.Errorf("mem_usage = %f, want 70.1", info.MemUsage)
	}
	if info.NfLoad != 62 {
		t.Errorf("nf_load = %d, want 62", info.NfLoad)
	}
	if info.Timestamp.IsZero() {
		t.Error("timestamp is zero, want non-zero")
	}

	// Verify stored in mock
	var stored models.NFLoadInfo
	ctx := context.Background()
	if err := Store.Get(ctx, "nf-metrics/amf-001", &stored); err != nil {
		t.Fatalf("metrics not stored: %v", err)
	}
	if stored.NfLoad != 62 {
		t.Errorf("stored nf_load = %d, want 62", stored.NfLoad)
	}
}

func TestHandle_DataCollect_NFLoad_DefaultType(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(DataCollectRequest{
		CollectType:  "NF_LOAD",
		NFInstanceID: "smf-002",
		CPUUsage:     30.0,
		NfLoad:       25,
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var info models.NFLoadInfo
	json.Unmarshal(resp.Body, &info)
	if info.NFType != "NF" {
		t.Errorf("nf_type = %q, want NF (default)", info.NFType)
	}
}

func TestHandle_DataCollect_NFLoad_MissingInstanceID(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(DataCollectRequest{
		CollectType: "NF_LOAD",
		NFType:      "AMF",
		CPUUsage:    50.0,
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

func TestHandle_DataCollect_SliceLoad(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(DataCollectRequest{
		CollectType: "SLICE_LOAD",
		SNSSAI:      &models.SNSSAI{SST: 1, SD: "010203"},
		NfLoad:      45,
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var info models.SliceLoadInfo
	if err := json.Unmarshal(resp.Body, &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if info.SST != 1 {
		t.Errorf("sst = %d, want 1", info.SST)
	}
	if info.SD != "010203" {
		t.Errorf("sd = %q, want 010203", info.SD)
	}
	if info.MeanNFLoad != 45.0 {
		t.Errorf("mean_nf_load = %f, want 45.0", info.MeanNFLoad)
	}
	// No NSACF counters pre-stored, so UE/session counts should be 0
	if info.CurrentUEs != 0 {
		t.Errorf("current_ues = %d, want 0", info.CurrentUEs)
	}
	if info.CurrentSessions != 0 {
		t.Errorf("current_sessions = %d, want 0", info.CurrentSessions)
	}
	if info.Timestamp.IsZero() {
		t.Error("timestamp is zero, want non-zero")
	}
}

func TestHandle_DataCollect_SliceLoad_WithCounters(t *testing.T) {
	mock := setupMock(t)

	ctx := context.Background()
	mock.Put(ctx, "nsacf-counters/1-010203", models.SliceCounters{
		SST:             1,
		SD:              "010203",
		CurrentUEs:      85,
		CurrentSessions: 210,
	})

	body, _ := json.Marshal(DataCollectRequest{
		CollectType: "SLICE_LOAD",
		SNSSAI:      &models.SNSSAI{SST: 1, SD: "010203"},
		NfLoad:      38,
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var info models.SliceLoadInfo
	json.Unmarshal(resp.Body, &info)
	if info.CurrentUEs != 85 {
		t.Errorf("current_ues = %d, want 85 (from nsacf counters)", info.CurrentUEs)
	}
	if info.CurrentSessions != 210 {
		t.Errorf("current_sessions = %d, want 210 (from nsacf counters)", info.CurrentSessions)
	}
	if info.MeanNFLoad != 38.0 {
		t.Errorf("mean_nf_load = %f, want 38.0", info.MeanNFLoad)
	}

	// Verify the metrics were stored
	var stored models.SliceLoadInfo
	if err := Store.Get(ctx, "slice-metrics/1-010203", &stored); err != nil {
		t.Fatalf("slice metrics not stored: %v", err)
	}
	if stored.CurrentUEs != 85 {
		t.Errorf("stored current_ues = %d, want 85", stored.CurrentUEs)
	}
}

func TestHandle_DataCollect_SliceLoad_NoSD(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(DataCollectRequest{
		CollectType: "SLICE_LOAD",
		SNSSAI:      &models.SNSSAI{SST: 2},
		NfLoad:      20,
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	// Verify stored under key without SD
	var stored models.SliceLoadInfo
	ctx := context.Background()
	if err := Store.Get(ctx, "slice-metrics/2", &stored); err != nil {
		t.Fatalf("metrics not stored under slice-metrics/2: %v", err)
	}
	if stored.SST != 2 {
		t.Errorf("stored sst = %d, want 2", stored.SST)
	}
}

func TestHandle_DataCollect_SliceLoad_MissingSNSSAI(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(DataCollectRequest{
		CollectType: "SLICE_LOAD",
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

func TestHandle_DataCollect_InvalidCollectType(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(DataCollectRequest{
		CollectType: "INVALID",
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

func TestHandle_DataCollect_InvalidJSON(t *testing.T) {
	setupMock(t)

	req := handler.Request{Method: "POST", Body: []byte(`{bad`)}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}
