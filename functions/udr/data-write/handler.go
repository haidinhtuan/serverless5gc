package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	handler "github.com/openfaas/templates-sdk/go-http"
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

// Handle writes subscriber data to the store.
// Expects a SubscriberData JSON body with a non-empty SUPI.
func Handle(req handler.Request) (handler.Response, error) {
	var sub models.SubscriberData
	if err := json.Unmarshal(req.Body, &sub); err != nil {
		return handler.Response{
			StatusCode: http.StatusBadRequest,
			Body:       []byte(fmt.Sprintf(`{"error":"invalid JSON: %s"}`, err)),
			Header:     jsonHeader(),
		}, nil
	}

	if sub.SUPI == "" {
		return handler.Response{
			StatusCode: http.StatusBadRequest,
			Body:       []byte(`{"error":"supi is required"}`),
			Header:     jsonHeader(),
		}, nil
	}

	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	key := "subscribers/" + sub.SUPI
	if err := Store.Put(ctx, key, sub); err != nil {
		return handler.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       []byte(fmt.Sprintf(`{"error":"%s"}`, err)),
			Header:     jsonHeader(),
		}, nil
	}

	body, _ := json.Marshal(sub)
	return handler.Response{
		StatusCode: http.StatusCreated,
		Body:       body,
		Header:     jsonHeader(),
	}, nil
}

func jsonHeader() http.Header {
	return http.Header{"Content-Type": []string{"application/json"}}
}
