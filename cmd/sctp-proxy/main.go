// The sctp-proxy binary bridges the N2 interface (SCTP/NGAP, TS 38.412) between
// UERANSIM (or any 3GPP-compliant gNB) and the serverless 5GC backend running on
// OpenFaaS. It terminates the SCTP association from the gNB, decodes NGAP messages,
// extracts NAS PDUs, and forwards procedure calls as HTTP/JSON to the appropriate
// OpenFaaS functions via the gateway.
//
// The proxy maintains a per-UE state machine (keyed by RAN-UE-NGAP-ID) to track
// the signaling flow: NG Setup → Registration → Authentication → Security Mode →
// PDU Session Establishment. NAS security (integrity protection via 128-EIA2) is
// applied to all downlink messages after Security Mode Command.
//
// Configuration via environment variables:
//
//	SCTP_LISTEN_ADDR  - SCTP listen address (default: 0.0.0.0:38412)
//	OPENFAAS_GATEWAY  - OpenFaaS gateway URL (default: http://gateway.openfaas:8080/function/)
//	REDIS_ADDR        - Redis address for reading auth vectors (default: localhost:6379)
//	PLMN_MCC          - PLMN Mobile Country Code (default: 001)
//	PLMN_MNC          - PLMN Mobile Network Code (default: 01)
//	SNSSAI_SD         - S-NSSAI Slice Differentiator hex (default: 010203)
package main

import (
	"encoding/hex"
	"log"
	"os"

	ngapCodec "github.com/haidinhtuan/serverless5gc/pkg/ngap"
	"github.com/haidinhtuan/serverless5gc/pkg/state"
)

func main() {
	listenAddr := os.Getenv("SCTP_LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = "0.0.0.0:38412"
	}
	openfaasGW := os.Getenv("OPENFAAS_GATEWAY")
	if openfaasGW == "" {
		openfaasGW = "http://gateway.openfaas:8080/function/"
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	// Eval config: PLMN 001/01, S-NSSAI SST=1, SD=010203
	mcc := os.Getenv("PLMN_MCC")
	if mcc == "" {
		mcc = "001"
	}
	mnc := os.Getenv("PLMN_MNC")
	if mnc == "" {
		mnc = "01"
	}
	sdHex := os.Getenv("SNSSAI_SD")
	if sdHex == "" {
		sdHex = "010203"
	}

	plmn := ngapCodec.PLMNBytes(mcc, mnc)
	sd, _ := hex.DecodeString(sdHex)
	backend := NewHTTPBackend(openfaasGW)
	store := state.NewRedisStore(redisAddr)

	proxy := NewSCTPProxy(listenAddr, backend, store, plmn, 0x01, sd)
	log.Fatal(proxy.Start())
}
