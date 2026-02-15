// Package function implements the AMF UE Deregistration handler.
// Processes UE-initiated deregistration per TS 23.502 Section 4.2.2.3.2.
//
// State transition: RM-REGISTERED → RM-DEREGISTERED (TS 23.502 Figure 4.2.2.3.2-1)
// CM state: CM-CONNECTED → CM-IDLE (implicit on deregistration)
package function

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/models"
	"github.com/tdinh/serverless5gc/pkg/nas"
	"github.com/tdinh/serverless5gc/pkg/state"
)

var store state.KVStore

// SetStore injects a KVStore (used in tests).
func SetStore(s state.KVStore) { store = s }

func init() {
	if store != nil {
		return
	}
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	store = state.NewRedisStore(addr)
}

// DeregistrationRequest is the JSON body for UE deregistration.
type DeregistrationRequest struct {
	SUPI            string `json:"supi"`
	DeregistrationType uint8  `json:"deregistration_type,omitempty"` // TS 24.501 Section 9.11.3.20
}

// DeregistrationResponse is returned on successful deregistration.
type DeregistrationResponse struct {
	Status     string `json:"status"`
	SUPI       string `json:"supi"`
	NASMessage string `json:"nas_message,omitempty"` // hex-encoded NAS Deregistration Accept
}

// Handle processes a UE deregistration request per TS 23.502 Section 4.2.2.3.2.
// Steps:
//  1. Validate request and read UE context
//  2. RM-REGISTERED → RM-DEREGISTERED state transition
//  3. Delete UE context from store (release AMF resources)
//  4. Build NAS Deregistration Accept (TS 24.501 Section 8.2.12)
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var deregReq DeregistrationRequest
	if err := json.Unmarshal(req.Body, &deregReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if deregReq.SUPI == "" {
		return errorResp(http.StatusBadRequest, "supi is required"), nil
	}

	key := "ue:" + deregReq.SUPI

	// Read existing UE context
	var ueCtx models.UEContext
	if err := store.Get(ctx, key, &ueCtx); err != nil {
		// TS 24.501 Cause #10: Implicitly de-registered (context not found)
		rejectNAS := nas.EncodeRegistrationReject(&nas.RegistrationReject{
			CauseCode: nas.CauseImplicitlyDeregistered,
		})
		return errorRespWithNAS(http.StatusNotFound,
			hex.EncodeToString(rejectNAS),
			"ue context not found: %s", err), nil
	}

	// State transition: RM-REGISTERED → RM-DEREGISTERED
	ueCtx.RegistrationState = "DEREGISTERED"
	ueCtx.CmState = "IDLE"

	// Delete UE context from store (release AMF resources)
	if err := store.Delete(ctx, key); err != nil {
		return errorResp(http.StatusInternalServerError, "delete ue context: %s", err), nil
	}

	// Build NAS Deregistration Accept (TS 24.501 Section 8.2.12)
	// Simplified: EPD(0x7E) + SecHdr(0x00) + MsgType(0x46)
	deregAcceptNAS := []byte{nas.EPD5GMM, nas.SecurityHeaderPlain, nas.MsgTypeDeregistrationAcceptUE}

	body, _ := json.Marshal(DeregistrationResponse{
		Status:     "deregistered",
		SUPI:       deregReq.SUPI,
		NASMessage: hex.EncodeToString(deregAcceptNAS),
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

func errorRespWithNAS(status int, nasHex string, format string, args ...interface{}) handler.Response {
	msg := fmt.Sprintf(format, args...)
	body, _ := json.Marshal(map[string]string{
		"error":       msg,
		"nas_message": nasHex,
	})
	return handler.Response{StatusCode: status, Body: body}
}
