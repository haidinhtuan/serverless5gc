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

func TestHandle_WriteSubscriber(t *testing.T) {
	mock := setupMock(t)

	sub := models.SubscriberData{
		SUPI: "imsi-001010000000001",
		AuthenticationData: &models.AuthData{
			AuthMethod:   "5G_AKA",
			PermanentKey: []byte{0x46, 0x5B, 0x5C, 0xE8, 0xB1, 0x99, 0xB4, 0x9F,
				0xAA, 0x5F, 0x0A, 0x2E, 0xE2, 0x38, 0xA6, 0xBC},
			OPc: []byte{0xCD, 0x63, 0xCB, 0x71, 0x95, 0x4A, 0x9F, 0x4E,
				0x48, 0xA5, 0x99, 0x4E, 0x37, 0xA0, 0x2B, 0xAF},
			AMF: []byte{0xB9, 0xB9},
			SQN: []byte{0xFF, 0x9B, 0xB4, 0xD0, 0xB6, 0x07},
		},
		AccessAndMobility: &models.AccessMobData{
			NSSAI:      []models.SNSSAI{{SST: 1, SD: "010203"}},
			DefaultDNN: "internet",
		},
	}
	body, _ := json.Marshal(sub)

	req := handler.Request{
		Method: "POST",
		Body:   body,
	}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201; body: %s", resp.StatusCode, resp.Body)
	}

	// Verify the data was stored
	var stored models.SubscriberData
	if err := mock.Get(context.Background(), "subscribers/imsi-001010000000001", &stored); err != nil {
		t.Fatalf("Get stored data: %v", err)
	}
	if stored.SUPI != "imsi-001010000000001" {
		t.Errorf("stored SUPI = %s, want imsi-001010000000001", stored.SUPI)
	}
}

func TestHandle_WriteSubscriber_InvalidJSON(t *testing.T) {
	setupMock(t)

	req := handler.Request{
		Method: "POST",
		Body:   []byte("not json"),
	}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}

func TestHandle_WriteSubscriber_MissingSUPI(t *testing.T) {
	setupMock(t)

	body, _ := json.Marshal(models.SubscriberData{})
	req := handler.Request{Method: "POST", Body: body}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}
}
