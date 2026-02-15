package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// AWS Lambda pricing (x86).
const (
	lambdaPricePerGBSecond = 0.0000166667
	lambdaPricePerRequest  = 0.0000002 // $0.20 / 1M
)

// AWS Fargate pricing for baseline comparison.
const (
	fargatePricePerVCPUHour = 0.04048
	fargatePricePerGBHour   = 0.004445
)

// Baseline VM specs (matching IONOS provisioned VMs).
const (
	baselineVCPUs   = 8
	baselineMemryGB = 16
)

var (
	functionCost = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "serverless5gc_function_cost_usd",
			Help: "Projected cost in USD based on AWS Lambda pricing per function",
		},
		[]string{"function_name", "pricing_model"},
	)

	totalCostServerless = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "serverless5gc_total_cost_serverless_usd",
		Help: "Total projected hourly cost for serverless deployment (Lambda model)",
	})

	totalCostTraditional = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "serverless5gc_total_cost_traditional_usd",
			Help: "Total projected hourly cost for traditional deployment (Fargate model)",
		},
		[]string{"baseline"},
	)

	functionInvocations = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "serverless5gc_function_invocations_total",
			Help: "Total invocations per function (mirrored from OpenFaaS)",
		},
		[]string{"function_name"},
	)
)

func init() {
	prometheus.MustRegister(functionCost, totalCostServerless, totalCostTraditional, functionInvocations)
}

// prometheusQueryResult represents the JSON structure from Prometheus instant query.
type prometheusQueryResult struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  [2]interface{}    `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func queryPrometheus(promURL, query string) (*prometheusQueryResult, error) {
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/query?query=%s", promURL, query))
	if err != nil {
		return nil, fmt.Errorf("query prometheus: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result prometheusQueryResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &result, nil
}

func parseFloat(v interface{}) float64 {
	switch val := v.(type) {
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	case float64:
		return val
	default:
		return 0
	}
}

func collectCosts(promURL string) {
	// Query total invocations per function over the last hour.
	invocResult, err := queryPrometheus(promURL,
		"sum(increase(gateway_function_invocation_total[1h])) by (function_name)")
	if err != nil {
		log.Printf("Error querying invocations: %v", err)
		return
	}

	// Query average duration per function over the last hour.
	durationResult, err := queryPrometheus(promURL,
		"rate(gateway_functions_seconds_sum[1h]) / rate(gateway_functions_seconds_count[1h])")
	if err != nil {
		log.Printf("Error querying durations: %v", err)
		return
	}

	// Build duration map: function_name -> avg duration in seconds.
	durations := make(map[string]float64)
	if durationResult.Status == "success" {
		for _, r := range durationResult.Data.Result {
			fname := r.Metric["function_name"]
			durations[fname] = parseFloat(r.Value[1])
		}
	}

	var totalServerlessCost float64
	defaultMemoryMB := 128.0 // default OpenFaaS function memory

	if invocResult.Status == "success" {
		for _, r := range invocResult.Data.Result {
			fname := r.Metric["function_name"]
			invocations := parseFloat(r.Value[1])
			avgDuration := durations[fname]
			if avgDuration == 0 {
				avgDuration = 0.1 // default 100ms if no data
			}

			functionInvocations.WithLabelValues(fname).Set(invocations)

			// Lambda cost model: compute + request charges.
			gbSeconds := (defaultMemoryMB / 1024.0) * avgDuration * invocations
			computeCost := gbSeconds * lambdaPricePerGBSecond
			requestCost := invocations * lambdaPricePerRequest
			lambdaCost := computeCost + requestCost

			functionCost.WithLabelValues(fname, "lambda").Set(lambdaCost)
			totalServerlessCost += lambdaCost
		}
	}

	totalCostServerless.Set(totalServerlessCost)

	// Fargate cost model for traditional deployments: fixed VM cost per hour.
	// Both Open5GS and free5GC run on 8 vCPU / 16 GB VMs.
	fargateCostPerHour := float64(baselineVCPUs)*fargatePricePerVCPUHour +
		float64(baselineMemryGB)*fargatePricePerGBHour
	totalCostTraditional.WithLabelValues("open5gs").Set(fargateCostPerHour)
	totalCostTraditional.WithLabelValues("free5gc").Set(fargateCostPerHour)
}

func main() {
	promURL := os.Getenv("PROMETHEUS_URL")
	if promURL == "" {
		promURL = "http://prometheus:9090"
	}

	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":9200"
	}

	// Collect costs periodically.
	go func() {
		for {
			collectCosts(promURL)
			time.Sleep(15 * time.Second)
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	log.Printf("Cost exporter listening on %s (prometheus: %s)", listenAddr, promURL)
	log.Fatal(http.ListenAndServe(listenAddr, nil))
}
