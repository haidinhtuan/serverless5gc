// Package function implements Nnsacf_SliceAvailabilityCheck per 3GPP TS 29.536
// to determine whether a network slice can admit a new UE or PDU session.
package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

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

// AvailabilityCheckRequest represents a slice admission check per TS 29.536.
type AvailabilityCheckRequest struct {
	SNSSAI    models.SNSSAI `json:"snssai"`
	CheckType string        `json:"check_type"` // "UE" or "PDU_SESSION"
}

// AvailabilityCheckResponse contains the admission decision.
type AvailabilityCheckResponse struct {
	Allowed   bool          `json:"allowed"`
	SNSSAI    models.SNSSAI `json:"snssai"`
	CheckType string        `json:"check_type"`
	Current   int64         `json:"current"`
	Max       int64         `json:"max"`
	Cause     string        `json:"cause,omitempty"`
}

func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// If NSACF is disabled, always allow
	if os.Getenv("ENABLE_NSACF") != "true" {
		resp := AvailabilityCheckResponse{Allowed: true}
		body, _ := json.Marshal(resp)
		return handler.Response{
			StatusCode: http.StatusOK,
			Body:       body,
			Header:     jsonHeader(),
		}, nil
	}

	var checkReq AvailabilityCheckRequest
	if err := json.Unmarshal(req.Body, &checkReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if checkReq.SNSSAI.SST <= 0 {
		return errorResp(http.StatusBadRequest, "snssai.sst must be > 0"), nil
	}
	if checkReq.CheckType != "UE" && checkReq.CheckType != "PDU_SESSION" {
		return errorResp(http.StatusBadRequest, "check_type must be UE or PDU_SESSION"), nil
	}

	sliceKey := sliceKeyFromSNSSAI(checkReq.SNSSAI)

	// Retrieve admission policy; no policy means no limit
	var policy models.SliceAdmissionPolicy
	policyKey := fmt.Sprintf("nsacf-policies/%s", sliceKey)
	if err := Store.Get(ctx, policyKey, &policy); err != nil {
		resp := AvailabilityCheckResponse{
			Allowed:   true,
			SNSSAI:    checkReq.SNSSAI,
			CheckType: checkReq.CheckType,
		}
		body, _ := json.Marshal(resp)
		return handler.Response{
			StatusCode: http.StatusOK,
			Body:       body,
			Header:     jsonHeader(),
		}, nil
	}

	// Retrieve current counters; missing means zero
	var counters models.SliceCounters
	countersKey := fmt.Sprintf("nsacf-counters/%s", sliceKey)
	if err := Store.Get(ctx, countersKey, &counters); err != nil {
		counters = models.SliceCounters{
			SST: checkReq.SNSSAI.SST,
			SD:  checkReq.SNSSAI.SD,
		}
	}

	var current, max int64
	allowed := true

	switch checkReq.CheckType {
	case "UE":
		current = counters.CurrentUEs
		max = policy.MaxUEs
		if current >= max {
			allowed = false
		}
	case "PDU_SESSION":
		current = counters.CurrentSessions
		max = policy.MaxSessions
		if current >= max {
			allowed = false
		}
	}

	resp := AvailabilityCheckResponse{
		Allowed:   allowed,
		SNSSAI:    checkReq.SNSSAI,
		CheckType: checkReq.CheckType,
		Current:   current,
		Max:       max,
	}

	statusCode := http.StatusOK
	if !allowed {
		statusCode = http.StatusForbidden
		resp.Cause = "SLICE_NOT_AVAILABLE"
	}

	body, _ := json.Marshal(resp)
	return handler.Response{
		StatusCode: statusCode,
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
