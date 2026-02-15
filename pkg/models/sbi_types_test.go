package models

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestProblemDetails_JSON(t *testing.T) {
	pd := ProblemDetails{
		Type:   "https://example.com/errors/not-found",
		Title:  "Not Found",
		Status: http.StatusNotFound,
		Detail: "UE context not found for given SUPI",
		Cause:  "CONTEXT_NOT_FOUND",
	}

	data, err := json.Marshal(pd)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify camelCase field names per TS 29.571
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	if _, ok := raw["type"]; !ok {
		t.Error("missing 'type' field")
	}
	if _, ok := raw["title"]; !ok {
		t.Error("missing 'title' field")
	}
	if _, ok := raw["status"]; !ok {
		t.Error("missing 'status' field")
	}
	if _, ok := raw["detail"]; !ok {
		t.Error("missing 'detail' field")
	}
	if _, ok := raw["cause"]; !ok {
		t.Error("missing 'cause' field")
	}
}

func TestProblemDetails_Unmarshal(t *testing.T) {
	jsonStr := `{"type":"urn:3gpp","title":"Forbidden","status":403,"detail":"auth failed","cause":"SERVING_NETWORK_NOT_AUTHORIZED"}`
	var pd ProblemDetails
	if err := json.Unmarshal([]byte(jsonStr), &pd); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if pd.Status != 403 {
		t.Errorf("Status = %d, want 403", pd.Status)
	}
	if pd.Cause != "SERVING_NETWORK_NOT_AUTHORIZED" {
		t.Errorf("Cause = %q, want SERVING_NETWORK_NOT_AUTHORIZED", pd.Cause)
	}
}

func TestNewProblemDetails(t *testing.T) {
	pd := NewProblemDetails(http.StatusForbidden, "AUTHENTICATION_FAILURE", "auth failed for SUPI")

	if pd.Status != http.StatusForbidden {
		t.Errorf("Status = %d, want %d", pd.Status, http.StatusForbidden)
	}
	if pd.Title != "Forbidden" {
		t.Errorf("Title = %q, want Forbidden", pd.Title)
	}
	if pd.Cause != "AUTHENTICATION_FAILURE" {
		t.Errorf("Cause = %q, want AUTHENTICATION_FAILURE", pd.Cause)
	}
	if pd.Detail != "auth failed for SUPI" {
		t.Errorf("Detail = %q, want %q", pd.Detail, "auth failed for SUPI")
	}
}

func TestNewProblemDetails_StandardTitles(t *testing.T) {
	tests := []struct {
		status int
		title  string
	}{
		{http.StatusBadRequest, "Bad Request"},
		{http.StatusUnauthorized, "Unauthorized"},
		{http.StatusForbidden, "Forbidden"},
		{http.StatusNotFound, "Not Found"},
		{http.StatusConflict, "Conflict"},
		{http.StatusInternalServerError, "Internal Server Error"},
		{http.StatusBadGateway, "Bad Gateway"},
		{http.StatusServiceUnavailable, "Service Unavailable"},
		{599, "Error"},
	}

	for _, tt := range tests {
		pd := NewProblemDetails(tt.status, "", "")
		if pd.Title != tt.title {
			t.Errorf("NewProblemDetails(%d).Title = %q, want %q", tt.status, pd.Title, tt.title)
		}
	}
}

func TestUeContext3GPPFieldNames(t *testing.T) {
	ctx := UEContext{
		SUPI:              "imsi-001010000000001",
		GUTI:              "5g-guti-test",
		RegistrationState: "REGISTERED",
		CmState:           "CONNECTED",
	}

	data, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	// Verify field names match 3GPP SBI naming
	if _, ok := raw["supi"]; !ok {
		t.Error("missing 'supi' field")
	}
	if _, ok := raw["guti"]; !ok {
		t.Error("missing 'guti' field")
	}
}

func TestNFProfile3GPPFieldNames(t *testing.T) {
	profile := NFProfile{
		NFInstanceID:  "amf-001",
		NFType:        "AMF",
		NFStatus:      "REGISTERED",
		IPv4Addresses: []string{"10.0.0.1"},
	}

	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	// TS 29.510 field names
	if _, ok := raw["nfInstanceId"]; !ok {
		t.Error("missing 'nfInstanceId' field (TS 29.510)")
	}
	if _, ok := raw["nfType"]; !ok {
		t.Error("missing 'nfType' field (TS 29.510)")
	}
	if _, ok := raw["nfStatus"]; !ok {
		t.Error("missing 'nfStatus' field (TS 29.510)")
	}
}
