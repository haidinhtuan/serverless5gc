// Package function implements the AMF PDU Session Relay handler.
// Forwards PDU session establishment requests from the UE to the SMF
// per TS 23.502 Section 4.3.2.2.1.
//
// SBI call: AMF → SMF: Nsmf_PDUSession_CreateSMContext (TS 29.502)
package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/models"
	"github.com/tdinh/serverless5gc/pkg/sbi"
	"github.com/tdinh/serverless5gc/pkg/state"
)

// SBICaller abstracts inter-NF communication for testability.
type SBICaller interface {
	CallFunction(funcName string, payload interface{}, result interface{}) error
}

var (
	store     state.KVStore
	sbiClient SBICaller
)

// SetStore injects a KVStore (used in tests).
func SetStore(s state.KVStore) { store = s }

// SetSBI injects an SBI caller (used in tests).
func SetSBI(s SBICaller) { sbiClient = s }

func init() {
	if store != nil {
		return
	}
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	store = state.NewRedisStore(addr)
	sbiClient = sbi.NewClient()
}

// PDUSessionRelayRequest is the JSON body for a PDU session relay.
// Corresponds to Nsmf_PDUSession_CreateSMContext request (TS 29.502).
type PDUSessionRelayRequest struct {
	SUPI   string        `json:"supi"`
	SNSSAI models.SNSSAI `json:"snssai"`
	DNN    string        `json:"dnn"`
}

// PDUSessionRelayResponse is returned after forwarding to SMF.
type PDUSessionRelayResponse struct {
	Status    string `json:"status"`
	SessionID string `json:"session_id,omitempty"`
	SUPI      string `json:"supi"`
}

type smfResponse struct {
	Status    string `json:"status"`
	SessionID string `json:"session_id"`
}

// Handle forwards a PDU session request to the SMF via SBI.
// Steps per TS 23.502 Section 4.3.2.2.1:
//  1. Validate UE is in RM-REGISTERED state
//  2. Forward to SMF: Nsmf_PDUSession_CreateSMContext (TS 29.502)
//  3. Return SMF response to the UE (via SCTP proxy)
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var relayReq PDUSessionRelayRequest
	if err := json.Unmarshal(req.Body, &relayReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if relayReq.SUPI == "" {
		return errorResp(http.StatusBadRequest, "supi is required"), nil
	}

	// Verify UE is in RM-REGISTERED state
	key := "ue:" + relayReq.SUPI
	var ueCtx models.UEContext
	if err := store.Get(ctx, key, &ueCtx); err != nil {
		return errorResp(http.StatusNotFound, "ue context not found: %s", err), nil
	}
	if ueCtx.RegistrationState != "REGISTERED" {
		return errorResp(http.StatusConflict, "UE is not registered"), nil
	}

	// Nsmf_PDUSession_CreateSMContext (TS 29.502)
	var smfResp smfResponse
	if err := sbiClient.CallFunction("smf-pdu-session-create", relayReq, &smfResp); err != nil {
		return errorResp(http.StatusBadGateway, "Nsmf_PDUSession_CreateSMContext: %s", err), nil
	}

	body, _ := json.Marshal(PDUSessionRelayResponse{
		Status:    smfResp.Status,
		SessionID: smfResp.SessionID,
		SUPI:      relayReq.SUPI,
	})
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func errorResp(status int, format string, args ...interface{}) handler.Response {
	msg := fmt.Sprintf(format, args...)
	body, _ := json.Marshal(map[string]string{"error": msg})
	return handler.Response{StatusCode: status, Body: body}
}
