// Package function implements the SMF Nsmf_PDUSession_ReleaseSMContext service operation
// per 3GPP TS 29.502 and TS 23.502 Section 4.3.4.
package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/models"
	"github.com/tdinh/serverless5gc/pkg/sbi"
	"github.com/tdinh/serverless5gc/pkg/state"
)

// PFCPDeleter abstracts PFCP deletion for testability.
type PFCPDeleter interface {
	DeleteSession(seid uint64) error
}

// Store is the backing KV store. Override in tests via SetStore.
var Store state.KVStore

// PFCP is the PFCP client. Override in tests via SetPFCP.
var PFCP PFCPDeleter

func SetStore(s state.KVStore) { Store = s }
func SetPFCP(p PFCPDeleter)     { PFCP = p }

// SBICaller abstracts inter-NF communication for testability.
type SBICaller interface {
	CallFunction(funcName string, payload interface{}, result interface{}) error
}

var SBI SBICaller

func SetSBI(s SBICaller) { SBI = s }

func init() {
	if Store != nil {
		return
	}
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	Store = state.NewRedisStore(addr)
	SBI = sbi.NewClient()
}

// ReleaseSMContextRequest per TS 29.502 (Nsmf_PDUSession_ReleaseSMContext).
type ReleaseSMContextRequest struct {
	SessionID string `json:"session_id"`
}

// Handle releases a PDU session per TS 23.502 Section 4.3.4:
//  1. Fetch session from store
//  2. Send PFCP Session Deletion to UPF (N4, TS 29.244 Section 7.5.6)
//  3. Release UE IP address back to pool
//  4. Remove session from store
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var relReq ReleaseSMContextRequest
	if err := json.Unmarshal(req.Body, &relReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if relReq.SessionID == "" {
		return errorResp(http.StatusBadRequest, "session_id is required"), nil
	}

	// Step 1: Fetch existing session
	key := "pdu-sessions/" + relReq.SessionID
	var session models.PDUSession
	if err := Store.Get(ctx, key, &session); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return errorResp(http.StatusNotFound, "session %s not found", relReq.SessionID), nil
		}
		return errorResp(http.StatusInternalServerError, "get session: %s", err), nil
	}

	// Step 2: Send PFCP Session Deletion to UPF (N4, TS 29.244 Section 7.5.6)
	if PFCP != nil {
		seid := extractSEID(relReq.SessionID)
		if err := PFCP.DeleteSession(seid); err != nil {
			return errorResp(http.StatusInternalServerError, "N4 Session Deletion: %s", err), nil
		}
	}

	// Step 3: Release UE IP address back to pool
	if session.UEAddress != "" {
		Store.Delete(ctx, "ip-pool/allocated/"+session.UEAddress)
	}

	// R17: CHF charging release (TS 32.291)
	if os.Getenv("ENABLE_CHARGING") == "true" && SBI != nil && session.ChargingID != "" {
		SBI.CallFunction("chf-charging-release", map[string]interface{}{
			"charging_id": session.ChargingID,
		}, nil)
	}

	// R17: BSF binding deregistration (TS 29.521)
	if os.Getenv("ENABLE_BSF") == "true" && SBI != nil && session.BSFBindingID != "" {
		SBI.CallFunction("bsf-binding-deregister", map[string]interface{}{
			"binding_id": session.BSFBindingID,
		}, nil)
	}

	// R17: NSACF session counter decrement (TS 29.536)
	if os.Getenv("ENABLE_NSACF") == "true" && SBI != nil {
		SBI.CallFunction("nsacf-update-counters", map[string]interface{}{
			"snssai":       session.SNSSAI,
			"counter_type": "PDU_SESSION",
			"operation":    "DECREMENT",
		}, nil)
	}

	// Step 4: Remove session from store
	if err := Store.Delete(ctx, key); err != nil {
		return errorResp(http.StatusInternalServerError, "delete session: %s", err), nil
	}

	body, _ := json.Marshal(map[string]string{
		"session_id": relReq.SessionID,
		"state":      "RELEASED",
	})
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     jsonHeader(),
	}, nil
}

func extractSEID(sessionID string) uint64 {
	parts := strings.Split(sessionID, "-")
	if len(parts) < 2 {
		return 0
	}
	var seid uint64
	fmt.Sscanf(parts[len(parts)-1], "%d", &seid)
	return seid
}

func errorResp(code int, format string, args ...interface{}) handler.Response {
	msg := fmt.Sprintf(format, args...)
	return handler.Response{
		StatusCode: code,
		Body:       []byte(fmt.Sprintf(`{"error":"%s"}`, msg)),
		Header:     jsonHeader(),
	}
}

func jsonHeader() http.Header {
	return http.Header{"Content-Type": []string{"application/json"}}
}
