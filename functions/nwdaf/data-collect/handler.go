// Package function implements Nnwdaf_DataManagement per 3GPP TS 29.520
// to collect NF load and slice load metrics for analytics processing.
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
	"github.com/tdinh/serverless5gc/pkg/state"
)

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

type DataCollectRequest struct {
	CollectType  string         `json:"collect_type"`
	NFInstanceID string         `json:"nf_instance_id,omitempty"`
	NFType       string         `json:"nf_type,omitempty"`
	CPUUsage     float64        `json:"cpu_usage,omitempty"`
	MemUsage     float64        `json:"mem_usage,omitempty"`
	NfLoad       int            `json:"nf_load,omitempty"`
	SNSSAI       *models.SNSSAI `json:"snssai,omitempty"`
}

func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var collectReq DataCollectRequest
	if err := json.Unmarshal(req.Body, &collectReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}

	if collectReq.CollectType != "NF_LOAD" && collectReq.CollectType != "SLICE_LOAD" {
		return errorResp(http.StatusBadRequest, "collect_type must be NF_LOAD or SLICE_LOAD"), nil
	}

	switch collectReq.CollectType {
	case "NF_LOAD":
		return handleNFLoad(ctx, collectReq)
	case "SLICE_LOAD":
		return handleSliceLoad(ctx, collectReq)
	}

	return errorResp(http.StatusBadRequest, "unsupported collect_type"), nil
}

func handleNFLoad(ctx context.Context, collectReq DataCollectRequest) (handler.Response, error) {
	if collectReq.NFInstanceID == "" {
		return errorResp(http.StatusBadRequest, "nf_instance_id is required for NF_LOAD"), nil
	}

	nfType := collectReq.NFType
	if nfType == "" {
		nfType = "NF"
	}

	info := models.NFLoadInfo{
		NFInstanceID: collectReq.NFInstanceID,
		NFType:       nfType,
		CPUUsage:     collectReq.CPUUsage,
		MemUsage:     collectReq.MemUsage,
		NfLoad:       collectReq.NfLoad,
		Timestamp:    time.Now(),
	}

	key := fmt.Sprintf("nf-metrics/%s", collectReq.NFInstanceID)
	if err := Store.Put(ctx, key, info); err != nil {
		return errorResp(http.StatusInternalServerError, "store put: %s", err), nil
	}

	body, _ := json.Marshal(info)
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     jsonHeader(),
	}, nil
}

func handleSliceLoad(ctx context.Context, collectReq DataCollectRequest) (handler.Response, error) {
	if collectReq.SNSSAI == nil || collectReq.SNSSAI.SST <= 0 {
		return errorResp(http.StatusBadRequest, "snssai with sst > 0 is required for SLICE_LOAD"), nil
	}

	sliceKey := sliceKeyFromSNSSAI(*collectReq.SNSSAI)

	var currentUEs, currentSessions int64
	var counters models.SliceCounters
	countersKey := fmt.Sprintf("nsacf-counters/%s", sliceKey)
	if err := Store.Get(ctx, countersKey, &counters); err == nil {
		currentUEs = counters.CurrentUEs
		currentSessions = counters.CurrentSessions
	}

	info := models.SliceLoadInfo{
		SST:             collectReq.SNSSAI.SST,
		SD:              collectReq.SNSSAI.SD,
		CurrentUEs:      currentUEs,
		CurrentSessions: currentSessions,
		MeanNFLoad:      float64(collectReq.NfLoad),
		Timestamp:       time.Now(),
	}

	key := fmt.Sprintf("slice-metrics/%s", sliceKey)
	if err := Store.Put(ctx, key, info); err != nil {
		return errorResp(http.StatusInternalServerError, "store put: %s", err), nil
	}

	body, _ := json.Marshal(info)
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     jsonHeader(),
	}, nil
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
