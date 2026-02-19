// Package function implements the AMF N2 Handover handler.
// Processes inter-gNB handover per TS 23.502 Section 4.9.1.
//
// Simplified handover flow:
//  1. Validate Handover Required from source gNB
//  2. Read UE context, verify RM-REGISTERED + CM-CONNECTED
//  3. Update UE context with new gNB (target)
//  4. Return Handover Command
package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/haidinhtuan/serverless5gc/pkg/models"
	"github.com/haidinhtuan/serverless5gc/pkg/state"
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

// HandoverRequest is the JSON body for an N2 handover.
type HandoverRequest struct {
	SUPI        string `json:"supi"`
	TargetGnbID string `json:"target_gnb_id"`
	Cause       string `json:"cause,omitempty"` // TS 38.413 Handover cause
}

// HandoverResponse is returned on successful handover.
type HandoverResponse struct {
	Status      string `json:"status"`
	SUPI        string `json:"supi"`
	SourceGnbID string `json:"source_gnb_id"`
	TargetGnbID string `json:"target_gnb_id"`
}

// Handle processes an N2 Handover per TS 23.502 Section 4.9.1.
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var hoReq HandoverRequest
	if err := json.Unmarshal(req.Body, &hoReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if hoReq.SUPI == "" || hoReq.TargetGnbID == "" {
		return errorResp(http.StatusBadRequest, "supi and target_gnb_id are required"), nil
	}

	key := "ue:" + hoReq.SUPI

	var ueCtx models.UEContext
	if err := store.Get(ctx, key, &ueCtx); err != nil {
		return errorResp(http.StatusNotFound, "ue context not found: %s", err), nil
	}

	// Precondition: UE must be RM-REGISTERED and CM-CONNECTED for handover.
	if ueCtx.RegistrationState != "REGISTERED" {
		return errorResp(http.StatusConflict, "UE not registered (state: %s)", ueCtx.RegistrationState), nil
	}
	if ueCtx.CmState != "CONNECTED" {
		return errorResp(http.StatusConflict, "UE not connected (state: %s)", ueCtx.CmState), nil
	}

	sourceGnb := ueCtx.GnbID

	// Update UE context: move to target gNB.
	ueCtx.GnbID = hoReq.TargetGnbID
	ueCtx.LastActivity = time.Now()

	if err := store.Put(ctx, key, ueCtx); err != nil {
		return errorResp(http.StatusInternalServerError, "update ue context: %s", err), nil
	}

	body, _ := json.Marshal(HandoverResponse{
		Status:      "handover_complete",
		SUPI:        hoReq.SUPI,
		SourceGnbID: sourceGnb,
		TargetGnbID: hoReq.TargetGnbID,
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
