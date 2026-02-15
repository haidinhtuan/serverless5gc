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

func TestHandle_ReadSubscriber_ByQuery(t *testing.T) {
	mock := setupMock(t)

	sub := models.SubscriberData{
		SUPI: "imsi-001010000000001",
		AuthenticationData: &models.AuthData{
			AuthMethod:   "5G_AKA",
			PermanentKey: []byte{0x46, 0x5B, 0x5C, 0xE8},
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

	var result models.SubscriberData
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.SUPI != "imsi-001010000000001" {
		t.Errorf("SUPI = %s, want imsi-001010000000001", result.SUPI)
	}
	if result.AuthenticationData == nil {
		t.Fatal("AuthenticationData is nil")
	}
	if result.AuthenticationData.AuthMethod != "5G_AKA" {
		t.Errorf("AuthMethod = %s, want 5G_AKA", result.AuthenticationData.AuthMethod)
	}
}

func TestHandle_ReadSubscriber_ByBody(t *testing.T) {
	mock := setupMock(t)

	sub := models.SubscriberData{SUPI: "imsi-001010000000002"}
	mock.Put(context.Background(), "subscribers/imsi-001010000000002", sub)

	body, _ := json.Marshal(map[string]string{"supi": "imsi-001010000000002"})
	req := handler.Request{Method: "GET", Body: body}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", resp.StatusCode, resp.Body)
	}
}

func TestHandle_ReadSubscriber_NotFound(t *testing.T) {
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

func TestHandle_ReadSubscriber_MissingSUPI(t *testing.T) {
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
