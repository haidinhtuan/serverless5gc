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
