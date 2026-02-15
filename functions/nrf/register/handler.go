package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/models"
	"github.com/tdinh/serverless5gc/pkg/state"
)

// store is the backing KVStore. Set via init() in production; overridden in tests.
var store state.KVStore

// SetStore allows tests to inject a mock store.
func SetStore(s state.KVStore) {
	store = s
}

// Handle registers an NF instance in the NRF.
// It expects a JSON-encoded NFProfile in the request body.
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var profile models.NFProfile
	if err := json.Unmarshal(req.Body, &profile); err != nil {
		return handler.Response{
			StatusCode: http.StatusBadRequest,
			Body:       []byte(fmt.Sprintf(`{"error":"invalid body: %s"}`, err)),
		}, nil
	}

	if profile.NFInstanceID == "" {
		return handler.Response{
			StatusCode: http.StatusBadRequest,
			Body:       []byte(`{"error":"nfInstanceId is required"}`),
		}, nil
	}

	key := fmt.Sprintf("/nrf/nf-instances/%s", profile.NFInstanceID)
	profile.NFStatus = "REGISTERED"

	if err := store.Put(ctx, key, profile); err != nil {
		return handler.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       []byte(fmt.Sprintf(`{"error":"store put: %s"}`, err)),
		}, nil
	}

	body, _ := json.Marshal(profile)
	return handler.Response{
		StatusCode: http.StatusCreated,
		Body:       body,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}
