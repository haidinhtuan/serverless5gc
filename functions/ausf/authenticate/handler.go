package function

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/crypto"
	"github.com/tdinh/serverless5gc/pkg/state"
)

// Store is the backing KV store. Override in tests via SetStore.
var Store state.KVStore

// SetStore replaces the store (used for testing).
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

// AuthRequest is the input from the UE (via AMF) for authentication verification.
type AuthRequest struct {
	SUPI    string `json:"supi"`
	RESstar string `json:"res_star"` // hex-encoded RES* from the UE
}

// AuthResponse is the authentication result.
type AuthResponse struct {
	AuthResult string `json:"auth_result"` // SUCCESS or FAILURE
	SUPI       string `json:"supi"`
	KAUSF      string `json:"kausf,omitempty"` // hex-encoded, returned on success
}

// Handle receives an authentication response (RES*) from the UE,
// compares it with the stored XRES*, and returns the auth result.
func Handle(req handler.Request) (handler.Response, error) {
	var authReq AuthRequest
	if err := json.Unmarshal(req.Body, &authReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if authReq.SUPI == "" || authReq.RESstar == "" {
		return errorResp(http.StatusBadRequest, "supi and res_star are required"), nil
	}

	resStar, err := hex.DecodeString(authReq.RESstar)
	if err != nil {
		return errorResp(http.StatusBadRequest, "invalid res_star hex: %s", err), nil
	}

	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Retrieve stored auth vector
	var av crypto.AuthVector
	avKey := "auth-vectors/" + authReq.SUPI
	if err := Store.Get(ctx, avKey, &av); err != nil {
		return errorResp(http.StatusNotFound, "no auth vector for %s: %s", authReq.SUPI, err), nil
	}

	// Compare RES* with XRES* using constant-time comparison
	if subtle.ConstantTimeCompare(resStar, av.XRES) != 1 {
		resp := AuthResponse{
			AuthResult: "FAILURE",
			SUPI:       authReq.SUPI,
		}
		body, _ := json.Marshal(resp)
		return handler.Response{
			StatusCode: http.StatusOK,
			Body:       body,
			Header:     jsonHeader(),
		}, nil
	}

	// Authentication successful - clean up stored vector
	_ = Store.Delete(ctx, avKey)

	resp := AuthResponse{
		AuthResult: "SUCCESS",
		SUPI:       authReq.SUPI,
		KAUSF:      hex.EncodeToString(av.KAUSF),
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
