// Package function implements Nbsf_Management_Register per 3GPP TS 29.521
// to create a PCF-to-PDU-session binding in the BSF.
package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/haidinhtuan/serverless5gc/pkg/models"
	"github.com/haidinhtuan/serverless5gc/pkg/state"
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

// BindingRegisterRequest per TS 29.521 Section 5.3.2 (PcfBinding).
type BindingRegisterRequest struct {
	SUPI         string        `json:"supi"`
	DNN          string        `json:"dnn"`
	SNSSAI       models.SNSSAI `json:"snssai"`
	PCFAddress   string        `json:"pcf_address"`
	UEAddress    string        `json:"ue_address,omitempty"`
	PDUSessionID string        `json:"pdu_session_id"`
}

var (
	bindingCounter uint64
	counterMu      sync.Mutex
)

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

	var regReq BindingRegisterRequest
	if err := json.Unmarshal(req.Body, &regReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if regReq.SUPI == "" {
		return errorResp(http.StatusBadRequest, "supi is required"), nil
	}
	if regReq.PDUSessionID == "" {
		return errorResp(http.StatusBadRequest, "pdu_session_id is required"), nil
	}

	if regReq.DNN == "" {
		regReq.DNN = "internet"
	}
	if regReq.PCFAddress == "" {
		regReq.PCFAddress = "pcf-001"
	}

	counterMu.Lock()
	bindingCounter++
	id := bindingCounter
	counterMu.Unlock()

	bindingID := fmt.Sprintf("bsf-%s-%d", regReq.SUPI, id)

	binding := models.PCFBinding{
		BindingID:    bindingID,
		SUPI:         regReq.SUPI,
		DNN:          regReq.DNN,
		SNSSAI:       regReq.SNSSAI,
		PCFAddress:   regReq.PCFAddress,
		UEAddress:    regReq.UEAddress,
		PDUSessionID: regReq.PDUSessionID,
	}

	// Store primary binding record
	primaryKey := fmt.Sprintf("bsf-bindings/%s", bindingID)
	if err := Store.Put(ctx, primaryKey, binding); err != nil {
		return errorResp(http.StatusInternalServerError, "failed to store binding: %s", err), nil
	}

	// Store IP-based index for discovery
	if regReq.UEAddress != "" {
		ipKey := fmt.Sprintf("bsf-by-ip/%s", regReq.UEAddress)
		if err := Store.Put(ctx, ipKey, bindingID); err != nil {
			return errorResp(http.StatusInternalServerError, "failed to store ip index: %s", err), nil
		}
	}

	// Store SUPI+DNN index for discovery
	supiKey := fmt.Sprintf("bsf-by-supi/%s/%s", regReq.SUPI, regReq.DNN)
	if err := Store.Put(ctx, supiKey, bindingID); err != nil {
		return errorResp(http.StatusInternalServerError, "failed to store supi index: %s", err), nil
	}

	body, _ := json.Marshal(binding)
	return handler.Response{
		StatusCode: http.StatusCreated,
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
