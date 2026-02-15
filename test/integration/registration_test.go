//go:build integration

package integration

import (
	"net/http"
	"testing"
)

// TestRegistrationFlow tests the full UE Initial Registration procedure
// per 3GPP TS 23.502 Section 4.2.2.2.2:
//  1. Provision subscriber in UDR
//  2. Send NAS Registration Request to AMF
//  3. AMF authenticates via AUSF → UDM → UDR chain
//  4. AMF creates UE context in Redis (RM-REGISTERED)
//  5. AMF returns Registration Accept with GUTI and allowed NSSAI
func TestRegistrationFlow(t *testing.T) {
	// Step 1: Provision test subscriber in UDR
	provisionTestSubscriber(t)

	// Step 2: Send Registration Request to AMF
	regReq := map[string]interface{}{
		"supi":              testSUPI,
		"ran_ue_ngap_id":    1001,
		"gnb_id":            "gnb-001",
		"registration_type": 1, // Initial Registration
		"requested_nssai": []map[string]interface{}{
			{"sst": 1, "sd": "010203"},
		},
	}

	resp, body := callFunction(t, "amf-initial-registration", regReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("registration failed: status %d, body: %s", resp.StatusCode, body)
	}

	// Step 3: Verify registration response
	var regResp struct {
		Status            string `json:"status"`
		SUPI              string `json:"supi"`
		GUTI              string `json:"guti"`
		T3512Value        uint32 `json:"t3512_value"`
		NASMessage        string `json:"nas_message"`
		SecurityActivated bool   `json:"security_activated"`
		AllowedNSSAI      []struct {
			SST int32  `json:"sst"`
			SD  string `json:"sd"`
		} `json:"allowed_nssai"`
	}
	decodeJSON(t, body, &regResp)

	if regResp.Status != "registered" {
		t.Errorf("status = %q, want %q", regResp.Status, "registered")
	}
	if regResp.SUPI != testSUPI {
		t.Errorf("supi = %q, want %q", regResp.SUPI, testSUPI)
	}
	if regResp.GUTI == "" {
		t.Error("GUTI should not be empty")
	}
	if regResp.NASMessage == "" {
		t.Error("NAS Registration Accept message should not be empty")
	}
	if !regResp.SecurityActivated {
		t.Error("security should be activated after registration")
	}
	if regResp.T3512Value == 0 {
		t.Error("T3512 timer should be set")
	}

	// Step 4: Verify UE context was created by reading subscriber data
	// (The UE context is stored in Redis under "ue:<SUPI>" - we verify
	// indirectly via the deregistration test below)
}

// TestRegistrationInvalidSUPI tests that registration with an empty SUPI
// is rejected with 400 Bad Request per TS 24.501 cause #9.
func TestRegistrationInvalidSUPI(t *testing.T) {
	regReq := map[string]interface{}{
		"ran_ue_ngap_id": 2001,
		"gnb_id":         "gnb-001",
	}

	resp, _ := callFunction(t, "amf-initial-registration", regReq)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// TestRegistrationUnknownSubscriber tests registration for a UE not provisioned in UDR.
// Should fail at the AUSF authentication step.
func TestRegistrationUnknownSubscriber(t *testing.T) {
	regReq := map[string]interface{}{
		"supi":           "imsi-001010000099999",
		"ran_ue_ngap_id": 3001,
		"gnb_id":         "gnb-001",
	}

	resp, body := callFunction(t, "amf-initial-registration", regReq)
	// Should fail - either 403 (auth failed) or 500 (upstream NF failure)
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expected failure for unknown subscriber, got 200: %s", body)
	}
}

// TestDeregistrationAfterRegistration tests the full deregistration flow:
// register a UE, then deregister it, verify the state transition.
func TestDeregistrationAfterRegistration(t *testing.T) {
	// First provision and register
	provisionTestSubscriber(t)

	regReq := map[string]interface{}{
		"supi":              testSUPI,
		"ran_ue_ngap_id":    4001,
		"gnb_id":            "gnb-001",
		"registration_type": 1,
	}
	resp, body := callFunction(t, "amf-initial-registration", regReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("registration failed: status %d, body: %s", resp.StatusCode, body)
	}

	// Now deregister
	deregReq := map[string]interface{}{
		"supi": testSUPI,
	}
	resp, body = callFunction(t, "amf-deregistration", deregReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("deregistration failed: status %d, body: %s", resp.StatusCode, body)
	}

	var deregResp struct {
		Status     string `json:"status"`
		SUPI       string `json:"supi"`
		NASMessage string `json:"nas_message"`
	}
	decodeJSON(t, body, &deregResp)

	if deregResp.Status != "deregistered" {
		t.Errorf("status = %q, want %q", deregResp.Status, "deregistered")
	}
	if deregResp.NASMessage == "" {
		t.Error("NAS Deregistration Accept should not be empty")
	}

	// Verify UE context is gone - deregistering again should fail
	resp, _ = callFunction(t, "amf-deregistration", deregReq)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("second deregistration: status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// TestDeregistrationWithoutRegistration tests that deregistering an unknown UE
// returns 404 Not Found.
func TestDeregistrationWithoutRegistration(t *testing.T) {
	deregReq := map[string]interface{}{
		"supi": "imsi-001010000088888",
	}

	resp, _ := callFunction(t, "amf-deregistration", deregReq)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
