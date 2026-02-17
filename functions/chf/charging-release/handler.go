// Package function implements Nchf_ConvergedCharging_Release per 3GPP TS 32.291
// for finalizing a charging session and generating a CDR.
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

// ChargingReleaseRequest per TS 32.291 Section 6.1.6.2.4 (ChargingDataRequest for release).
type ChargingReleaseRequest struct {
	ChargingID string `json:"charging_id"`
}

// Handle processes Nchf_ConvergedCharging_Release (TS 32.291 Section 6.1.6.2).
// Finalizes a charging session, generates a CDR per TS 32.298, and removes the session.
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

	var rReq ChargingReleaseRequest
	if err := json.Unmarshal(req.Body, &rReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if rReq.ChargingID == "" {
		return errorResp(http.StatusBadRequest, "charging_id is required"), nil
	}

	key := "charging-sessions/" + rReq.ChargingID
	var session models.ChargingSession
	if err := Store.Get(ctx, key, &session); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return errorResp(http.StatusNotFound, "charging session %s not found", rReq.ChargingID), nil
		}
		return errorResp(http.StatusInternalServerError, "get session: %s", err), nil
	}

	session.State = "RELEASED"

	now := time.Now()
	recordID := "cdr-" + rReq.ChargingID
	cdr := models.ChargingDataRecord{
		RecordID:       recordID,
		ChargingID:     session.ChargingID,
		SUPI:           session.SUPI,
		PDUSessionID:   session.PDUSessionID,
		DNN:            session.DNN,
		SNSSAI:         session.SNSSAI,
		VolumeUplink:   session.VolumeUplink,
		VolumeDownlink: session.VolumeDownlink,
		Duration:       int64(now.Sub(session.CreatedAt).Seconds()),
		StartTime:      session.CreatedAt,
		EndTime:        now,
	}

	cdrKey := "charging-records/" + recordID
	if err := Store.Put(ctx, cdrKey, cdr); err != nil {
		return errorResp(http.StatusInternalServerError, "store CDR: %s", err), nil
	}

	if err := Store.Delete(ctx, key); err != nil {
		return errorResp(http.StatusInternalServerError, "delete session: %s", err), nil
	}

	body, _ := json.Marshal(cdr)
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
