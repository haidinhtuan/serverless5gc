// Package function implements Nbsf_Management_Deregister per 3GPP TS 29.521
// to remove a PCF-to-PDU-session binding from the BSF.
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
	"github.com/tdinh/serverless5gc/pkg/state"
)

// Store is the backing KV store. Override in tests via SetStore.
var Store state.KVStore

func SetStore(s state.KVStore) { Store = s }

func init() {
	if Store != nil {
		return
	}
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	Store = state.NewRedisStore(addr)
}

// BindingDeregisterRequest per TS 29.521 Section 5.3.4.
type BindingDeregisterRequest struct {
	BindingID string `json:"binding_id"`
}

func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	if os.Getenv("ENABLE_BSF") != "true" {
		body, _ := json.Marshal(map[string]string{"status": "disabled"})
		return handler.Response{
			StatusCode: http.StatusOK,
			Body:       body,
			Header:     jsonHeader(),
		}, nil
	}

	var deregReq BindingDeregisterRequest
	if err := json.Unmarshal(req.Body, &deregReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if deregReq.BindingID == "" {
		return errorResp(http.StatusBadRequest, "binding_id is required"), nil
	}

	// Retrieve the binding to get index keys
	primaryKey := fmt.Sprintf("bsf-bindings/%s", deregReq.BindingID)
	var binding models.PCFBinding
	if err := Store.Get(ctx, primaryKey, &binding); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return errorResp(http.StatusNotFound, "binding %s not found", deregReq.BindingID), nil
		}
		return errorResp(http.StatusInternalServerError, "store error: %s", err), nil
	}

	// Delete IP index if present
	if binding.UEAddress != "" {
		ipKey := fmt.Sprintf("bsf-by-ip/%s", binding.UEAddress)
		Store.Delete(ctx, ipKey)
	}

	// Delete SUPI+DNN index
	supiKey := fmt.Sprintf("bsf-by-supi/%s/%s", binding.SUPI, binding.DNN)
	Store.Delete(ctx, supiKey)

	// Delete primary record
	Store.Delete(ctx, primaryKey)

	result := map[string]string{
		"binding_id": deregReq.BindingID,
		"status":     "deregistered",
	}
	body, _ := json.Marshal(result)
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     jsonHeader(),
	}, nil
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
