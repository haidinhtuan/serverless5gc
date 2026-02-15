//go:build integration

package integration

import (
	"net/http"
	"testing"
)

// TestPDUSessionEstablishment tests the full PDU Session Establishment procedure
// per 3GPP TS 23.502 Section 4.3.2:
//  1. Provision subscriber and register UE (prerequisite)
//  2. Send Nsmf_PDUSession_CreateSMContext to SMF
//  3. SMF calls PCF for policy (Npcf_SMPolicyControl_Create)
//  4. SMF allocates UE IP from pool
//  5. SMF sends PFCP Session Establishment to UPF (N4)
//  6. SMF stores PDU session in Redis
//  7. Verify session is ACTIVE with valid UE address
func TestPDUSessionEstablishment(t *testing.T) {
	// Prerequisite: provision subscriber and register
	provisionTestSubscriber(t)

	regReq := map[string]interface{}{
		"supi":              testSUPI,
		"ran_ue_ngap_id":    5001,
		"gnb_id":            "gnb-001",
		"registration_type": 1,
		"requested_nssai": []map[string]interface{}{
			{"sst": 1, "sd": "010203"},
		},
	}
	resp, body := callFunction(t, "amf-initial-registration", regReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("registration failed: status %d, body: %s", resp.StatusCode, body)
	}

	// Step 2: Create PDU session
	pduReq := map[string]interface{}{
		"supi":             testSUPI,
		"snssai":           map[string]interface{}{"sst": 1, "sd": "010203"},
		"dnn":              testDNN,
		"pdu_session_type": "IPv4",
		"pdu_session_id":   1,
	}

	resp, body = callFunction(t, "smf-pdu-session-create", pduReq)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("PDU session create failed: status %d, body: %s", resp.StatusCode, body)
	}

	var pduResp struct {
		SessionID string `json:"session_id"`
		UEAddress string `json:"ue_address"`
		State     string `json:"state"`
		QFI       uint8  `json:"qfi"`
		AMBRUL    uint64 `json:"session_ambr_ul"`
		AMBRDL    uint64 `json:"session_ambr_dl"`
		DNN       string `json:"dnn"`
	}
	decodeJSON(t, body, &pduResp)

	if pduResp.SessionID == "" {
		t.Error("session_id should not be empty")
	}
	if pduResp.UEAddress == "" {
		t.Error("ue_address should not be empty (IP allocation failed)")
	}
	if pduResp.State != "ACTIVE" {
		t.Errorf("state = %q, want %q", pduResp.State, "ACTIVE")
	}
	if pduResp.DNN != testDNN {
		t.Errorf("dnn = %q, want %q", pduResp.DNN, testDNN)
	}
	if pduResp.QFI == 0 {
		t.Error("QFI should be > 0 (from PCF policy)")
	}
	if pduResp.AMBRUL == 0 || pduResp.AMBRDL == 0 {
		t.Error("Session AMBR should be > 0")
	}
}

// TestPDUSessionRelease tests the PDU session release procedure
// per 3GPP TS 23.502 Section 4.3.4:
//  1. Create a PDU session
//  2. Release it via Nsmf_PDUSession_ReleaseSMContext
//  3. Verify state is RELEASED
//  4. Verify releasing again returns 404
func TestPDUSessionRelease(t *testing.T) {
	provisionTestSubscriber(t)

	// Register UE
	regReq := map[string]interface{}{
		"supi":              testSUPI,
		"ran_ue_ngap_id":    6001,
		"gnb_id":            "gnb-001",
		"registration_type": 1,
	}
	resp, body := callFunction(t, "amf-initial-registration", regReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("registration failed: status %d, body: %s", resp.StatusCode, body)
	}

	// Create PDU session
	pduReq := map[string]interface{}{
		"supi":   testSUPI,
		"snssai": map[string]interface{}{"sst": 1, "sd": "010203"},
		"dnn":    testDNN,
	}
	resp, body = callFunction(t, "smf-pdu-session-create", pduReq)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("PDU session create failed: status %d, body: %s", resp.StatusCode, body)
	}

	var createResp struct {
		SessionID string `json:"session_id"`
	}
	decodeJSON(t, body, &createResp)

	// Release PDU session
	releaseReq := map[string]interface{}{
		"session_id": createResp.SessionID,
	}
	resp, body = callFunction(t, "smf-pdu-session-release", releaseReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PDU session release failed: status %d, body: %s", resp.StatusCode, body)
	}

	var releaseResp struct {
		SessionID string `json:"session_id"`
		State     string `json:"state"`
	}
	decodeJSON(t, body, &releaseResp)

	if releaseResp.State != "RELEASED" {
		t.Errorf("state = %q, want %q", releaseResp.State, "RELEASED")
	}

	// Verify releasing again returns 404 (session no longer exists)
	resp, _ = callFunction(t, "smf-pdu-session-release", releaseReq)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("double release: status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// TestPDUSessionMissingSUPI tests that creating a PDU session without SUPI fails.
func TestPDUSessionMissingSUPI(t *testing.T) {
	pduReq := map[string]interface{}{
		"dnn": testDNN,
	}

	resp, _ := callFunction(t, "smf-pdu-session-create", pduReq)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// TestPDUSessionDefaultDNN tests that DNN defaults to "internet" when not specified.
func TestPDUSessionDefaultDNN(t *testing.T) {
	provisionTestSubscriber(t)

	// Register UE first
	regReq := map[string]interface{}{
		"supi":              testSUPI,
		"ran_ue_ngap_id":    7001,
		"gnb_id":            "gnb-001",
		"registration_type": 1,
	}
	resp, body := callFunction(t, "amf-initial-registration", regReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("registration failed: status %d, body: %s", resp.StatusCode, body)
	}

	// Create PDU session without DNN
	pduReq := map[string]interface{}{
		"supi":   testSUPI,
		"snssai": map[string]interface{}{"sst": 1},
	}
	resp, body = callFunction(t, "smf-pdu-session-create", pduReq)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("PDU session create failed: status %d, body: %s", resp.StatusCode, body)
	}

	var pduResp struct {
		DNN string `json:"dnn"`
	}
	decodeJSON(t, body, &pduResp)

	if pduResp.DNN != "internet" {
		t.Errorf("default dnn = %q, want %q", pduResp.DNN, "internet")
	}
}

// TestMultiplePDUSessions tests creating multiple PDU sessions for the same UE.
func TestMultiplePDUSessions(t *testing.T) {
	provisionTestSubscriber(t)

	// Register UE
	regReq := map[string]interface{}{
		"supi":              testSUPI,
		"ran_ue_ngap_id":    8001,
		"gnb_id":            "gnb-001",
		"registration_type": 1,
	}
	resp, body := callFunction(t, "amf-initial-registration", regReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("registration failed: status %d, body: %s", resp.StatusCode, body)
	}

	// Create first PDU session
	pduReq1 := map[string]interface{}{
		"supi":           testSUPI,
		"snssai":         map[string]interface{}{"sst": 1, "sd": "010203"},
		"dnn":            testDNN,
		"pdu_session_id": 1,
	}
	resp, body = callFunction(t, "smf-pdu-session-create", pduReq1)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first PDU session create failed: status %d, body: %s", resp.StatusCode, body)
	}
	var resp1 struct {
		SessionID string `json:"session_id"`
		UEAddress string `json:"ue_address"`
	}
	decodeJSON(t, body, &resp1)

	// Create second PDU session
	pduReq2 := map[string]interface{}{
		"supi":           testSUPI,
		"snssai":         map[string]interface{}{"sst": 1, "sd": "010203"},
		"dnn":            "ims",
		"pdu_session_id": 2,
	}
	resp, body = callFunction(t, "smf-pdu-session-create", pduReq2)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("second PDU session create failed: status %d, body: %s", resp.StatusCode, body)
	}
	var resp2 struct {
		SessionID string `json:"session_id"`
		UEAddress string `json:"ue_address"`
	}
	decodeJSON(t, body, &resp2)

	// Verify different session IDs and IP addresses
	if resp1.SessionID == resp2.SessionID {
		t.Error("two sessions should have different session IDs")
	}
	if resp1.UEAddress == resp2.UEAddress {
		t.Error("two sessions should have different UE addresses")
	}
}

// TestFullLifecycle tests the complete UE lifecycle:
// provision → register → PDU session create → PDU session release → deregister
func TestFullLifecycle(t *testing.T) {
	provisionTestSubscriber(t)

	// 1. Register
	regReq := map[string]interface{}{
		"supi":              testSUPI,
		"ran_ue_ngap_id":    9001,
		"gnb_id":            "gnb-001",
		"registration_type": 1,
		"requested_nssai": []map[string]interface{}{
			{"sst": 1, "sd": "010203"},
		},
	}
	resp, body := callFunction(t, "amf-initial-registration", regReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register: status %d, body: %s", resp.StatusCode, body)
	}
	t.Log("UE registered successfully")

	// 2. Create PDU session
	pduReq := map[string]interface{}{
		"supi":   testSUPI,
		"snssai": map[string]interface{}{"sst": 1, "sd": "010203"},
		"dnn":    testDNN,
	}
	resp, body = callFunction(t, "smf-pdu-session-create", pduReq)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create PDU session: status %d, body: %s", resp.StatusCode, body)
	}
	var pduResp struct {
		SessionID string `json:"session_id"`
		UEAddress string `json:"ue_address"`
		State     string `json:"state"`
	}
	decodeJSON(t, body, &pduResp)
	t.Logf("PDU session created: %s, UE addr: %s", pduResp.SessionID, pduResp.UEAddress)

	if pduResp.State != "ACTIVE" {
		t.Errorf("PDU session state = %q, want ACTIVE", pduResp.State)
	}

	// 3. Release PDU session
	releaseReq := map[string]interface{}{
		"session_id": pduResp.SessionID,
	}
	resp, body = callFunction(t, "smf-pdu-session-release", releaseReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("release PDU session: status %d, body: %s", resp.StatusCode, body)
	}
	t.Log("PDU session released")

	// 4. Deregister
	deregReq := map[string]interface{}{
		"supi": testSUPI,
	}
	resp, body = callFunction(t, "amf-deregistration", deregReq)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("deregister: status %d, body: %s", resp.StatusCode, body)
	}

	var deregResp struct {
		Status string `json:"status"`
	}
	decodeJSON(t, body, &deregResp)
	if deregResp.Status != "deregistered" {
		t.Errorf("deregister status = %q, want %q", deregResp.Status, "deregistered")
	}
	t.Log("UE deregistered - full lifecycle complete")
}
