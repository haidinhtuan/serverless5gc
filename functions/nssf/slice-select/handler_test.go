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

func TestHandle_SliceSelect_AllAllowed(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(SliceSelectRequest{
		RequestedNSSAI: []models.SNSSAI{
			{SST: 1, SD: "010203"},
		},
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var ssResp SliceSelectResponse
	if err := json.Unmarshal(resp.Body, &ssResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(ssResp.AllowedNSSAI) != 1 {
		t.Fatalf("allowed count = %d, want 1", len(ssResp.AllowedNSSAI))
	}
	if ssResp.AllowedNSSAI[0].SST != 1 || ssResp.AllowedNSSAI[0].SD != "010203" {
		t.Errorf("allowed[0] = %+v, want SST=1 SD=010203", ssResp.AllowedNSSAI[0])
	}
}

func TestHandle_SliceSelect_MultipleSlices(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(SliceSelectRequest{
		RequestedNSSAI: []models.SNSSAI{
			{SST: 1, SD: "010203"},
			{SST: 2, SD: "010203"},
			{SST: 3, SD: "010203"},
		},
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	var ssResp SliceSelectResponse
	json.Unmarshal(resp.Body, &ssResp)
	if len(ssResp.AllowedNSSAI) != 3 {
		t.Errorf("allowed count = %d, want 3", len(ssResp.AllowedNSSAI))
	}
	if len(ssResp.RejectedNSSAI) != 0 {
		t.Errorf("rejected count = %d, want 0", len(ssResp.RejectedNSSAI))
	}
}

func TestHandle_SliceSelect_SomeRejected(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(SliceSelectRequest{
		RequestedNSSAI: []models.SNSSAI{
			{SST: 1, SD: "010203"},  // allowed
			{SST: 99, SD: "FFFFFF"}, // not configured
		},
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	var ssResp SliceSelectResponse
	json.Unmarshal(resp.Body, &ssResp)
	if len(ssResp.AllowedNSSAI) != 1 {
		t.Errorf("allowed count = %d, want 1", len(ssResp.AllowedNSSAI))
	}
	if len(ssResp.RejectedNSSAI) != 1 {
		t.Errorf("rejected count = %d, want 1", len(ssResp.RejectedNSSAI))
	}
	if ssResp.RejectedNSSAI[0].SST != 99 {
		t.Errorf("rejected[0].SST = %d, want 99", ssResp.RejectedNSSAI[0].SST)
	}
}

func TestHandle_SliceSelect_AllRejected(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(SliceSelectRequest{
		RequestedNSSAI: []models.SNSSAI{
			{SST: 99, SD: "AAAAAA"},
		},
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	var ssResp SliceSelectResponse
	json.Unmarshal(resp.Body, &ssResp)
	if len(ssResp.AllowedNSSAI) != 0 {
		t.Errorf("allowed count = %d, want 0", len(ssResp.AllowedNSSAI))
	}
	if len(ssResp.RejectedNSSAI) != 1 {
		t.Errorf("rejected count = %d, want 1", len(ssResp.RejectedNSSAI))
	}
}

func TestHandle_SliceSelect_CustomConfigFromStore(t *testing.T) {
	mock := setupMock(t)

	// Configure custom slices in store
	customSlices := []models.SNSSAI{
		{SST: 5, SD: "CUSTOM"},
	}
	mock.Put(context.Background(), "nssf/configured-slices", customSlices)

	body, _ := json.Marshal(SliceSelectRequest{
		RequestedNSSAI: []models.SNSSAI{
			{SST: 5, SD: "CUSTOM"},  // allowed (custom)
			{SST: 1, SD: "010203"},  // rejected (not in custom config)
		},
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	var ssResp SliceSelectResponse
	json.Unmarshal(resp.Body, &ssResp)
	if len(ssResp.AllowedNSSAI) != 1 {
		t.Fatalf("allowed count = %d, want 1", len(ssResp.AllowedNSSAI))
	}
	if ssResp.AllowedNSSAI[0].SST != 5 {
		t.Errorf("allowed[0].SST = %d, want 5", ssResp.AllowedNSSAI[0].SST)
	}
	if len(ssResp.RejectedNSSAI) != 1 {
		t.Fatalf("rejected count = %d, want 1", len(ssResp.RejectedNSSAI))
	}
}

func TestHandle_SliceSelect_EmptyRequest(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(SliceSelectRequest{
		RequestedNSSAI: []models.SNSSAI{},
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

func TestHandle_SliceSelect_InvalidJSON(t *testing.T) {
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
