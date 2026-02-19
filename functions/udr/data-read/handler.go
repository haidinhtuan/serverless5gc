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

// Handle reads subscriber data from the store by SUPI.
// SUPI is taken from the query string parameter "supi"
// or from a JSON body {"supi":"imsi-..."}.
func Handle(req handler.Request) (handler.Response, error) {
	supi, err := extractSUPI(req)
	if err != nil || supi == "" {
		return handler.Response{
			StatusCode: http.StatusBadRequest,
			Body:       []byte(`{"error":"supi parameter required"}`),
			Header:     jsonHeader(),
		}, nil
	}

	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var sub models.SubscriberData
	key := "subscribers/" + supi
	if err := Store.Get(ctx, key, &sub); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return handler.Response{
				StatusCode: http.StatusNotFound,
				Body:       []byte(fmt.Sprintf(`{"error":"subscriber %s not found"}`, supi)),
				Header:     jsonHeader(),
			}, nil
		}
		return handler.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       []byte(fmt.Sprintf(`{"error":"%s"}`, err)),
			Header:     jsonHeader(),
		}, nil
	}

	body, _ := json.Marshal(sub)
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     jsonHeader(),
	}, nil
}

func extractSUPI(req handler.Request) (string, error) {
	// Try query string first
	if req.QueryString != "" {
		params, err := url.ParseQuery(req.QueryString)
		if err == nil {
			if supi := params.Get("supi"); supi != "" {
				return supi, nil
			}
		}
	}
	// Fall back to JSON body
	if len(req.Body) > 0 {
		var body struct {
			SUPI string `json:"supi"`
		}
		if err := json.Unmarshal(req.Body, &body); err == nil && body.SUPI != "" {
			return body.SUPI, nil
		}
	}
	return "", fmt.Errorf("supi not provided")
}

func jsonHeader() http.Header {
	return http.Header{"Content-Type": []string{"application/json"}}
}
