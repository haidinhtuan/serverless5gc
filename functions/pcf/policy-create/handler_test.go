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

func TestHandle_PolicyCreate_DefaultEMBB(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(PolicyCreateRequest{
		SUPI:   "imsi-001010000000001",
		SNSSAI: models.SNSSAI{SST: 1, SD: "010203"},
		DNN:    "internet",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	var policy PolicyDecision
	if err := json.Unmarshal(resp.Body, &policy); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if policy.PolicyID == "" {
		t.Error("PolicyID should not be empty")
	}
	if policy.QFI != 9 {
		t.Errorf("QFI = %d, want 9 (eMBB default)", policy.QFI)
	}
	if policy.AMBRUL != 1000000 {
		t.Errorf("AMBRUL = %d, want 1000000", policy.AMBRUL)
	}
	if policy.AMBRDL != 5000000 {
		t.Errorf("AMBRDL = %d, want 5000000", policy.AMBRDL)
	}
}

func TestHandle_PolicyCreate_URLLC(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(PolicyCreateRequest{
		SUPI:   "imsi-001010000000001",
		SNSSAI: models.SNSSAI{SST: 2},
		DNN:    "internet",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	var policy PolicyDecision
	json.Unmarshal(resp.Body, &policy)
	if policy.QFI != 7 {
		t.Errorf("QFI = %d, want 7 (URLLC)", policy.QFI)
	}
	if policy.FiveQI != 7 {
		t.Errorf("5QI = %d, want 7", policy.FiveQI)
	}
}

func TestHandle_PolicyCreate_mMTC(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(PolicyCreateRequest{
		SUPI:   "imsi-001010000000001",
		SNSSAI: models.SNSSAI{SST: 3},
		DNN:    "iot",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201", resp.StatusCode)
	}

	var policy PolicyDecision
	json.Unmarshal(resp.Body, &policy)
	if policy.AMBRUL != 100000 {
		t.Errorf("AMBRUL = %d, want 100000 (mMTC)", policy.AMBRUL)
	}
}

func TestHandle_PolicyCreate_CustomPolicyFromStore(t *testing.T) {
	mock := setupMock(t)

	// Pre-configure a custom policy in the store
	customPolicy := PolicyDecision{
		QFI:    1,
		AMBRUL: 9999999,
		AMBRDL: 9999999,
		FiveQI: 1,
	}
	mock.Put(context.Background(), "policies/sst-1-sd-010203-dnn-enterprise", customPolicy)

	body, _ := json.Marshal(PolicyCreateRequest{
		SUPI:   "imsi-001010000000001",
		SNSSAI: models.SNSSAI{SST: 1, SD: "010203"},
		DNN:    "enterprise",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201", resp.StatusCode)
	}

	var policy PolicyDecision
	json.Unmarshal(resp.Body, &policy)
	if policy.AMBRUL != 9999999 {
		t.Errorf("AMBRUL = %d, want 9999999 (custom)", policy.AMBRUL)
	}
}

func TestHandle_PolicyCreate_StoredForRetrieval(t *testing.T) {
	mock := setupMock(t)

	body, _ := json.Marshal(PolicyCreateRequest{
		SUPI:   "imsi-001010000000001",
		SNSSAI: models.SNSSAI{SST: 1, SD: "010203"},
		DNN:    "internet",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, _ := Handle(req)

	var policy PolicyDecision
	json.Unmarshal(resp.Body, &policy)

	// Verify the policy was stored for later retrieval
	var stored PolicyDecision
	err := mock.Get(context.Background(), "active-policies/"+policy.PolicyID, &stored)
	if err != nil {
		t.Fatalf("active policy not stored: %v", err)
	}
	if stored.QFI != policy.QFI {
		t.Errorf("stored QFI = %d, want %d", stored.QFI, policy.QFI)
	}
}

func TestHandle_PolicyCreate_MissingSUPI(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(PolicyCreateRequest{DNN: "internet"})
	req := handler.Request{Method: "POST", Body: body}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestHandle_PolicyCreate_InvalidJSON(t *testing.T) {
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
