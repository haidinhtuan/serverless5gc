package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/pfcp"
	"github.com/tdinh/serverless5gc/pkg/state"
)

// PFCPEstablisher abstracts PFCP session establishment for testability.
type PFCPEstablisher interface {
	EstablishSession(seid uint64, ueIP string, teid uint32) error
}

// Store is the backing KV store. Override in tests via SetStore.
var Store state.KVStore

// PFCP is the PFCP client. Override in tests via SetPFCP.
var PFCP PFCPEstablisher

func SetStore(s state.KVStore) { Store = s }
func SetPFCP(p PFCPEstablisher) { PFCP = p }

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

// N4SetupRequest contains parameters for PFCP session establishment.
type N4SetupRequest struct {
	SEID uint64 `json:"seid"`
	UEIP string `json:"ue_ip"`
	TEID uint32 `json:"teid"`
}

// N4SetupResponse is returned after successful PFCP session setup.
type N4SetupResponse struct {
	SEID   uint64 `json:"seid"`
	Status string `json:"status"`
}

// Handle is an internal helper that establishes a PFCP session with the UPF.
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	_ = ctx // available for future store operations

	var n4Req N4SetupRequest
	if err := json.Unmarshal(req.Body, &n4Req); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if n4Req.UEIP == "" {
		return errorResp(http.StatusBadRequest, "ue_ip is required"), nil
	}
	if n4Req.SEID == 0 {
		return errorResp(http.StatusBadRequest, "seid is required"), nil
	}

	if PFCP != nil {
		if err := PFCP.EstablishSession(n4Req.SEID, n4Req.UEIP, n4Req.TEID); err != nil {
			return errorResp(http.StatusInternalServerError, "pfcp establish: %s", err), nil
		}
	}

	resp := N4SetupResponse{
		SEID:   n4Req.SEID,
		Status: "ESTABLISHED",
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

// Ensure pfcp.Client satisfies PFCPEstablisher.
var _ PFCPEstablisher = (*pfcp.Client)(nil)
