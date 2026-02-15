package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/ishidawataru/sctp"
	ngapRouter "github.com/tdinh/serverless5gc/pkg/ngap"
)

// SCTPProxy bridges SCTP connections from gNBs to OpenFaaS HTTP functions.
type SCTPProxy struct {
	listenAddr string
	openfaasGW string
	httpClient *http.Client
}

// NewSCTPProxy creates a new proxy instance.
func NewSCTPProxy(listenAddr, openfaasGW string) *SCTPProxy {
	return &SCTPProxy{
		listenAddr: listenAddr,
		openfaasGW: openfaasGW,
		httpClient: &http.Client{},
	}
}

// Start begins listening for SCTP connections.
func (p *SCTPProxy) Start() error {
	addr, err := sctp.ResolveSCTPAddr("sctp", p.listenAddr)
	if err != nil {
		return fmt.Errorf("resolve sctp addr: %w", err)
	}

	ln, err := sctp.ListenSCTP("sctp", addr)
	if err != nil {
		return fmt.Errorf("sctp listen %s: %w", p.listenAddr, err)
	}
	defer ln.Close()

	log.Printf("SCTP proxy listening on %s", p.listenAddr)

	for {
		conn, err := ln.AcceptSCTP()
		if err != nil {
			log.Printf("sctp accept: %v", err)
			continue
		}
		go p.handleConnection(conn)
	}
}

func (p *SCTPProxy) handleConnection(conn *sctp.SCTPConn) {
	defer conn.Close()
	buf := make([]byte, 65535)

	for {
		n, _, err := conn.SCTPRead(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("sctp read: %v", err)
			}
			return
		}

		route, routeErr := ngapRouter.RouteNGAP(buf[:n])
		if routeErr != nil {
			log.Printf("ngap route: %v", routeErr)
			continue
		}

		url := fmt.Sprintf("%s%s", p.openfaasGW, route.FunctionName)
		resp, err := p.httpClient.Post(url, "application/octet-stream",
			bytes.NewReader(buf[:n]))
		if err != nil {
			log.Printf("forward to %s: %v", route.FunctionName, err)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if len(respBody) > 0 {
			if _, writeErr := conn.SCTPWrite(respBody, nil); writeErr != nil {
				log.Printf("sctp write: %v", writeErr)
				return
			}
		}
	}
}
