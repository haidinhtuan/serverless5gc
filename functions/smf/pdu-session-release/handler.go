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

// PFCPDeleter abstracts PFCP deletion for testability.
type PFCPDeleter interface {
	DeleteSession(seid uint64) error
}

// Store is the backing KV store. Override in tests via SetStore.
var Store state.KVStore

// PFCP is the PFCP client. Override in tests via SetPFCP.
var PFCP PFCPDeleter

func SetStore(s state.KVStore) { Store = s }
func SetPFCP(p PFCPDeleter) { PFCP = p }

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

// ReleaseSMContextRequest is the input for session release.
type ReleaseSMContextRequest struct {
	SessionID string `json:"session_id"`
}

// Handle releases a PDU session: sends PFCP deletion and removes from store.
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var relReq ReleaseSMContextRequest
	if err := json.Unmarshal(req.Body, &relReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if relReq.SessionID == "" {
		return errorResp(http.StatusBadRequest, "session_id is required"), nil
	}

	// Fetch existing session to verify it exists
	key := "pdu-sessions/" + relReq.SessionID
	var session models.PDUSession
	if err := Store.Get(ctx, key, &session); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return errorResp(http.StatusNotFound, "session %s not found", relReq.SessionID), nil
		}
		return errorResp(http.StatusInternalServerError, "get session: %s", err), nil
	}

	// Send PFCP Session Deletion to UPF
	if PFCP != nil {
		seid := extractSEID(relReq.SessionID)
		if err := PFCP.DeleteSession(seid); err != nil {
			return errorResp(http.StatusInternalServerError, "pfcp delete: %s", err), nil
		}
	}

	// Remove session from store
	if err := Store.Delete(ctx, key); err != nil {
		return errorResp(http.StatusInternalServerError, "delete session: %s", err), nil
	}

	body, _ := json.Marshal(map[string]string{
		"session_id": relReq.SessionID,
		"state":      "RELEASED",
	})
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     jsonHeader(),
	}, nil
}

func extractSEID(sessionID string) uint64 {
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
