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
	"github.com/tdinh/serverless5gc/pkg/models"
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

// SubscriberDataResponse contains the access and mobility subscription data.
type SubscriberDataResponse struct {
	SUPI              string              `json:"supi"`
	AccessAndMobility *models.AccessMobData `json:"access_mobility_data,omitempty"`
	SessionManagement []models.SMPolicyData `json:"session_management,omitempty"`
}

// Handle returns the subscription data (NSSAI, DNN, QoS) for a given SUPI.
func Handle(req handler.Request) (handler.Response, error) {
	supi := extractSUPI(req)
	if supi == "" {
		return errorResp(http.StatusBadRequest, "supi parameter required"), nil
	}

	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var sub models.SubscriberData
	key := "subscribers/" + supi
	if err := Store.Get(ctx, key, &sub); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return errorResp(http.StatusNotFound, "subscriber %s not found", supi), nil
		}
		return errorResp(http.StatusInternalServerError, "%s", err), nil
	}

	resp := SubscriberDataResponse{
		SUPI:              sub.SUPI,
		AccessAndMobility: sub.AccessAndMobility,
		SessionManagement: sub.SessionManagement,
	}
	body, _ := json.Marshal(resp)
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     jsonHeader(),
	}, nil
}

func extractSUPI(req handler.Request) string {
	if req.QueryString != "" {
		params, err := url.ParseQuery(req.QueryString)
		if err == nil {
			if supi := params.Get("supi"); supi != "" {
				return supi
			}
		}
	}
	if len(req.Body) > 0 {
		var body struct {
			SUPI string `json:"supi"`
		}
		if err := json.Unmarshal(req.Body, &body); err == nil && body.SUPI != "" {
			return body.SUPI
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
