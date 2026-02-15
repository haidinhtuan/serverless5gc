package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/sbi"
	"github.com/tdinh/serverless5gc/pkg/state"

	// NRF functions (etcd-backed)
	nrfRegister     "github.com/tdinh/serverless5gc/functions/nrf/register"
	nrfDiscover     "github.com/tdinh/serverless5gc/functions/nrf/discover"
	nrfStatusNotify "github.com/tdinh/serverless5gc/functions/nrf/status-notify"

	// AMF functions (redis-backed)
	amfRegistration   "github.com/tdinh/serverless5gc/functions/amf/registration"
	amfDeregistration "github.com/tdinh/serverless5gc/functions/amf/deregistration"
	amfServiceRequest "github.com/tdinh/serverless5gc/functions/amf/service-request"
	amfPduSessionRelay "github.com/tdinh/serverless5gc/functions/amf/pdu-session-relay"
	amfAuthInitiate   "github.com/tdinh/serverless5gc/functions/amf/auth-initiate"

	// SMF functions (redis-backed)
	smfPduSessionCreate  "github.com/tdinh/serverless5gc/functions/smf/pdu-session-create"
	smfPduSessionUpdate  "github.com/tdinh/serverless5gc/functions/smf/pdu-session-update"
	smfPduSessionRelease "github.com/tdinh/serverless5gc/functions/smf/pdu-session-release"
	smfN4SessionSetup    "github.com/tdinh/serverless5gc/functions/smf/n4-session-setup"

	// UDM functions (redis-backed)
	udmGenerateAuthData  "github.com/tdinh/serverless5gc/functions/udm/generate-auth-data"
	udmGetSubscriberData "github.com/tdinh/serverless5gc/functions/udm/get-subscriber-data"

	// UDR functions (redis-backed)
	udrDataRead  "github.com/tdinh/serverless5gc/functions/udr/data-read"
	udrDataWrite "github.com/tdinh/serverless5gc/functions/udr/data-write"

	// AUSF functions (redis-backed)
	ausfAuthenticate "github.com/tdinh/serverless5gc/functions/ausf/authenticate"

	// PCF functions (redis-backed)
	pcfPolicyCreate "github.com/tdinh/serverless5gc/functions/pcf/policy-create"
	pcfPolicyGet    "github.com/tdinh/serverless5gc/functions/pcf/policy-get"

	// NSSF functions (redis-backed)
	nssfSliceSelect "github.com/tdinh/serverless5gc/functions/nssf/slice-select"
)

// wrapHandler converts an OpenFaaS function handler into a standard http.HandlerFunc.
// It reads the incoming HTTP request, constructs a handler.Request, calls the
// function handler, and writes the handler.Response back to the HTTP response.
func wrapHandler(fn func(handler.Request) (handler.Response, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"read body: %s"}`, err), http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()

		req := handler.Request{
			Body:        body,
			Header:      r.Header,
			QueryString: r.URL.RawQuery,
			Method:      r.Method,
			Host:        r.Host,
		}
		req.WithContext(r.Context())

		resp, err := fn(req)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusInternalServerError)
			return
		}

		for k, vals := range resp.Header {
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}

		if resp.StatusCode == 0 {
			resp.StatusCode = http.StatusOK
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(resp.Body)
	}
}

func main() {
	// Redis store for most functions
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "redis:6379"
	}
	redisStore := state.NewRedisStore(redisAddr)

	// Etcd store for NRF functions
	etcdEndpoint := os.Getenv("ETCD_ENDPOINT")
	if etcdEndpoint == "" {
		etcdEndpoint = "etcd:2379"
	}
	etcdStore, err := state.NewEtcdStore([]string{etcdEndpoint})
	if err != nil {
		log.Fatalf("Failed to connect to etcd at %s: %v", etcdEndpoint, err)
	}

	// SBI client pointing to this gateway for inter-function calls
	sbiClient := sbi.NewClientWithGateway("http://localhost:8080/function")

	// --- Configure NRF functions (etcd-backed) ---
	nrfRegister.SetStore(etcdStore)
	nrfDiscover.SetStore(etcdStore)
	nrfStatusNotify.SetStore(etcdStore)

	// --- Configure AMF functions (redis-backed) ---
	amfRegistration.SetStore(redisStore)
	amfRegistration.SetSBI(sbiClient)

	amfDeregistration.SetStore(redisStore)

	amfServiceRequest.SetStore(redisStore)

	amfPduSessionRelay.SetStore(redisStore)
	amfPduSessionRelay.SetSBI(sbiClient)

	amfAuthInitiate.SetStore(redisStore)
	amfAuthInitiate.SetSBI(sbiClient)

	// --- Configure SMF functions (redis-backed) ---
	// PFCP is left nil; handlers skip PFCP operations when nil.
	smfPduSessionCreate.SetStore(redisStore)
	smfPduSessionCreate.SetSBI(sbiClient)

	smfPduSessionUpdate.SetStore(redisStore)

	smfPduSessionRelease.SetStore(redisStore)

	smfN4SessionSetup.SetStore(redisStore)

	// --- Configure UDM functions (redis-backed) ---
	udmGenerateAuthData.SetStore(redisStore)
	udmGetSubscriberData.SetStore(redisStore)

	// --- Configure UDR functions (redis-backed) ---
	udrDataRead.SetStore(redisStore)
	udrDataWrite.SetStore(redisStore)

	// --- Configure AUSF function (redis-backed) ---
	ausfAuthenticate.SetStore(redisStore)

	// --- Configure PCF functions (redis-backed) ---
	pcfPolicyCreate.SetStore(redisStore)
	pcfPolicyGet.SetStore(redisStore)

	// --- Configure NSSF function (redis-backed) ---
	nssfSliceSelect.SetStore(redisStore)

	// --- Register HTTP routes matching OpenFaaS function names from stack.yml ---
	mux := http.NewServeMux()

	// Health check for integration test readiness probe
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// NRF
	mux.HandleFunc("/function/nrf-register", wrapHandler(nrfRegister.Handle))
	mux.HandleFunc("/function/nrf-discover", wrapHandler(nrfDiscover.Handle))
	mux.HandleFunc("/function/nrf-status-notify", wrapHandler(nrfStatusNotify.Handle))

	// AMF
	mux.HandleFunc("/function/amf-initial-registration", wrapHandler(amfRegistration.Handle))
	mux.HandleFunc("/function/amf-deregistration", wrapHandler(amfDeregistration.Handle))
	mux.HandleFunc("/function/amf-service-request", wrapHandler(amfServiceRequest.Handle))
	mux.HandleFunc("/function/amf-pdu-session-relay", wrapHandler(amfPduSessionRelay.Handle))
	mux.HandleFunc("/function/amf-auth-initiate", wrapHandler(amfAuthInitiate.Handle))

	// SMF
	mux.HandleFunc("/function/smf-pdu-session-create", wrapHandler(smfPduSessionCreate.Handle))
	mux.HandleFunc("/function/smf-pdu-session-update", wrapHandler(smfPduSessionUpdate.Handle))
	mux.HandleFunc("/function/smf-pdu-session-release", wrapHandler(smfPduSessionRelease.Handle))
	mux.HandleFunc("/function/smf-n4-session-setup", wrapHandler(smfN4SessionSetup.Handle))

	// UDM
	mux.HandleFunc("/function/udm-generate-auth-data", wrapHandler(udmGenerateAuthData.Handle))
	mux.HandleFunc("/function/udm-get-subscriber-data", wrapHandler(udmGetSubscriberData.Handle))

	// UDR
	mux.HandleFunc("/function/udr-data-read", wrapHandler(udrDataRead.Handle))
	mux.HandleFunc("/function/udr-data-write", wrapHandler(udrDataWrite.Handle))

	// AUSF
	mux.HandleFunc("/function/ausf-authenticate", wrapHandler(ausfAuthenticate.Handle))

	// PCF
	mux.HandleFunc("/function/pcf-policy-create", wrapHandler(pcfPolicyCreate.Handle))
	mux.HandleFunc("/function/pcf-policy-get", wrapHandler(pcfPolicyGet.Handle))

	// NSSF
	mux.HandleFunc("/function/nssf-slice-select", wrapHandler(nssfSliceSelect.Handle))

	addr := ":8080"
	log.Printf("Test gateway listening on %s", addr)
	log.Printf("Redis: %s | etcd: %s", redisAddr, etcdEndpoint)
	log.Printf("Registered 20 function handlers")
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
