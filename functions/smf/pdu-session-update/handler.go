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
	"github.com/tdinh/serverless5gc/pkg/pfcp"
	"github.com/tdinh/serverless5gc/pkg/state"
)

// PFCPModifier abstracts PFCP modification for testability.
type PFCPModifier interface {
	ModifySession(seid uint64, params pfcp.ModifyParams) error
}

// Store is the backing KV store. Override in tests via SetStore.
var Store state.KVStore

// PFCP is the PFCP client. Override in tests via SetPFCP.
var PFCP PFCPModifier

func SetStore(s state.KVStore) { Store = s }
func SetPFCP(p PFCPModifier) { PFCP = p }

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

// UpdateSMContextRequest is the input for session modification.
type UpdateSMContextRequest struct {
	SessionID string `json:"session_id"`
	AMBRUL    uint64 `json:"ambr_ul,omitempty"`
	AMBRDL    uint64 `json:"ambr_dl,omitempty"`
	QFI       uint8  `json:"qfi,omitempty"`
}

// Handle modifies a PDU session's QoS and sends PFCP Session Modification.
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var updateReq UpdateSMContextRequest
	if err := json.Unmarshal(req.Body, &updateReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if updateReq.SessionID == "" {
		return errorResp(http.StatusBadRequest, "session_id is required"), nil
	}

	// Fetch existing session
	key := "pdu-sessions/" + updateReq.SessionID
	var session models.PDUSession
	if err := Store.Get(ctx, key, &session); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return errorResp(http.StatusNotFound, "session %s not found", updateReq.SessionID), nil
		}
		return errorResp(http.StatusInternalServerError, "get session: %s", err), nil
	}

	// Apply updates
	if updateReq.AMBRUL > 0 {
		session.AMBRUL = updateReq.AMBRUL
	}
	if updateReq.AMBRDL > 0 {
		session.AMBRDL = updateReq.AMBRDL
	}
	if updateReq.QFI > 0 {
		session.QFI = updateReq.QFI
	}

	// Send PFCP Session Modification to UPF
	if PFCP != nil {
		params := pfcp.ModifyParams{
			AMBRUL: session.AMBRUL,
			AMBRDL: session.AMBRDL,
			QFI:    session.QFI,
		}
		// Extract SEID from session ID (last segment after last dash)
		seid := extractSEID(updateReq.SessionID)
		if err := PFCP.ModifySession(seid, params); err != nil {
			return errorResp(http.StatusInternalServerError, "pfcp modify: %s", err), nil
		}
	}

	// Update session in store
	if err := Store.Put(ctx, key, session); err != nil {
		return errorResp(http.StatusInternalServerError, "update session: %s", err), nil
	}

	body, _ := json.Marshal(session)
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     jsonHeader(),
	}, nil
}

func extractSEID(sessionID string) uint64 {
	// Session IDs are formatted as "pdu-<supi>-<seid>"
	parts := strings.Split(sessionID, "-")
	if len(parts) < 2 {
		return 0
	}
	var seid uint64
	fmt.Sscanf(parts[len(parts)-1], "%d", &seid)
	return seid
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
