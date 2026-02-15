package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/sbi"
	"github.com/tdinh/serverless5gc/pkg/state"
)

// SBICaller abstracts inter-NF communication for testability.
type SBICaller interface {
	CallFunction(funcName string, payload interface{}, result interface{}) error
}

var (
	store     state.KVStore
	sbiClient SBICaller
)

// SetStore injects a KVStore (used in tests).
func SetStore(s state.KVStore) { store = s }

// SetSBI injects an SBI caller (used in tests).
func SetSBI(s SBICaller) { sbiClient = s }

func init() {
	if store != nil {
		return
	}
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	store = state.NewRedisStore(addr)
	sbiClient = sbi.NewClient()
}

// AuthInitiateRequest is the JSON body for initiating authentication.
type AuthInitiateRequest struct {
	SUPI               string `json:"supi"`
	ServingNetworkName string `json:"serving_network_name"`
}

// AuthInitiateResponse is the auth challenge returned to the UE.
type AuthInitiateResponse struct {
	AuthType string `json:"auth_type"`
	RAND     string `json:"rand"`
	AUTN     string `json:"autn"`
	SUPI     string `json:"supi"`
}

type udmAuthDataResponse struct {
	AuthType string `json:"auth_type"`
	RAND     string `json:"rand"`
	AUTN     string `json:"autn"`
	XRESstar string `json:"xres_star"`
	KAUSF    string `json:"kausf"`
}

// Handle initiates authentication by calling UDM to generate auth data.
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	_ = ctx // store not directly used in this handler's main path

	var authReq AuthInitiateRequest
	if err := json.Unmarshal(req.Body, &authReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if authReq.SUPI == "" {
		return errorResp(http.StatusBadRequest, "supi is required"), nil
	}
	if authReq.ServingNetworkName == "" {
		authReq.ServingNetworkName = "5G:mnc001.mcc001.3gppnetwork.org"
	}

	// Call UDM generate-auth-data
	var udmResp udmAuthDataResponse
	if err := sbiClient.CallFunction("udm-generate-auth-data",
		map[string]string{
			"supi":                 authReq.SUPI,
			"serving_network_name": authReq.ServingNetworkName,
		},
		&udmResp); err != nil {
		return errorResp(http.StatusInternalServerError, "udm-generate-auth-data: %s", err), nil
	}

	// Store the expected XRES* for later verification (by ausf-authenticate)
	if udmResp.XRESstar != "" {
		storeKey := "auth-pending:" + authReq.SUPI
		_ = store.Put(ctx, storeKey, map[string]string{
			"xres_star": udmResp.XRESstar,
			"kausf":     udmResp.KAUSF,
		})
	}

	body, _ := json.Marshal(AuthInitiateResponse{
		AuthType: udmResp.AuthType,
		RAND:     udmResp.RAND,
		AUTN:     udmResp.AUTN,
		SUPI:     authReq.SUPI,
	})
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func errorResp(status int, format string, args ...interface{}) handler.Response {
	msg := fmt.Sprintf(format, args...)
	body, _ := json.Marshal(map[string]string{"error": msg})
	return handler.Response{StatusCode: status, Body: body}
}
