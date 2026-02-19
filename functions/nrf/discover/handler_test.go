package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/haidinhtuan/serverless5gc/pkg/models"
)

// mockNRFStore is an in-memory NRFStore for testing.
type mockNRFStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMockNRFStore() *mockNRFStore {
	return &mockNRFStore{data: make(map[string][]byte)}
}

func (m *mockNRFStore) Put(_ context.Context, key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m.data[key] = data
	return nil
}

func (m *mockNRFStore) Get(_ context.Context, key string, dest interface{}) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.data[key]
	if !ok {
		return fmt.Errorf("key %s not found", key)
	}
	return json.Unmarshal(data, dest)
}

func (m *mockNRFStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *mockNRFStore) GetByPrefix(_ context.Context, prefix string) ([][]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var results [][]byte
	for k, v := range m.data {
		if strings.HasPrefix(k, prefix) {
			results = append(results, v)
		}
	}
	return results, nil
}

func seedProfiles(t *testing.T, mock *mockNRFStore) {
	t.Helper()
	ctx := context.Background()
	profiles := []models.NFProfile{
		{NFInstanceID: "amf-001", NFType: "AMF", NFStatus: "REGISTERED", IPv4Addresses: []string{"10.0.0.1"}},
		{NFInstanceID: "amf-002", NFType: "AMF", NFStatus: "REGISTERED", IPv4Addresses: []string{"10.0.0.2"}},
		{NFInstanceID: "smf-001", NFType: "SMF", NFStatus: "REGISTERED", IPv4Addresses: []string{"10.0.1.1"}},
		{NFInstanceID: "udm-001", NFType: "UDM", NFStatus: "REGISTERED", IPv4Addresses: []string{"10.0.2.1"}},
	}
	for _, p := range profiles {
		key := fmt.Sprintf("/nrf/nf-instances/%s", p.NFInstanceID)
		if err := mock.Put(ctx, key, p); err != nil {
			t.Fatalf("seed profile %s: %v", p.NFInstanceID, err)
		}
	}
}

func TestHandle_DiscoverByType(t *testing.T) {
	mock := newMockNRFStore()
	SetStore(mock)
	seedProfiles(t, mock)

	req := handler.Request{
		Method:      "GET",
		QueryString: "target-nf-type=AMF",
	}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want %d; body: %s", resp.StatusCode, http.StatusOK, resp.Body)
	}

	var result DiscoverResult
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(result.NFInstances) != 2 {
		t.Fatalf("got %d AMF instances, want 2", len(result.NFInstances))
	}
	for _, nf := range result.NFInstances {
		if nf.NFType != "AMF" {
			t.Fatalf("got type %s, want AMF", nf.NFType)
		}
	}
}

func TestHandle_DiscoverNoMatch(t *testing.T) {
	mock := newMockNRFStore()
	SetStore(mock)
	seedProfiles(t, mock)

	req := handler.Request{
		Method:      "GET",
		QueryString: "target-nf-type=PCF",
	}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result DiscoverResult
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(result.NFInstances) != 0 {
		t.Fatalf("got %d instances, want 0", len(result.NFInstances))
	}
}

func TestHandle_DiscoverMissingType(t *testing.T) {
	SetStore(newMockNRFStore())

	req := handler.Request{
		Method:      "GET",
		QueryString: "",
	}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestHandle_DiscoverSMF(t *testing.T) {
	mock := newMockNRFStore()
	SetStore(mock)
	seedProfiles(t, mock)

	req := handler.Request{
		Method:      "GET",
		QueryString: "target-nf-type=SMF",
	}

	resp, err := Handle(req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result DiscoverResult
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(result.NFInstances) != 1 {
		t.Fatalf("got %d SMF instances, want 1", len(result.NFInstances))
	}
	if result.NFInstances[0].NFInstanceID != "smf-001" {
		t.Fatalf("got ID %s, want smf-001", result.NFInstances[0].NFInstanceID)
	}
}
