package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

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

// DeregistrationRequest is the JSON body for UE deregistration.
type DeregistrationRequest struct {
	SUPI string `json:"supi"`
}

// Handle processes a UE deregistration request.
// Reads the UE context, marks it as DEREGISTERED, and deletes from the store.
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
		return errorResp(http.StatusNotFound, "ue context not found: %s", err), nil
	}

	// Mark as deregistered
	ueCtx.RegistrationState = "DEREGISTERED"
	ueCtx.CmState = "IDLE"

	// Delete from store
	if err := store.Delete(ctx, key); err != nil {
		return errorResp(http.StatusInternalServerError, "delete ue context: %s", err), nil
	}

	body, _ := json.Marshal(map[string]string{
		"status": "deregistered",
		"supi":   deregReq.SUPI,
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
