// Package function implements Npcf_SMPolicyControl_Create per 3GPP TS 29.512
// and TS 23.502 Section 4.16.4.
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

// PolicyCreateRequest per TS 29.512 Section 5.6.2.1 (SmPolicyContextData).
type PolicyCreateRequest struct {
	SUPI   string        `json:"supi"`
	SNSSAI models.SNSSAI `json:"snssai"`
	DNN    string        `json:"dnn"`
}

// PolicyDecision per TS 29.512 Section 5.6.2.2 (SmPolicyDecision).
type PolicyDecision struct {
	PolicyID  string                `json:"policy_id"`
	QFI       uint8                 `json:"qfi"`
	AMBRUL    uint64                `json:"ambr_ul"`
	AMBRDL    uint64                `json:"ambr_dl"`
	FiveQI    int                   `json:"5qi"`
	SessRules map[string]SessionRule `json:"sess_rules,omitempty"`
}

// SessionRule per TS 29.512 Section 5.6.2.7.
type SessionRule struct {
	SessionAMBR *AMBR `json:"sess_ambr,omitempty"`
	DefQFI      uint8 `json:"def_qos_flow_indication,omitempty"`
}

// AMBR per TS 29.571.
type AMBR struct {
	Uplink   uint64 `json:"uplink"`
	Downlink uint64 `json:"downlink"`
}

// Default QoS profiles per slice type (TS 23.501 Section 5.7).
var defaultPolicies = map[int32]PolicyDecision{
	1: { // eMBB: 5QI=9 (best effort internet), QFI=1 (default flow)
		QFI:    1,
		AMBRUL: 1000000,  // 1 Mbps
		AMBRDL: 5000000,  // 5 Mbps
		FiveQI: 9,
	},
	2: { // URLLC: 5QI=7 (voice/video), low latency
		QFI:    7,
		AMBRUL: 500000,   // 500 kbps
		AMBRDL: 500000,
		FiveQI: 7,
	},
	3: { // mMTC: 5QI=9, low throughput for IoT
		QFI:    9,
		AMBRUL: 100000,   // 100 kbps
		AMBRDL: 100000,
		FiveQI: 9,
	},
}

var policyCounter uint64

// Handle processes Npcf_SMPolicyControl_Create (TS 29.512 Section 5.6.2).
// Returns an SmPolicyDecision containing session rules and default QoS parameters.
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var pcfReq PolicyCreateRequest
	if err := json.Unmarshal(req.Body, &pcfReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if pcfReq.SUPI == "" {
		return errorResp(http.StatusBadRequest, "supi is required"), nil
	}

	// Try to load a configured policy from store
	policyKey := fmt.Sprintf("policies/sst-%d-sd-%s-dnn-%s", pcfReq.SNSSAI.SST, pcfReq.SNSSAI.SD, pcfReq.DNN)
	var policy PolicyDecision
	if err := Store.Get(ctx, policyKey, &policy); err != nil {
		// Fall back to default policy based on SST
		def, ok := defaultPolicies[pcfReq.SNSSAI.SST]
		if !ok {
			def = defaultPolicies[1] // default to eMBB
		}
		policy = def
	}

	policyCounter++
	policy.PolicyID = fmt.Sprintf("pol-%s-%d", pcfReq.SUPI, policyCounter)

	// Build session rules per TS 29.512 Section 5.6.2.7
	policy.SessRules = map[string]SessionRule{
		"sr-1": {
			SessionAMBR: &AMBR{Uplink: policy.AMBRUL, Downlink: policy.AMBRDL},
			DefQFI:      policy.QFI,
		},
	}

	// Store the created policy for Npcf_SMPolicyControl_Get (TS 29.512)
	storedKey := "active-policies/" + policy.PolicyID
	Store.Put(ctx, storedKey, policy)

	body, _ := json.Marshal(policy)
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
