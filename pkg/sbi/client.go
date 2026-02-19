// Package sbi provides the inter-NF communication client for the serverless 5GC.
// In the Function-per-Procedure architecture, each function calls other functions
// through the OpenFaaS gateway using HTTP POST with JSON payloads. This mirrors
// the 3GPP Service-Based Interface (SBI) pattern where NFs communicate via
// RESTful HTTP/2 APIs (TS 29.500), adapted for the OpenFaaS function routing model.
//
// The gateway URL defaults to http://gateway.openfaas:8080/function (the standard
// OpenFaaS in-cluster endpoint). Go's default HTTP transport maintains a persistent
// keep-alive connection pool, so each function call reuses an existing TCP connection
// rather than performing a fresh handshake.
package sbi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// Client calls other NF functions via the OpenFaaS gateway.
type Client struct {
	gateway    string
	httpClient *http.Client
}

// NewClient creates an SBI client using the OPENFAAS_GATEWAY env var.
func NewClient() *Client {
	gw := os.Getenv("OPENFAAS_GATEWAY")
	if gw == "" {
		gw = "http://gateway.openfaas:8080/function"
	}
	return &Client{gateway: gw, httpClient: &http.Client{}}
}

// NewClientWithGateway creates an SBI client with an explicit gateway URL.
func NewClientWithGateway(gateway string) *Client {
	return &Client{gateway: gateway, httpClient: &http.Client{}}
}

// CallFunction invokes another OpenFaaS function by name.
func (c *Client) CallFunction(funcName string, payload interface{}, result interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/%s", c.gateway, funcName)
	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("call %s: %w", funcName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s returned %d: %s", funcName, resp.StatusCode, errBody)
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}
