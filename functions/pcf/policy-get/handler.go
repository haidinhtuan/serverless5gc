package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	handler "github.com/openfaas/templates-sdk/go-http"
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

// PolicyDecision represents a QoS policy.
type PolicyDecision struct {
	PolicyID string `json:"policy_id"`
	QFI      uint8  `json:"qfi"`
	AMBRUL   uint64 `json:"ambr_ul"`
	AMBRDL   uint64 `json:"ambr_dl"`
	FiveQI   int    `json:"5qi"`
}

// Handle retrieves an existing policy by ID.
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	policyID := extractPolicyID(req)
	if policyID == "" {
		return errorResp(http.StatusBadRequest, "policy_id is required"), nil
	}

	key := "active-policies/" + policyID
	var policy PolicyDecision
	if err := Store.Get(ctx, key, &policy); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return errorResp(http.StatusNotFound, "policy %s not found", policyID), nil
		}
		return errorResp(http.StatusInternalServerError, "get policy: %s", err), nil
	}

	body, _ := json.Marshal(policy)
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     jsonHeader(),
	}, nil
}

func extractPolicyID(req handler.Request) string {
	// Try query string
	if req.QueryString != "" {
		params, err := url.ParseQuery(req.QueryString)
		if err == nil {
			if id := params.Get("policy_id"); id != "" {
				return id
			}
		}
	}
	// Try JSON body
	if len(req.Body) > 0 {
		var body struct {
			PolicyID string `json:"policy_id"`
		}
		if err := json.Unmarshal(req.Body, &body); err == nil && body.PolicyID != "" {
			return body.PolicyID
		}
	}
	return ""
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
