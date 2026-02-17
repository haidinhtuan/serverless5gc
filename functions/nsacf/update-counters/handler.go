// Package function implements Nnsacf_UpdateCounters per 3GPP TS 29.536
// to increment or decrement UE and PDU session counters for slice admission control.
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

// UpdateCountersRequest represents a counter update per TS 29.536.
type UpdateCountersRequest struct {
	SNSSAI      models.SNSSAI `json:"snssai"`
	CounterType string        `json:"counter_type"` // "UE" or "PDU_SESSION"
	Operation   string        `json:"operation"`     // "INCREMENT" or "DECREMENT"
}

func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// If NSACF is disabled, return disabled status
	if os.Getenv("ENABLE_NSACF") != "true" {
		body, _ := json.Marshal(map[string]string{"status": "disabled"})
		return handler.Response{
			StatusCode: http.StatusOK,
			Body:       body,
			Header:     jsonHeader(),
		}, nil
	}

	var updateReq UpdateCountersRequest
	if err := json.Unmarshal(req.Body, &updateReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if updateReq.SNSSAI.SST <= 0 {
		return errorResp(http.StatusBadRequest, "snssai.sst must be > 0"), nil
	}
	if updateReq.CounterType != "UE" && updateReq.CounterType != "PDU_SESSION" {
		return errorResp(http.StatusBadRequest, "counter_type must be UE or PDU_SESSION"), nil
	}
	if updateReq.Operation != "INCREMENT" && updateReq.Operation != "DECREMENT" {
		return errorResp(http.StatusBadRequest, "operation must be INCREMENT or DECREMENT"), nil
	}

	sliceKey := sliceKeyFromSNSSAI(updateReq.SNSSAI)
	countersKey := fmt.Sprintf("nsacf-counters/%s", sliceKey)

	var counters models.SliceCounters
	if err := Store.Get(ctx, countersKey, &counters); err != nil {
		counters = models.SliceCounters{
			SST: updateReq.SNSSAI.SST,
			SD:  updateReq.SNSSAI.SD,
		}
	}

	switch updateReq.CounterType {
	case "UE":
		if updateReq.Operation == "INCREMENT" {
			counters.CurrentUEs++
		} else {
			counters.CurrentUEs--
			if counters.CurrentUEs < 0 {
				counters.CurrentUEs = 0
			}
		}
	case "PDU_SESSION":
		if updateReq.Operation == "INCREMENT" {
			counters.CurrentSessions++
		} else {
			counters.CurrentSessions--
			if counters.CurrentSessions < 0 {
				counters.CurrentSessions = 0
			}
		}
	}

	if err := Store.Put(ctx, countersKey, counters); err != nil {
		return errorResp(http.StatusInternalServerError, "failed to update counters: %s", err), nil
	}

	body, _ := json.Marshal(counters)
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     jsonHeader(),
	}, nil
}

func sliceKeyFromSNSSAI(s models.SNSSAI) string {
	if s.SD == "" {
		return fmt.Sprintf("%d", s.SST)
	}
	return fmt.Sprintf("%d-%s", s.SST, s.SD)
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
