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
	// Reset counter between tests
	subMu.Lock()
	subCounter = 0
	subMu.Unlock()
	return mock
}

func TestHandle_Subscribe_NFLoad(t *testing.T) {
	mock := setupMock(t)

	ctx := context.Background()
	mock.Put(ctx, "nf-metrics/amf-001", models.NFLoadInfo{
		NFInstanceID: "amf-001",
		NFType:       "AMF",
		CPUUsage:     45.5,
		MemUsage:     60.2,
		NfLoad:       52,
		Timestamp:    time.Now(),
	})

	body, _ := json.Marshal(AnalyticsSubscribeRequest{
		EventID:  "NF_LOAD",
		TargetNF: "amf-001",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	var subResp AnalyticsSubscribeResponse
	if err := json.Unmarshal(resp.Body, &subResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if subResp.SubscriptionID != "nwdaf-sub-1" {
		t.Errorf("subscription_id = %q, want nwdaf-sub-1", subResp.SubscriptionID)
	}
	if subResp.EventID != "NF_LOAD" {
		t.Errorf("event_id = %q, want NF_LOAD", subResp.EventID)
	}
	if subResp.AnalyticsData == nil {
		t.Fatal("analytics_data is nil, want NFLoadInfo")
	}

	// Verify analytics data contains the pre-stored metrics
	dataBytes, _ := json.Marshal(subResp.AnalyticsData)
	var loadInfo models.NFLoadInfo
	if err := json.Unmarshal(dataBytes, &loadInfo); err != nil {
		t.Fatalf("unmarshal analytics_data: %v", err)
	}
	if loadInfo.NFInstanceID != "amf-001" {
		t.Errorf("nf_instance_id = %q, want amf-001", loadInfo.NFInstanceID)
	}
	if loadInfo.NfLoad != 52 {
		t.Errorf("nf_load = %d, want 52", loadInfo.NfLoad)
	}
}

func TestHandle_Subscribe_NFLoad_NoData(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(AnalyticsSubscribeRequest{
		EventID:  "NF_LOAD",
		TargetNF: "smf-999",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	var subResp AnalyticsSubscribeResponse
	json.Unmarshal(resp.Body, &subResp)
	if subResp.AnalyticsData == nil {
		t.Fatal("analytics_data is nil, want default NFLoadInfo")
	}

	dataBytes, _ := json.Marshal(subResp.AnalyticsData)
	var loadInfo models.NFLoadInfo
	json.Unmarshal(dataBytes, &loadInfo)
	if loadInfo.NFInstanceID != "smf-999" {
		t.Errorf("nf_instance_id = %q, want smf-999", loadInfo.NFInstanceID)
	}
	if loadInfo.NfLoad != 0 {
		t.Errorf("nf_load = %d, want 0 (default)", loadInfo.NfLoad)
	}
}

func TestHandle_Subscribe_SliceLoad(t *testing.T) {
	mock := setupMock(t)

	ctx := context.Background()
	mock.Put(ctx, "slice-metrics/1-010203", models.SliceLoadInfo{
		SST:             1,
		SD:              "010203",
		CurrentUEs:      120,
		CurrentSessions: 350,
		MeanNFLoad:      42.5,
		Timestamp:       time.Now(),
	})

	body, _ := json.Marshal(AnalyticsSubscribeRequest{
		EventID: "SLICE_LOAD",
		SNSSAI:  &models.SNSSAI{SST: 1, SD: "010203"},
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	var subResp AnalyticsSubscribeResponse
	json.Unmarshal(resp.Body, &subResp)
	if subResp.AnalyticsData == nil {
		t.Fatal("analytics_data is nil, want SliceLoadInfo")
	}

	dataBytes, _ := json.Marshal(subResp.AnalyticsData)
	var sliceInfo models.SliceLoadInfo
	json.Unmarshal(dataBytes, &sliceInfo)
	if sliceInfo.SST != 1 {
		t.Errorf("sst = %d, want 1", sliceInfo.SST)
	}
	if sliceInfo.CurrentUEs != 120 {
		t.Errorf("current_ues = %d, want 120", sliceInfo.CurrentUEs)
	}
	if sliceInfo.MeanNFLoad != 42.5 {
		t.Errorf("mean_nf_load = %f, want 42.5", sliceInfo.MeanNFLoad)
	}
}

func TestHandle_Subscribe_UEMobility(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(AnalyticsSubscribeRequest{
		EventID:         "UE_MOBILITY",
		NotificationURI: "http://amf:8080/notify",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	var subResp AnalyticsSubscribeResponse
	json.Unmarshal(resp.Body, &subResp)
	if subResp.EventID != "UE_MOBILITY" {
		t.Errorf("event_id = %q, want UE_MOBILITY", subResp.EventID)
	}
	if subResp.AnalyticsData != nil {
		t.Errorf("analytics_data = %v, want nil for UE_MOBILITY", subResp.AnalyticsData)
	}
	if subResp.SubscriptionID == "" {
		t.Error("subscription_id is empty")
	}

	// Verify the subscription was persisted
	var stored models.AnalyticsSubscription
	ctx := context.Background()
	key := "nwdaf-subscriptions/" + subResp.SubscriptionID
	if err := Store.Get(ctx, key, &stored); err != nil {
		t.Fatalf("subscription not stored: %v", err)
	}
	if stored.NotificationURI != "http://amf:8080/notify" {
		t.Errorf("notification_uri = %q, want http://amf:8080/notify", stored.NotificationURI)
	}
}

func TestHandle_Subscribe_InvalidEventID(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(AnalyticsSubscribeRequest{
		EventID: "INVALID_EVENT",
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

func TestHandle_Subscribe_InvalidJSON(t *testing.T) {
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
