package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/haidinhtuan/serverless5gc/pkg/models"
)

// mockStore is an in-memory KVStore for testing.
type mockStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMockStore() *mockStore {
	return &mockStore{data: make(map[string][]byte)}
}

func (m *mockStore) Put(_ context.Context, key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m.data[key] = data
	return nil
}

func (m *mockStore) Get(_ context.Context, key string, dest interface{}) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.data[key]
	if !ok {
		return fmt.Errorf("key %s not found", key)
	}
	return json.Unmarshal(data, dest)
}

func (m *mockStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func TestHandle_RegisterNF(t *testing.T) {
	mock := newMockStore()
	SetStore(mock)

	profile := models.NFProfile{
		NFInstanceID:  "amf-001",
		NFType:        "AMF",
		IPv4Addresses: []string{"10.0.0.1"},
		NFServices: []models.NFService{
			{ServiceName: "namf-comm", Scheme: "http", FQDN: "amf.local"},
		},
	}
	body, _ := json.Marshal(profile)

	req := handler.Request{
		Body:   body,
		Method: "PUT",
	}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("got status %d, want %d; body: %s", resp.StatusCode, http.StatusCreated, resp.Body)
	}

	var registered models.NFProfile
	if err := json.Unmarshal(resp.Body, &registered); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if registered.NFInstanceID != "amf-001" {
		t.Fatalf("got ID %s, want amf-001", registered.NFInstanceID)
	}
	if registered.NFStatus != "REGISTERED" {
		t.Fatalf("got status %s, want REGISTERED", registered.NFStatus)
	}

	// Verify stored in mock
	var stored models.NFProfile
	if err := mock.Get(context.Background(), "/nrf/nf-instances/amf-001", &stored); err != nil {
		t.Fatalf("profile not found in store: %v", err)
	}
	if stored.NFType != "AMF" {
		t.Fatalf("stored type %s, want AMF", stored.NFType)
	}
}

func TestHandle_RegisterNF_InvalidJSON(t *testing.T) {
	SetStore(newMockStore())

	req := handler.Request{
		Body:   []byte(`{invalid`),
		Method: "PUT",
	}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandle_RegisterNF_MissingID(t *testing.T) {
	SetStore(newMockStore())

	profile := models.NFProfile{NFType: "AMF"}
	body, _ := json.Marshal(profile)

	req := handler.Request{
		Body:   body,
		Method: "PUT",
	}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}
