package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/models"
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

// Handle processes a UE service request.
// Updates CM state from IDLE to CONNECTED.
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

	if ueCtx.RegistrationState != "REGISTERED" {
		return errorResp(http.StatusConflict, "UE is not registered (state: %s)", ueCtx.RegistrationState), nil
	}

	ueCtx.CmState = "CONNECTED"
	ueCtx.LastActivity = time.Now()

	if err := store.Put(ctx, key, ueCtx); err != nil {
		return errorResp(http.StatusInternalServerError, "update ue context: %s", err), nil
	}

	body, _ := json.Marshal(map[string]string{
		"status":   "connected",
		"supi":     svcReq.SUPI,
		"cm_state": "CONNECTED",
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
