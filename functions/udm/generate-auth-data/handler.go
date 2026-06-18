package function

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/haidinhtuan/serverless5gc/pkg/crypto"
	"github.com/haidinhtuan/serverless5gc/pkg/models"
	"github.com/haidinhtuan/serverless5gc/pkg/state"
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

// AuthDataRequest is the input to the UDM generate-auth-data function.
type AuthDataRequest struct {
	SUPI               string `json:"supi"`
	ServingNetworkName string `json:"serving_network_name"`
}

// AuthDataResponse is returned with the generated authentication vector.
type AuthDataResponse struct {
	AuthType string `json:"auth_type"`
	RAND     string `json:"rand"`
	AUTN     string `json:"autn"`
	XREStar  string `json:"xres_star"`
	KAUSF    string `json:"kausf"`
}

// Handle receives a SUPI, fetches subscriber auth data from the UDR store,
// generates a 5G-AKA auth vector, and returns it.
func Handle(req handler.Request) (handler.Response, error) {
	var authReq AuthDataRequest
	if err := json.Unmarshal(req.Body, &authReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if authReq.SUPI == "" {
		return errorResp(http.StatusBadRequest, "supi is required"), nil
	}
	if authReq.ServingNetworkName == "" {
		authReq.ServingNetworkName = "5G:mnc001.mcc001.3gppnetwork.org"
	}

	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Look up subscriber auth data
	var sub models.SubscriberData
	key := "subscribers/" + authReq.SUPI
	if err := Store.Get(ctx, key, &sub); err != nil {
		return errorResp(http.StatusNotFound, "subscriber %s not found: %s", authReq.SUPI, err), nil
	}

	if sub.AuthenticationData == nil {
		return errorResp(http.StatusNotFound, "no auth data for %s", authReq.SUPI), nil
	}

	authData := sub.AuthenticationData

	// Advance the sequence number before generating the vector so the AUTN
	// carries a SEQ the USIM will accept (TS 33.102 SQN management). Without a
	// fresh SEQ the AUTN repeats the stored value and the UE rejects it with
	// "SQN out of range".
	newSQN := advanceSQN(authData.SQN)

	av, err := crypto.GenerateAuthVector(
		authData.PermanentKey,
		authData.OPc,
		newSQN,
		authData.AMF,
		authReq.ServingNetworkName,
	)
	if err != nil {
		return errorResp(http.StatusInternalServerError, "generate auth vector: %s", err), nil
	}

	// Persist the advanced SQN so the next vector uses a strictly higher SEQ.
	authData.SQN = newSQN
	if err := Store.Put(ctx, key, &sub); err != nil {
		return errorResp(http.StatusInternalServerError, "persist SQN: %s", err), nil
	}

	// Store the auth vector for later verification by AUSF
	avKey := "auth-vectors/" + authReq.SUPI
	if err := Store.Put(ctx, avKey, av); err != nil {
		return errorResp(http.StatusInternalServerError, "store auth vector: %s", err), nil
	}

	resp := AuthDataResponse{
		AuthType: authData.AuthMethod,
		RAND:     hex.EncodeToString(av.RAND),
		AUTN:     hex.EncodeToString(av.AUTN),
		XREStar:  hex.EncodeToString(av.XRES),
		KAUSF:    hex.EncodeToString(av.KAUSF),
	}
	body, _ := json.Marshal(resp)
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     jsonHeader(),
	}, nil
}

// advanceSQN returns the 48-bit sequence number advanced by one SEQ step. The
// low 5 bits are the IND (TS 33.102); incrementing by 1<<5 keeps the IND fixed
// and raises SEQ by one, which the USIM requires to accept the next AUTN.
func advanceSQN(sqn []byte) []byte {
	var v uint64
	for _, b := range sqn {
		v = v<<8 | uint64(b)
	}
	const seqStep = 1 << 5
	v = (v + seqStep) & ((1 << 48) - 1)
	out := make([]byte, 6)
	for i := 5; i >= 0; i-- {
		out[i] = byte(v)
		v >>= 8
	}
	return out
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
