// Package function implements Nnwdaf_AnalyticsSubscription per 3GPP TS 29.520
// to create analytics subscriptions and return current analytics snapshots.
package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/haidinhtuan/serverless5gc/pkg/models"
	"github.com/haidinhtuan/serverless5gc/pkg/state"
)

var Store state.KVStore

func SetStore(s state.KVStore) { Store = s }

var (
	subCounter uint64
	subMu      sync.Mutex
)

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

type AnalyticsSubscribeRequest struct {
	EventID         string         `json:"event_id"`
	TargetNF        string         `json:"target_nf,omitempty"`
	SNSSAI          *models.SNSSAI `json:"snssai,omitempty"`
	NotificationURI string         `json:"notification_uri,omitempty"`
}

type AnalyticsSubscribeResponse struct {
	SubscriptionID string      `json:"subscription_id"`
	EventID        string      `json:"event_id"`
	AnalyticsData  interface{} `json:"analytics_data,omitempty"`
}

func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var subReq AnalyticsSubscribeRequest
	if err := json.Unmarshal(req.Body, &subReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}

	if subReq.EventID != "NF_LOAD" && subReq.EventID != "SLICE_LOAD" && subReq.EventID != "UE_MOBILITY" {
		return errorResp(http.StatusBadRequest, "event_id must be NF_LOAD, SLICE_LOAD, or UE_MOBILITY"), nil
	}

	subMu.Lock()
	subCounter++
	subID := fmt.Sprintf("nwdaf-sub-%d", subCounter)
	subMu.Unlock()

	sub := models.AnalyticsSubscription{
		SubscriptionID:  subID,
		EventID:         subReq.EventID,
		TargetNF:        subReq.TargetNF,
		SNSSAI:          subReq.SNSSAI,
		NotificationURI: subReq.NotificationURI,
	}

	subKey := fmt.Sprintf("nwdaf-subscriptions/%s", subID)
	if err := Store.Put(ctx, subKey, sub); err != nil {
		return errorResp(http.StatusInternalServerError, "store put: %s", err), nil
	}

	var analyticsData interface{}

	switch subReq.EventID {
	case "NF_LOAD":
		analyticsData = fetchNFLoad(ctx, subReq.TargetNF)
	case "SLICE_LOAD":
		analyticsData = fetchSliceLoad(ctx, subReq.SNSSAI)
	case "UE_MOBILITY":
		// No immediate data for UE mobility subscriptions
	}

	resp := AnalyticsSubscribeResponse{
		SubscriptionID: subID,
		EventID:        subReq.EventID,
		AnalyticsData:  analyticsData,
	}

	body, _ := json.Marshal(resp)
	return handler.Response{
		StatusCode: http.StatusCreated,
		Body:       body,
		Header:     jsonHeader(),
	}, nil
}

func fetchNFLoad(ctx context.Context, targetNF string) *models.NFLoadInfo {
	if targetNF == "" {
		return &models.NFLoadInfo{NfLoad: 0}
	}
	var info models.NFLoadInfo
	key := fmt.Sprintf("nf-metrics/%s", targetNF)
	if err := Store.Get(ctx, key, &info); err != nil {
		return &models.NFLoadInfo{NFInstanceID: targetNF, NfLoad: 0}
	}
	return &info
}

func fetchSliceLoad(ctx context.Context, snssai *models.SNSSAI) *models.SliceLoadInfo {
	if snssai == nil {
		return &models.SliceLoadInfo{}
	}
	sliceKey := sliceKeyFromSNSSAI(*snssai)
	var info models.SliceLoadInfo
	key := fmt.Sprintf("slice-metrics/%s", sliceKey)
	if err := Store.Get(ctx, key, &info); err != nil {
		return &models.SliceLoadInfo{SST: snssai.SST, SD: snssai.SD}
	}
	return &info
}

func sliceKeyFromSNSSAI(s models.SNSSAI) string {
	if s.SD == "" {
		return fmt.Sprintf("%d", s.SST)
	}
	return fmt.Sprintf("%d-%s", s.SST, s.SD)
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
