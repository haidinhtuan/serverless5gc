// Package function implements Nbsf_Management_Discovery per 3GPP TS 29.521
// to discover a PCF binding by UE IP address or SUPI+DNN.
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

// BindingDiscoverRequest per TS 29.521 Section 5.3.3.
type BindingDiscoverRequest struct {
	UEAddress string `json:"ue_address,omitempty"`
	SUPI      string `json:"supi,omitempty"`
	DNN       string `json:"dnn,omitempty"`
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

	var discReq BindingDiscoverRequest
	if err := json.Unmarshal(req.Body, &discReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if discReq.UEAddress == "" && discReq.SUPI == "" {
		return errorResp(http.StatusBadRequest, "at least one of ue_address or supi is required"), nil
	}

	var bindingID string

	if discReq.UEAddress != "" {
		ipKey := fmt.Sprintf("bsf-by-ip/%s", discReq.UEAddress)
		if err := Store.Get(ctx, ipKey, &bindingID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return errorResp(http.StatusNotFound, "no binding for ue_address %s", discReq.UEAddress), nil
			}
			return errorResp(http.StatusInternalServerError, "store error: %s", err), nil
		}
	} else {
		dnn := discReq.DNN
		if dnn == "" {
			dnn = "internet"
		}
		supiKey := fmt.Sprintf("bsf-by-supi/%s/%s", discReq.SUPI, dnn)
		if err := Store.Get(ctx, supiKey, &bindingID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return errorResp(http.StatusNotFound, "no binding for supi %s dnn %s", discReq.SUPI, dnn), nil
			}
			return errorResp(http.StatusInternalServerError, "store error: %s", err), nil
		}
	}

	var binding models.PCFBinding
	primaryKey := fmt.Sprintf("bsf-bindings/%s", bindingID)
	if err := Store.Get(ctx, primaryKey, &binding); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return errorResp(http.StatusNotFound, "binding %s not found", bindingID), nil
		}
		return errorResp(http.StatusInternalServerError, "store error: %s", err), nil
	}

	body, _ := json.Marshal(binding)
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
