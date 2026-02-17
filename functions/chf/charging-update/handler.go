// Package function implements Nchf_ConvergedCharging_Update per 3GPP TS 32.291
// for reporting usage and requesting additional quota.
package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/models"
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

// ChargingUpdateRequest per TS 32.291 Section 6.1.6.2.3 (ChargingDataRequest for update).
type ChargingUpdateRequest struct {
	ChargingID     string `json:"charging_id"`
	VolumeUplink   uint64 `json:"volume_uplink"`
	VolumeDownlink uint64 `json:"volume_downlink"`
}

// ChargingUpdateResponse includes the updated session and quota grant info.
type ChargingUpdateResponse struct {
	models.ChargingSession
	AdditionalQuotaGranted bool `json:"additional_quota_granted"`
}

const quotaIncrement = 1000000

// Handle processes Nchf_ConvergedCharging_Update (TS 32.291 Section 6.1.6.2).
// Reports usage volumes and grants additional quota when usage exceeds the limit.
func Handle(req handler.Request) (handler.Response, error) {
	if os.Getenv("ENABLE_CHARGING") != "true" {
		return handler.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(`{"status":"disabled"}`),
			Header:     jsonHeader(),
		}, nil
	}

	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var uReq ChargingUpdateRequest
	if err := json.Unmarshal(req.Body, &uReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if uReq.ChargingID == "" {
		return errorResp(http.StatusBadRequest, "charging_id is required"), nil
	}

	key := "charging-sessions/" + uReq.ChargingID
	var session models.ChargingSession
	if err := Store.Get(ctx, key, &session); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return errorResp(http.StatusNotFound, "charging session %s not found", uReq.ChargingID), nil
		}
		return errorResp(http.StatusInternalServerError, "get session: %s", err), nil
	}

	session.VolumeUplink += uReq.VolumeUplink
	session.VolumeDownlink += uReq.VolumeDownlink

	additionalGranted := false
	totalUsage := session.VolumeUplink + session.VolumeDownlink
	if totalUsage > session.GrantedUnits {
		session.GrantedUnits += quotaIncrement
		additionalGranted = true
	}

	session.LastUpdated = time.Now()

	if err := Store.Put(ctx, key, session); err != nil {
		return errorResp(http.StatusInternalServerError, "update session: %s", err), nil
	}

	resp := ChargingUpdateResponse{
		ChargingSession:        session,
		AdditionalQuotaGranted: additionalGranted,
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
