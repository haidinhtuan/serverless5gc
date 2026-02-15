//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// Test subscriber data: PLMN 001/01, standard test credentials per 3GPP TS 35.208 Annex B
const (
	testSUPI = "imsi-001010000000001"
	testK    = "465B5CE8B199B49FAA5F0A2EE238A6BC"
	testOPc  = "E8ED289DEBA952E4283B54E88E6183CA"
	testDNN  = "internet"
)

var gatewayURL string

func TestMain(m *testing.M) {
	gw := os.Getenv("GATEWAY_URL")
	if gw == "" {
		gw = "http://localhost:8080"
	}
	gatewayURL = gw

	// Wait for gateway to be ready
	if err := waitForGateway(gatewayURL, 30*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "gateway not ready: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func waitForGateway(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("gateway at %s not ready after %s", url, timeout)
}

// callFunction sends a POST request to the test gateway, mimicking an OpenFaaS invocation.
func callFunction(t *testing.T, funcName string, payload interface{}) (*http.Response, []byte) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	url := fmt.Sprintf("%s/function/%s", gatewayURL, funcName)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}

	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp, respBody
}

// decodeJSON unmarshals a JSON response body into the given destination.
func decodeJSON(t *testing.T, data []byte, dest interface{}) {
	t.Helper()
	if err := json.Unmarshal(data, dest); err != nil {
		t.Fatalf("decode JSON %q: %v", string(data), err)
	}
}

// provisionTestSubscriber writes a test subscriber to UDR so authentication succeeds.
func provisionTestSubscriber(t *testing.T) {
	t.Helper()
	sub := map[string]interface{}{
		"supi": testSUPI,
		"auth_data": map[string]interface{}{
			"auth_method": "5G_AKA",
			"k":           hexToBytes(testK),
			"opc":         hexToBytes(testOPc),
			"amf":         []byte{0x80, 0x00},
			"sqn":         []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
		},
		"access_mobility_data": map[string]interface{}{
			"nssai": []map[string]interface{}{
				{"sst": 1, "sd": "010203"},
			},
			"default_dnn": testDNN,
		},
		"session_management": []map[string]interface{}{
			{
				"snssai": map[string]interface{}{"sst": 1, "sd": "010203"},
				"dnn":    testDNN,
				"qos_ref": 9,
			},
		},
	}

	resp, body := callFunction(t, "udr-data-write", sub)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("provision subscriber: status %d, body: %s", resp.StatusCode, body)
	}
}

func hexToBytes(h string) []byte {
	b := make([]byte, len(h)/2)
	for i := 0; i < len(h); i += 2 {
		fmt.Sscanf(h[i:i+2], "%02x", &b[i/2])
	}
	return b
}
