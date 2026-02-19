package function

import (
	"context"
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

func TestHandle_Check_AllowedWhenNoPolicy(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_NSACF", "true")

	body, _ := json.Marshal(AvailabilityCheckRequest{
		SNSSAI:    models.SNSSAI{SST: 1, SD: "010203"},
		CheckType: "UE",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var checkResp AvailabilityCheckResponse
	if err := json.Unmarshal(resp.Body, &checkResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !checkResp.Allowed {
		t.Errorf("allowed = false, want true (no policy should mean no limit)")
	}
}

func TestHandle_Check_AllowedUEUnderLimit(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_NSACF", "true")

	ctx := context.Background()
	mock.Put(ctx, "nsacf-policies/1-010203", models.SliceAdmissionPolicy{
		SST: 1, SD: "010203", MaxUEs: 100, MaxSessions: 200,
	})
	mock.Put(ctx, "nsacf-counters/1-010203", models.SliceCounters{
		SST: 1, SD: "010203", CurrentUEs: 50, CurrentSessions: 10,
	})

	body, _ := json.Marshal(AvailabilityCheckRequest{
		SNSSAI:    models.SNSSAI{SST: 1, SD: "010203"},
		CheckType: "UE",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var checkResp AvailabilityCheckResponse
	json.Unmarshal(resp.Body, &checkResp)
	if !checkResp.Allowed {
		t.Errorf("allowed = false, want true")
	}
	if checkResp.Current != 50 {
		t.Errorf("current = %d, want 50", checkResp.Current)
	}
	if checkResp.Max != 100 {
		t.Errorf("max = %d, want 100", checkResp.Max)
	}
}

func TestHandle_Check_DeniedUEOverLimit(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_NSACF", "true")

	ctx := context.Background()
	mock.Put(ctx, "nsacf-policies/1-010203", models.SliceAdmissionPolicy{
		SST: 1, SD: "010203", MaxUEs: 100, MaxSessions: 200,
	})
	mock.Put(ctx, "nsacf-counters/1-010203", models.SliceCounters{
		SST: 1, SD: "010203", CurrentUEs: 100, CurrentSessions: 10,
	})

	body, _ := json.Marshal(AvailabilityCheckRequest{
		SNSSAI:    models.SNSSAI{SST: 1, SD: "010203"},
		CheckType: "UE",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403; body: %s", resp.StatusCode, resp.Body)
	}

	var checkResp AvailabilityCheckResponse
	json.Unmarshal(resp.Body, &checkResp)
	if checkResp.Allowed {
		t.Errorf("allowed = true, want false")
	}
	if checkResp.Cause != "SLICE_NOT_AVAILABLE" {
		t.Errorf("cause = %q, want SLICE_NOT_AVAILABLE", checkResp.Cause)
	}
	if checkResp.Current != 100 {
		t.Errorf("current = %d, want 100", checkResp.Current)
	}
}

func TestHandle_Check_AllowedSessionUnderLimit(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_NSACF", "true")

	ctx := context.Background()
	mock.Put(ctx, "nsacf-policies/2", models.SliceAdmissionPolicy{
		SST: 2, MaxUEs: 50, MaxSessions: 500,
	})
	mock.Put(ctx, "nsacf-counters/2", models.SliceCounters{
		SST: 2, CurrentUEs: 10, CurrentSessions: 499,
	})

	body, _ := json.Marshal(AvailabilityCheckRequest{
		SNSSAI:    models.SNSSAI{SST: 2},
		CheckType: "PDU_SESSION",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var checkResp AvailabilityCheckResponse
	json.Unmarshal(resp.Body, &checkResp)
	if !checkResp.Allowed {
		t.Errorf("allowed = false, want true")
	}
	if checkResp.Current != 499 {
		t.Errorf("current = %d, want 499", checkResp.Current)
	}
	if checkResp.Max != 500 {
		t.Errorf("max = %d, want 500", checkResp.Max)
	}
}

func TestHandle_Check_DeniedSessionOverLimit(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_NSACF", "true")

	ctx := context.Background()
	mock.Put(ctx, "nsacf-policies/1-010203", models.SliceAdmissionPolicy{
		SST: 1, SD: "010203", MaxUEs: 100, MaxSessions: 200,
	})
	mock.Put(ctx, "nsacf-counters/1-010203", models.SliceCounters{
		SST: 1, SD: "010203", CurrentUEs: 10, CurrentSessions: 200,
	})

	body, _ := json.Marshal(AvailabilityCheckRequest{
		SNSSAI:    models.SNSSAI{SST: 1, SD: "010203"},
		CheckType: "PDU_SESSION",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403; body: %s", resp.StatusCode, resp.Body)
	}

	var checkResp AvailabilityCheckResponse
	json.Unmarshal(resp.Body, &checkResp)
	if checkResp.Allowed {
		t.Errorf("allowed = true, want false")
	}
	if checkResp.Cause != "SLICE_NOT_AVAILABLE" {
		t.Errorf("cause = %q, want SLICE_NOT_AVAILABLE", checkResp.Cause)
	}
}

func TestHandle_Check_DisabledByEnvVar(t *testing.T) {
	setupMock(t)
	// ENABLE_NSACF not set, should always allow

	body, _ := json.Marshal(AvailabilityCheckRequest{
		SNSSAI:    models.SNSSAI{SST: 1, SD: "010203"},
		CheckType: "UE",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var checkResp AvailabilityCheckResponse
	json.Unmarshal(resp.Body, &checkResp)
	if !checkResp.Allowed {
		t.Errorf("allowed = false, want true when NSACF is disabled")
	}
}

func TestHandle_Check_InvalidJSON(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_NSACF", "true")

	req := handler.Request{Method: "POST", Body: []byte(`{bad`)}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestHandle_Check_MissingSNSSAI(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_NSACF", "true")

	body, _ := json.Marshal(AvailabilityCheckRequest{
		SNSSAI:    models.SNSSAI{SST: 0},
		CheckType: "UE",
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
