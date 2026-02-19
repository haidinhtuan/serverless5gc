// Package function implements Nnssf_NSSelection per 3GPP TS 29.531
// and TS 23.502 Section 4.3.2 (network slice selection during registration).
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

// ConfiguredSlices holds the S-NSSAI values configured on this network
// per TS 23.501 Section 5.15.2.
var ConfiguredSlices = []models.SNSSAI{
	{SST: 1, SD: "010203"}, // eMBB
	{SST: 1, SD: "112233"}, // eMBB variant
	{SST: 2, SD: "010203"}, // URLLC
	{SST: 3, SD: "010203"}, // mMTC
}

// SliceSelectRequest per TS 29.531 Section 6.1.6.2.3 (Nnssf_NSSelection).
type SliceSelectRequest struct {
	RequestedNSSAI []models.SNSSAI `json:"requested_nssai"`
	PLMN           models.PlmnID   `json:"plmn,omitempty"`
}

// SliceSelectResponse per TS 29.531 Section 6.1.6.2.3 (AuthorizedNetworkSliceInfo).
type SliceSelectResponse struct {
	AllowedNSSAI  []models.SNSSAI `json:"allowed_nssai"`
	RejectedNSSAI []models.SNSSAI `json:"rejected_nssai,omitempty"`
}

// Handle processes Nnssf_NSSelection (TS 29.531) to determine Allowed NSSAI
// from Requested NSSAI per TS 23.502 Section 4.3.2.
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var ssReq SliceSelectRequest
	if err := json.Unmarshal(req.Body, &ssReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if len(ssReq.RequestedNSSAI) == 0 {
		return errorResp(http.StatusBadRequest, "requested_nssai is required"), nil
	}

	// Try to load configured slices from store
	configured := ConfiguredSlices
	var storedSlices []models.SNSSAI
	if err := Store.Get(ctx, "nssf/configured-slices", &storedSlices); err == nil && len(storedSlices) > 0 {
		configured = storedSlices
	}

	// Build a set of configured slices for fast lookup
	configSet := make(map[string]bool)
	for _, s := range configured {
		key := fmt.Sprintf("%d-%s", s.SST, s.SD)
		configSet[key] = true
	}

	var allowed, rejected []models.SNSSAI
	for _, requested := range ssReq.RequestedNSSAI {
		key := fmt.Sprintf("%d-%s", requested.SST, requested.SD)
		if configSet[key] {
			allowed = append(allowed, requested)
		} else {
			// Also match by SST-only if SD is empty in configured
			sstKey := fmt.Sprintf("%d-", requested.SST)
			if configSet[sstKey] {
				allowed = append(allowed, requested)
			} else {
				rejected = append(rejected, requested)
			}
		}
	}

	resp := SliceSelectResponse{
		AllowedNSSAI:  allowed,
		RejectedNSSAI: rejected,
	}
	if len(allowed) == 0 {
		resp.AllowedNSSAI = []models.SNSSAI{} // never nil
	}

	body, _ := json.Marshal(resp)
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
