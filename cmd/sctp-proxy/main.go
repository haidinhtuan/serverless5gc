package main

import (
	"log"
	"os"
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

	proxy := NewSCTPProxy(listenAddr, openfaasGW)
	log.Fatal(proxy.Start())
}
