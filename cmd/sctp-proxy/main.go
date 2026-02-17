package main

import (
	"encoding/hex"
	"log"
	"os"

	ngapCodec "github.com/tdinh/serverless5gc/pkg/ngap"
	"github.com/tdinh/serverless5gc/pkg/state"
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
