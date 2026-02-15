// Package function implements the AMF Service Request handler.
// Processes UE Service Request per TS 23.502 Section 4.2.3.2.
//
// State transition: CM-IDLE → CM-CONNECTED (TS 23.502 Figure 4.2.3.2-1)
// Precondition: UE must be in RM-REGISTERED state.
package function

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

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

// ServiceRequest is the JSON body for a UE service request.
type ServiceRequest struct {
	SUPI string `json:"supi"`
}

// ServiceResponse is returned on successful service request.
type ServiceResponse struct {
	Status     string `json:"status"`
	SUPI       string `json:"supi"`
	CmState    string `json:"cm_state"`
	NASMessage string `json:"nas_message,omitempty"` // hex-encoded NAS Service Accept
}

// Handle processes a UE service request per TS 23.502 Section 4.2.3.2.
// Steps:
//  1. Validate request and read UE context
//  2. Verify RM-REGISTERED state
//  3. CM-IDLE → CM-CONNECTED state transition
//  4. Build NAS Service Accept (TS 24.501 Section 8.2.16)
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var svcReq ServiceRequest
	if err := json.Unmarshal(req.Body, &svcReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if svcReq.SUPI == "" {
		return errorResp(http.StatusBadRequest, "supi is required"), nil
	}

	key := "ue:" + svcReq.SUPI

	var ueCtx models.UEContext
	if err := store.Get(ctx, key, &ueCtx); err != nil {
		return errorResp(http.StatusNotFound, "ue context not found: %s", err), nil
	}

	// Precondition: UE must be in RM-REGISTERED (TS 23.502 Section 4.2.3.2)
	if ueCtx.RegistrationState != "REGISTERED" {
		// TS 24.501 Cause #10: Implicitly de-registered
		rejectNAS := []byte{nas.EPD5GMM, nas.SecurityHeaderPlain, nas.MsgTypeServiceReject, nas.CauseImplicitlyDeregistered}
		return errorRespWithNAS(http.StatusConflict,
			hex.EncodeToString(rejectNAS),
			"UE is not registered (state: %s)", ueCtx.RegistrationState), nil
	}

	// State transition: CM-IDLE → CM-CONNECTED
	ueCtx.CmState = "CONNECTED"
	ueCtx.LastActivity = time.Now()

	if err := store.Put(ctx, key, ueCtx); err != nil {
		return errorResp(http.StatusInternalServerError, "update ue context: %s", err), nil
	}

	// Build NAS Service Accept (TS 24.501 Section 8.2.16)
	svcAcceptNAS := []byte{nas.EPD5GMM, nas.SecurityHeaderPlain, nas.MsgTypeServiceAccept}

	body, _ := json.Marshal(ServiceResponse{
		Status:     "connected",
		SUPI:       svcReq.SUPI,
		CmState:    "CONNECTED",
		NASMessage: hex.EncodeToString(svcAcceptNAS),
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
