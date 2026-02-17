// Package function implements Nchf_ConvergedCharging_Create per 3GPP TS 32.291
// and TS 23.502 Section 4.16.
package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

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

var (
	chargingCounter uint64
	chargingMu      sync.Mutex
)

// ChargingCreateRequest per TS 32.291 Section 6.1.6.2.2 (ChargingDataRequest).
type ChargingCreateRequest struct {
	SUPI         string        `json:"supi"`
	PDUSessionID string        `json:"pdu_session_id"`
	DNN          string        `json:"dnn"`
	SNSSAI       models.SNSSAI `json:"snssai"`
	ChargingType string        `json:"charging_type,omitempty"`
}

const defaultGrantedUnits = 1000000

// Handle processes Nchf_ConvergedCharging_Create (TS 32.291 Section 6.1.6.2).
// Creates a new converged charging session for a PDU session.
func Handle(req handler.Request) (handler.Response, error) {
	if os.Getenv("ENABLE_CHARGING") != "true" {
		return handler.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"status":"disabled"}`),
			Header:     jsonHeader(),
		}, nil
	}

	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var cReq ChargingCreateRequest
	if err := json.Unmarshal(req.Body, &cReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if cReq.SUPI == "" {
		return errorResp(http.StatusBadRequest, "supi is required"), nil
	}
	if cReq.PDUSessionID == "" {
		return errorResp(http.StatusBadRequest, "pdu_session_id is required"), nil
	}
	if cReq.ChargingType == "" {
		cReq.ChargingType = "OFFLINE"
	}

	chargingMu.Lock()
	chargingCounter++
	id := chargingCounter
	chargingMu.Unlock()

	chargingID := fmt.Sprintf("chg-%s-%d", cReq.SUPI, id)
	now := time.Now()

	session := models.ChargingSession{
		ChargingID:   chargingID,
		SUPI:         cReq.SUPI,
		PDUSessionID: cReq.PDUSessionID,
		DNN:          cReq.DNN,
		SNSSAI:       cReq.SNSSAI,
		ChargingType: cReq.ChargingType,
		State:        "ACTIVE",
		GrantedUnits: defaultGrantedUnits,
		CreatedAt:    now,
		LastUpdated:  now,
	}

	key := "charging-sessions/" + chargingID
	if err := Store.Put(ctx, key, session); err != nil {
		return errorResp(http.StatusInternalServerError, "store session: %s", err), nil
	}

	body, _ := json.Marshal(session)
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
