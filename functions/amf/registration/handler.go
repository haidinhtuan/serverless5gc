package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/models"
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

// RegistrationRequest is the JSON body for a UE registration.
type RegistrationRequest struct {
	SUPI        string `json:"supi"`
	RANUeNgapID int64  `json:"ran_ue_ngap_id"`
	GnbID       string `json:"gnb_id"`
}

// RegistrationResponse is returned on successful registration.
type RegistrationResponse struct {
	Status string `json:"status"`
	SUPI   string `json:"supi"`
	GUTI   string `json:"guti"`
}

type authResponse struct {
	AuthResult string `json:"auth_result"`
	SUPI       string `json:"supi"`
	KAUSF      string `json:"kausf"`
}

// Handle processes a UE initial registration request.
// Flow: authenticate via AUSF, fetch subscriber data via UDM, create UE context.
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var regReq RegistrationRequest
	if err := json.Unmarshal(req.Body, &regReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if regReq.SUPI == "" {
		return errorResp(http.StatusBadRequest, "supi is required"), nil
	}

	// Step 1: Authenticate via AUSF
	var authResult authResponse
	if err := sbiClient.CallFunction("ausf-authenticate",
		map[string]string{"supi": regReq.SUPI, "res_star": ""},
		&authResult); err != nil {
		return errorResp(http.StatusInternalServerError, "ausf-authenticate: %s", err), nil
	}
	if authResult.AuthResult != "SUCCESS" {
		return errorResp(http.StatusForbidden, "authentication failed for %s", regReq.SUPI), nil
	}

	// Step 2: Get subscriber data from UDM
	var subData models.SubscriberData
	if err := sbiClient.CallFunction("udm-get-subscriber-data",
		map[string]string{"supi": regReq.SUPI},
		&subData); err != nil {
		return errorResp(http.StatusInternalServerError, "udm-get-subscriber-data: %s", err), nil
	}

	// Step 3: Create UE context
	var nssai []models.SNSSAI
	if subData.AccessAndMobility != nil {
		nssai = subData.AccessAndMobility.NSSAI
	}

	guti := fmt.Sprintf("5g-guti-%s", regReq.SUPI)
	ueCtx := models.UEContext{
		SUPI:              regReq.SUPI,
		GUTI:              guti,
		RegistrationState: "REGISTERED",
		CmState:           "CONNECTED",
		RANUeNgapID:       regReq.RANUeNgapID,
		GnbID:             regReq.GnbID,
		NSSAI:             nssai,
		LastActivity:      time.Now(),
	}

	if err := store.Put(ctx, "ue:"+regReq.SUPI, ueCtx); err != nil {
		return errorResp(http.StatusInternalServerError, "store ue context: %s", err), nil
	}

	body, _ := json.Marshal(RegistrationResponse{
		Status: "registered",
		SUPI:   regReq.SUPI,
		GUTI:   guti,
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
