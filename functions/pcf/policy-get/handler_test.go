package function

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/haidinhtuan/serverless5gc/pkg/state"
)

func setupMock(t *testing.T) *state.MockKVStore {
	t.Helper()
	mock := state.NewMockKVStore()
	SetStore(mock)
	return mock
}

func seedPolicy(t *testing.T, mock *state.MockKVStore) {
	t.Helper()
	policy := PolicyDecision{
		PolicyID: "pol-imsi-001010000000001-1",
		QFI:      9,
		AMBRUL:   1000000,
		AMBRDL:   5000000,
		FiveQI:   9,
	}
	if err := mock.Put(context.Background(), "active-policies/pol-imsi-001010000000001-1", policy); err != nil {
		t.Fatalf("seed policy: %v", err)
	}
}

func TestHandle_PolicyGet_ByQuery(t *testing.T) {
	mock := setupMock(t)
	seedPolicy(t, mock)

	req := handler.Request{
		Method:      "GET",
		QueryString: "policy_id=pol-imsi-001010000000001-1",
	}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var policy PolicyDecision
	if err := json.Unmarshal(resp.Body, &policy); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if policy.PolicyID != "pol-imsi-001010000000001-1" {
		t.Errorf("PolicyID = %s, want pol-imsi-001010000000001-1", policy.PolicyID)
	}
	if policy.QFI != 9 {
		t.Errorf("QFI = %d, want 9", policy.QFI)
	}
	if policy.AMBRUL != 1000000 {
		t.Errorf("AMBRUL = %d, want 1000000", policy.AMBRUL)
	}
}

func TestHandle_PolicyGet_ByBody(t *testing.T) {
	mock := setupMock(t)
	seedPolicy(t, mock)

	body, _ := json.Marshal(map[string]string{"policy_id": "pol-imsi-001010000000001-1"})
	req := handler.Request{Method: "POST", Body: body}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var policy PolicyDecision
	json.Unmarshal(resp.Body, &policy)
	if policy.AMBRDL != 5000000 {
		t.Errorf("AMBRDL = %d, want 5000000", policy.AMBRDL)
	}
}

func TestHandle_PolicyGet_NotFound(t *testing.T) {
	setupMock(t)

	req := handler.Request{
		Method:      "GET",
		QueryString: "policy_id=pol-nonexistent-1",
	}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
}

func TestHandle_PolicyGet_MissingPolicyID(t *testing.T) {
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
