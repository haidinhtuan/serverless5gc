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

func TestHandle_Update_IncrementUE(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_NSACF", "true")

	ctx := context.Background()
	mock.Put(ctx, "nsacf-counters/1-010203", models.SliceCounters{
		SST: 1, SD: "010203", CurrentUEs: 5, CurrentSessions: 10,
	})

	body, _ := json.Marshal(UpdateCountersRequest{
		SNSSAI:      models.SNSSAI{SST: 1, SD: "010203"},
		CounterType: "UE",
		Operation:   "INCREMENT",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var counters models.SliceCounters
	json.Unmarshal(resp.Body, &counters)
	if counters.CurrentUEs != 6 {
		t.Errorf("current_ues = %d, want 6", counters.CurrentUEs)
	}
	if counters.CurrentSessions != 10 {
		t.Errorf("current_sessions = %d, want 10 (unchanged)", counters.CurrentSessions)
	}
}

func TestHandle_Update_DecrementUE(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_NSACF", "true")

	ctx := context.Background()
	mock.Put(ctx, "nsacf-counters/1-010203", models.SliceCounters{
		SST: 1, SD: "010203", CurrentUEs: 5, CurrentSessions: 10,
	})

	body, _ := json.Marshal(UpdateCountersRequest{
		SNSSAI:      models.SNSSAI{SST: 1, SD: "010203"},
		CounterType: "UE",
		Operation:   "DECREMENT",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var counters models.SliceCounters
	json.Unmarshal(resp.Body, &counters)
	if counters.CurrentUEs != 4 {
		t.Errorf("current_ues = %d, want 4", counters.CurrentUEs)
	}
}

func TestHandle_Update_DecrementUEFloorsAtZero(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_NSACF", "true")

	ctx := context.Background()
	mock.Put(ctx, "nsacf-counters/1-010203", models.SliceCounters{
		SST: 1, SD: "010203", CurrentUEs: 0, CurrentSessions: 3,
	})

	body, _ := json.Marshal(UpdateCountersRequest{
		SNSSAI:      models.SNSSAI{SST: 1, SD: "010203"},
		CounterType: "UE",
		Operation:   "DECREMENT",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var counters models.SliceCounters
	json.Unmarshal(resp.Body, &counters)
	if counters.CurrentUEs != 0 {
		t.Errorf("current_ues = %d, want 0 (should not go negative)", counters.CurrentUEs)
	}
}

func TestHandle_Update_IncrementSession(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_NSACF", "true")

	ctx := context.Background()
	mock.Put(ctx, "nsacf-counters/2", models.SliceCounters{
		SST: 2, CurrentUEs: 3, CurrentSessions: 20,
	})

	body, _ := json.Marshal(UpdateCountersRequest{
		SNSSAI:      models.SNSSAI{SST: 2},
		CounterType: "PDU_SESSION",
		Operation:   "INCREMENT",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var counters models.SliceCounters
	json.Unmarshal(resp.Body, &counters)
	if counters.CurrentSessions != 21 {
		t.Errorf("current_sessions = %d, want 21", counters.CurrentSessions)
	}
	if counters.CurrentUEs != 3 {
		t.Errorf("current_ues = %d, want 3 (unchanged)", counters.CurrentUEs)
	}
}

func TestHandle_Update_DecrementSession(t *testing.T) {
	mock := setupMock(t)
	t.Setenv("ENABLE_NSACF", "true")

	ctx := context.Background()
	mock.Put(ctx, "nsacf-counters/1-010203", models.SliceCounters{
		SST: 1, SD: "010203", CurrentUEs: 5, CurrentSessions: 10,
	})

	body, _ := json.Marshal(UpdateCountersRequest{
		SNSSAI:      models.SNSSAI{SST: 1, SD: "010203"},
		CounterType: "PDU_SESSION",
		Operation:   "DECREMENT",
	})

	req := handler.Request{Method: "POST", Body: body}
	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}

	var counters models.SliceCounters
	json.Unmarshal(resp.Body, &counters)
	if counters.CurrentSessions != 9 {
		t.Errorf("current_sessions = %d, want 9", counters.CurrentSessions)
	}
}

func TestHandle_Update_DisabledByEnvVar(t *testing.T) {
	setupMock(t)
	// ENABLE_NSACF not set

	body, _ := json.Marshal(UpdateCountersRequest{
		SNSSAI:      models.SNSSAI{SST: 1, SD: "010203"},
		CounterType: "UE",
		Operation:   "INCREMENT",
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
		t.Errorf("status = %q, want disabled", result["status"])
	}
}

func TestHandle_Update_InvalidJSON(t *testing.T) {
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

func TestHandle_Update_InvalidOperation(t *testing.T) {
	setupMock(t)
	t.Setenv("ENABLE_NSACF", "true")

	body, _ := json.Marshal(UpdateCountersRequest{
		SNSSAI:      models.SNSSAI{SST: 1, SD: "010203"},
		CounterType: "UE",
		Operation:   "RESET",
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
