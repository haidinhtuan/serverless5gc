package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/haidinhtuan/serverless5gc/pkg/models"
	"github.com/haidinhtuan/serverless5gc/pkg/state"
)

// NRFStore extends KVStore with prefix queries needed for NF discovery.
type NRFStore interface {
	state.KVStore
	GetByPrefix(ctx context.Context, prefix string) ([][]byte, error)
}

// store is the backing NRFStore. Set via init() in production; overridden in tests.
var store NRFStore

// SetStore allows tests to inject a mock store.
func SetStore(s NRFStore) {
	store = s
}

// DiscoverResult is the response payload for NF discovery.
type DiscoverResult struct {
	NFInstances []models.NFProfile `json:"nfInstances"`
}

// Handle discovers NF instances by type.
// Query parameter: target-nf-type (required).
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	params, _ := url.ParseQuery(req.QueryString)
	nfType := params.Get("target-nf-type")
	if nfType == "" {
		return handler.Response{
			StatusCode: http.StatusBadRequest,
			Body:       []byte(`{"error":"target-nf-type query parameter is required"}`),
		}, nil
	}

	results, err := store.GetByPrefix(ctx, "/nrf/nf-instances/")
	if err != nil {
		return handler.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       []byte(fmt.Sprintf(`{"error":"store query: %s"}`, err)),
		}, nil
	}

	var matched []models.NFProfile
	for _, data := range results {
		var profile models.NFProfile
		if err := json.Unmarshal(data, &profile); err != nil {
			continue
		}
		if profile.NFType == nfType {
			matched = append(matched, profile)
		}
	}

	if matched == nil {
		matched = []models.NFProfile{}
	}

	body, _ := json.Marshal(DiscoverResult{NFInstances: matched})
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}
