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

func TestHandle_GetSubscriberData(t *testing.T) {
	mock := setupMock(t)

	sub := models.SubscriberData{
		SUPI: "imsi-001010000000001",
		AccessAndMobility: &models.AccessMobData{
			NSSAI:      []models.SNSSAI{{SST: 1, SD: "010203"}},
			DefaultDNN: "internet",
		},
		SessionManagement: []models.SMPolicyData{
			{SNSSAI: models.SNSSAI{SST: 1, SD: "010203"}, DNN: "internet", QoSRef: 9},
		},
	}
	mock.Put(context.Background(), "subscribers/imsi-001010000000001", sub)

	req := handler.Request{
		Method:      "GET",
		QueryString: "supi=imsi-001010000000001",
	}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var result SubscriberDataResponse
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.SUPI != "imsi-001010000000001" {
		t.Errorf("SUPI = %s, want imsi-001010000000001", result.SUPI)
	}
	if result.AccessAndMobility == nil {
		t.Fatal("AccessAndMobility is nil")
	}
	if result.AccessAndMobility.DefaultDNN != "internet" {
		t.Errorf("DefaultDNN = %s, want internet", result.AccessAndMobility.DefaultDNN)
	}
	if len(result.SessionManagement) != 1 {
		t.Fatalf("SessionManagement length = %d, want 1", len(result.SessionManagement))
	}
	if result.SessionManagement[0].QoSRef != 9 {
		t.Errorf("QoSRef = %d, want 9", result.SessionManagement[0].QoSRef)
	}
}

func TestHandle_GetSubscriberData_NotFound(t *testing.T) {
	setupMock(t)

	req := handler.Request{
		Method:      "GET",
		QueryString: "supi=imsi-999999999999999",
	}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
}

func TestHandle_GetSubscriberData_MissingSUPI(t *testing.T) {
	setupMock(t)

	req := handler.Request{Method: "GET"}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}
