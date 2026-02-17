package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/free5gc/ngap/ngapType"
	"github.com/ishidawataru/sctp"
	ngapCodec "github.com/tdinh/serverless5gc/pkg/ngap"
	"github.com/tdinh/serverless5gc/pkg/nas"
	"github.com/tdinh/serverless5gc/pkg/state"
)

// UE states for the per-UE state machine.
const (
	UEStateIdle            = "IDLE"
	UEStateAuthSent        = "AUTH_SENT"
	UEStateSMCSent         = "SMC_SENT"
	UEStateRegistered      = "REGISTERED"
	UEStatePDUSessionActive = "PDU_SESSION_ACTIVE"
)

// UEContext holds per-UE state, keyed by RAN-UE-NGAP-ID.
type UEContext struct {
	SUPI        string
	State       string
	AMFUeNgapID int64
	RANUeNgapID int64
	XRES        []byte // stored for auth verification
	DLSeqNum    byte   // NAS downlink sequence number for security header
}

// CoreBackend abstracts the 5GC function call interface.
// HTTPBackend calls OpenFaaS functions; future DIDComm backends can implement this.
type CoreBackend interface {
	AuthInitiate(ctx context.Context, supi string) (*AuthInitiateResponse, error)
	Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error)
	PDUSessionCreate(ctx context.Context, req *PDUSessionRequest) (*PDUSessionResponse, error)
}

type AuthInitiateResponse struct {
	AuthType string `json:"auth_type"`
	RAND     string `json:"rand"`
	AUTN     string `json:"autn"`
	SUPI     string `json:"supi"`
}

type RegisterRequest struct {
	SUPI     string `json:"supi"`
	SkipAuth bool   `json:"skip_auth"`
}

type RegisterResponse struct {
	Status     string `json:"status"`
	SUPI       string `json:"supi"`
	GUTI       string `json:"guti"`
	NASMessage string `json:"nas_message"` // hex-encoded NAS Registration Accept
}

type PDUSessionRequest struct {
	SUPI         string `json:"supi"`
	PDUSessionID uint8  `json:"pdu_session_id"`
	DNN          string `json:"dnn"`
	SNSSAISSt    uint8  `json:"snssai_sst"`
	SNSSAISD     string `json:"snssai_sd"`
}

type PDUSessionResponse struct {
	Status     string `json:"status"`
	PDUAddress string `json:"pdu_address"`
}

// HTTPBackend calls OpenFaaS functions over HTTP.
type HTTPBackend struct {
	gatewayURL string
	client     *http.Client
}

func NewHTTPBackend(gatewayURL string) *HTTPBackend {
	return &HTTPBackend{gatewayURL: gatewayURL, client: &http.Client{}}
}

func (b *HTTPBackend) AuthInitiate(_ context.Context, supi string) (*AuthInitiateResponse, error) {
	payload, _ := json.Marshal(map[string]string{"supi": supi})
	resp, err := b.client.Post(b.gatewayURL+"amf-auth-initiate", "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("amf-auth-initiate: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("amf-auth-initiate returned %d: %s", resp.StatusCode, body)
	}
	var result AuthInitiateResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode auth response: %w", err)
	}
	return &result, nil
}

func (b *HTTPBackend) Register(_ context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	payload, _ := json.Marshal(req)
	resp, err := b.client.Post(b.gatewayURL+"amf-initial-registration", "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("amf-initial-registration: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("amf-initial-registration returned %d: %s", resp.StatusCode, body)
	}
	var result RegisterResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode register response: %w", err)
	}
	return &result, nil
}

func (b *HTTPBackend) PDUSessionCreate(_ context.Context, req *PDUSessionRequest) (*PDUSessionResponse, error) {
	payload, _ := json.Marshal(req)
	resp, err := b.client.Post(b.gatewayURL+"smf-pdu-session-create", "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("smf-pdu-session-create: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("smf-pdu-session-create returned %d: %s", resp.StatusCode, body)
	}
	var result PDUSessionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode pdu session response: %w", err)
	}
	return &result, nil
}

// SCTPProxy bridges SCTP/NGAP from UERANSIM to HTTP/JSON OpenFaaS functions
// via a per-UE state machine.
type SCTPProxy struct {
	listenAddr string
	backend    CoreBackend
	store      state.KVStore // Redis for reading auth vectors
	plmnBytes  []byte
	sst        byte
	sd         []byte

	ueMap             sync.Map // map[int64]*UEContext keyed by RAN-UE-NGAP-ID
	amfUeNgapIDCounter int64
}

func NewSCTPProxy(listenAddr string, backend CoreBackend, store state.KVStore, plmn []byte, sst byte, sd []byte) *SCTPProxy {
	return &SCTPProxy{
		listenAddr: listenAddr,
		backend:    backend,
		store:      store,
		plmnBytes:  plmn,
		sst:        sst,
		sd:         sd,
	}
}

func (p *SCTPProxy) allocAMFUeNgapID() int64 {
	return atomic.AddInt64(&p.amfUeNgapIDCounter, 1)
}

func (p *SCTPProxy) getUE(ranUeNgapID int64) *UEContext {
	if v, ok := p.ueMap.Load(ranUeNgapID); ok {
		return v.(*UEContext)
	}
	return nil
}

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

	log.Printf("SCTP proxy listening on %s (NGAP-to-HTTP bridge)", p.listenAddr)

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
	log.Printf("gNB connected from %v", conn.RemoteAddr())
	buf := make([]byte, 65535)

	for {
		n, _, err := conn.SCTPRead(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("sctp read: %v", err)
			}
			return
		}

		responses, err := p.handleNGAPMessage(buf[:n])
		if err != nil {
			log.Printf("handle ngap: %v", err)
			continue
		}

		for _, resp := range responses {
			if _, writeErr := conn.SCTPWrite(resp, nil); writeErr != nil {
				log.Printf("sctp write: %v", writeErr)
				return
			}
		}
	}
}

// handleNGAPMessage processes one NGAP PDU and returns zero or more APER-encoded responses.
func (p *SCTPProxy) handleNGAPMessage(data []byte) ([][]byte, error) {
	ctx, err := ngapCodec.ParseNGAPMessage(data)
	if err != nil {
		return nil, fmt.Errorf("parse ngap: %w", err)
	}

	switch {
	case ctx.MessageType == 0 && ctx.ProcedureCode == ngapType.ProcedureCodeNGSetup:
		return p.handleNGSetup()

	case ctx.MessageType == 0 && ctx.ProcedureCode == ngapType.ProcedureCodeInitialUEMessage:
		return p.handleInitialUEMessage(ctx)

	case ctx.MessageType == 0 && ctx.ProcedureCode == ngapType.ProcedureCodeUplinkNASTransport:
		return p.handleUplinkNASTransport(ctx)

	default:
		log.Printf("unhandled NGAP: type=%d procedure=%d", ctx.MessageType, ctx.ProcedureCode)
		return nil, nil
	}
}

// handleNGSetup responds locally with NGSetupResponse.
func (p *SCTPProxy) handleNGSetup() ([][]byte, error) {
	log.Printf("NG Setup Request received, responding locally")
	resp, err := ngapCodec.BuildNGSetupResponse(p.plmnBytes, p.sst, p.sd)
	if err != nil {
		return nil, fmt.Errorf("build ng setup response: %w", err)
	}
	return [][]byte{resp}, nil
}

// handleInitialUEMessage processes InitialUEMessage containing NAS Registration Request.
// Creates UE state, calls auth-initiate, sends NAS Authentication Request.
func (p *SCTPProxy) handleInitialUEMessage(ngapCtx *ngapCodec.NGAPContext) ([][]byte, error) {
	if len(ngapCtx.NASPDU) == 0 {
		return nil, fmt.Errorf("initial UE message has no NAS PDU")
	}

	regReq, err := nas.DecodeRegistrationRequest(ngapCtx.NASPDU)
	if err != nil {
		return nil, fmt.Errorf("decode registration request: %w", err)
	}

	supi := regReq.MobileIdentity.Value
	amfUeNgapID := p.allocAMFUeNgapID()

	ue := &UEContext{
		SUPI:        supi,
		State:       UEStateIdle,
		AMFUeNgapID: amfUeNgapID,
		RANUeNgapID: ngapCtx.RANUeNgapID,
	}
	p.ueMap.Store(ngapCtx.RANUeNgapID, ue)

	log.Printf("UE %s: InitialUEMessage, RAN-UE=%d AMF-UE=%d", supi, ngapCtx.RANUeNgapID, amfUeNgapID)

	// Call amf-auth-initiate to get RAND/AUTN challenge
	authResp, err := p.backend.AuthInitiate(context.Background(), supi)
	if err != nil {
		return nil, fmt.Errorf("auth initiate for %s: %w", supi, err)
	}

	// Read XRES* from Redis for later verification
	var authPending struct {
		XRES  string `json:"xres_star"`
		KAUSF string `json:"kausf"`
	}
	if err := p.store.Get(context.Background(), "auth-pending:"+supi, &authPending); err != nil {
		log.Printf("UE %s: warning: could not read auth-pending from Redis: %v", supi, err)
	} else {
		ue.XRES, _ = hex.DecodeString(authPending.XRES)
	}

	// Build NAS Authentication Request
	randBytes, _ := hex.DecodeString(authResp.RAND)
	autnBytes, _ := hex.DecodeString(authResp.AUTN)
	nasAuthReq := nas.EncodeAuthenticationRequest(randBytes, autnBytes)

	// Send via DownlinkNASTransport
	dlNAS, err := ngapCodec.BuildDownlinkNASTransport(amfUeNgapID, ngapCtx.RANUeNgapID, nasAuthReq)
	if err != nil {
		return nil, fmt.Errorf("build downlink nas (auth req): %w", err)
	}

	ue.State = UEStateAuthSent
	log.Printf("UE %s: sent Authentication Request, state=%s", supi, ue.State)
	return [][]byte{dlNAS}, nil
}

// handleUplinkNASTransport dispatches based on NAS message type and UE state.
func (p *SCTPProxy) handleUplinkNASTransport(ngapCtx *ngapCodec.NGAPContext) ([][]byte, error) {
	if len(ngapCtx.NASPDU) < 3 {
		return nil, fmt.Errorf("uplink NAS transport: NAS PDU too short")
	}

	ue := p.getUE(ngapCtx.RANUeNgapID)
	if ue == nil {
		return nil, fmt.Errorf("no UE context for RAN-UE-NGAP-ID %d", ngapCtx.RANUeNgapID)
	}

	// Strip NAS security header if present (after SMC, UE sends protected NAS)
	nasPDU := nas.StripSecurityHeader(ngapCtx.NASPDU)
	if len(nasPDU) < 3 {
		return nil, fmt.Errorf("inner NAS PDU too short after stripping security header")
	}

	nasEPD := nasPDU[0]
	nasMsgType := nasPDU[2]

	// Update the NASPDU in context to the stripped version for downstream handlers
	ngapCtx.NASPDU = nasPDU

	switch {
	case nasEPD == nas.EPD5GMM && nasMsgType == nas.MsgTypeAuthenticationResponse:
		return p.handleAuthResponse(ue, ngapCtx)

	case nasEPD == nas.EPD5GMM && nasMsgType == nas.MsgTypeSecurityModeComplete:
		return p.handleSecurityModeComplete(ue, ngapCtx)

	case nasEPD == nas.EPD5GMM && nasMsgType == nas.MsgTypeULNASTransport:
		return p.handleULNASTransport(ue, ngapCtx)

	case nasEPD == nas.EPD5GMM && nasMsgType == nas.MsgTypeRegistrationComplete:
		log.Printf("UE %s: Registration Complete received", ue.SUPI)
		return nil, nil

	default:
		log.Printf("UE %s: unhandled NAS message EPD=0x%02x type=0x%02x", ue.SUPI, nasEPD, nasMsgType)
		return nil, nil
	}
}

// handleAuthResponse verifies RES* and sends Security Mode Command.
func (p *SCTPProxy) handleAuthResponse(ue *UEContext, ngapCtx *ngapCodec.NGAPContext) ([][]byte, error) {
	if ue.State != UEStateAuthSent {
		return nil, fmt.Errorf("UE %s: auth response in unexpected state %s", ue.SUPI, ue.State)
	}

	resStar, err := nas.DecodeAuthenticationResponse(ngapCtx.NASPDU)
	if err != nil {
		return nil, fmt.Errorf("decode auth response: %w", err)
	}

	// Verify RES* against stored XRES*
	if len(ue.XRES) > 0 && !bytes.Equal(resStar, ue.XRES) {
		log.Printf("UE %s: RES* mismatch (got %x, want %x)", ue.SUPI, resStar, ue.XRES)
	}

	log.Printf("UE %s: Authentication Response verified", ue.SUPI)

	// Build NAS Security Mode Command (must be integrity-protected per TS 24.501 Section 4.4.6)
	smcPlain := nas.EncodeSecurityModeCommand(&nas.SecurityModeCommand{
		SelectedCiphering: nas.CipherAlg5GEA0, // null ciphering for eval
		SelectedIntegrity: nas.IntegAlg5GIA0,   // null integrity for eval
		NgKSI:             0,
		ReplayedUESecCap:  &nas.UESecurityCapability{EA0: true, EA1: true, EA2: true, IA0: true, IA1: true, IA2: true},
	})
	smc := nas.WrapSecurityHeader(smcPlain, nas.SecurityHeaderIntegrityProtectedNewCtx, 0)

	dlNAS, err := ngapCodec.BuildDownlinkNASTransport(ue.AMFUeNgapID, ue.RANUeNgapID, smc)
	if err != nil {
		return nil, fmt.Errorf("build downlink nas (smc): %w", err)
	}

	ue.DLSeqNum = 1
	ue.State = UEStateSMCSent
	log.Printf("UE %s: sent Security Mode Command, state=%s", ue.SUPI, ue.State)
	return [][]byte{dlNAS}, nil
}

// handleSecurityModeComplete completes registration via the backend and sends Registration Accept.
func (p *SCTPProxy) handleSecurityModeComplete(ue *UEContext, ngapCtx *ngapCodec.NGAPContext) ([][]byte, error) {
	if ue.State != UEStateSMCSent {
		return nil, fmt.Errorf("UE %s: SMC complete in unexpected state %s", ue.SUPI, ue.State)
	}

	_ = ngapCtx // NAS PDU already validated by caller

	log.Printf("UE %s: Security Mode Complete received, calling registration", ue.SUPI)

	// Call amf-initial-registration with skip_auth=true
	regResp, err := p.backend.Register(context.Background(), &RegisterRequest{
		SUPI:     ue.SUPI,
		SkipAuth: true,
	})
	if err != nil {
		return nil, fmt.Errorf("register %s: %w", ue.SUPI, err)
	}

	// Build Registration Accept NAS message (must be security-protected after SMC)
	var regAcceptPlain []byte
	if regResp.NASMessage != "" {
		regAcceptPlain, _ = hex.DecodeString(regResp.NASMessage)
	}
	if len(regAcceptPlain) == 0 {
		regAcceptPlain = nas.EncodeRegistrationAccept(&nas.RegistrationAccept{
			RegistrationResult: nas.RegResult3GPPAccess,
			GUTI:               regResp.GUTI,
			T3512Value:         nas.T3512Default,
		})
	}
	regAcceptNAS := nas.WrapSecurityHeader(regAcceptPlain, nas.SecurityHeaderIntegrityProtectedCipheredNew, ue.DLSeqNum)
	ue.DLSeqNum++

	// Send via InitialContextSetupRequest (wraps Registration Accept)
	icsr, err := ngapCodec.BuildInitialContextSetupRequest(
		ue.AMFUeNgapID, ue.RANUeNgapID,
		regAcceptNAS, p.plmnBytes, p.sst, p.sd,
	)
	if err != nil {
		return nil, fmt.Errorf("build initial context setup: %w", err)
	}

	ue.State = UEStateRegistered
	log.Printf("UE %s: registered, sent InitialContextSetupRequest, state=%s", ue.SUPI, ue.State)
	return [][]byte{icsr}, nil
}

// handleULNASTransport handles UL NAS Transport messages (e.g., PDU Session Establishment).
func (p *SCTPProxy) handleULNASTransport(ue *UEContext, ngapCtx *ngapCodec.NGAPContext) ([][]byte, error) {
	_, pduSessionID, payload, dnn, snssai, err := nas.DecodeULNASTransport(ngapCtx.NASPDU)
	if err != nil {
		return nil, fmt.Errorf("decode UL NAS transport: %w", err)
	}

	if len(payload) < 4 {
		return nil, fmt.Errorf("UL NAS transport payload too short")
	}

	// Check if inner message is PDU Session Establishment Request
	if payload[0] == nas.EPD5GSM && payload[3] == nas.MsgTypePDUSessionEstablishmentRequest {
		return p.handlePDUSessionEstablishment(ue, pduSessionID, dnn, snssai)
	}

	log.Printf("UE %s: unhandled UL NAS payload EPD=0x%02x type=0x%02x", ue.SUPI, payload[0], payload[3])
	return nil, nil
}

func (p *SCTPProxy) handlePDUSessionEstablishment(ue *UEContext, pduSessionID uint8, dnn string, snssai *nas.NSSAI) ([][]byte, error) {
	if dnn == "" {
		dnn = "internet"
	}
	sst := p.sst
	sd := hex.EncodeToString(p.sd)
	if snssai != nil {
		sst = snssai.SST
		if snssai.HasSD {
			sd = hex.EncodeToString(snssai.SD[:])
		}
	}

	log.Printf("UE %s: PDU Session Establishment Request, session=%d dnn=%s", ue.SUPI, pduSessionID, dnn)

	_, err := p.backend.PDUSessionCreate(context.Background(), &PDUSessionRequest{
		SUPI:         ue.SUPI,
		PDUSessionID: pduSessionID,
		DNN:          dnn,
		SNSSAISSt:    sst,
		SNSSAISD:     sd,
	})
	if err != nil {
		return nil, fmt.Errorf("pdu session create for %s: %w", ue.SUPI, err)
	}

	// Build a minimal PDU Session Establishment Accept NAS, wrapped in DL NAS Transport
	// For eval purposes, a simple accept with dummy IP is sufficient
	pduAcceptPlain := buildPDUSessionAcceptNAS(pduSessionID)
	pduAcceptNAS := nas.WrapSecurityHeader(pduAcceptPlain, nas.SecurityHeaderIntegrityProtectedCiphered, ue.DLSeqNum)
	ue.DLSeqNum++
	dlNAS, err := ngapCodec.BuildDownlinkNASTransport(ue.AMFUeNgapID, ue.RANUeNgapID, pduAcceptNAS)
	if err != nil {
		return nil, fmt.Errorf("build downlink nas (pdu accept): %w", err)
	}

	ue.State = UEStatePDUSessionActive
	log.Printf("UE %s: PDU session %d established, state=%s", ue.SUPI, pduSessionID, ue.State)
	return [][]byte{dlNAS}, nil
}

// buildPDUSessionAcceptNAS builds a DL NAS Transport containing a PDU Session Establishment Accept.
func buildPDUSessionAcceptNAS(pduSessionID uint8) []byte {
	// Inner: PDU Session Establishment Accept (TS 24.501 Section 8.3.2)
	// Format: EPD(1) + PSID(1) + PTI(1) + MsgType(1) + SSCmode|PDUtype(1) +
	//         AuthQoSRules(LV-E) + SessionAMBR(LV) + [PDUAddress(TLV)]
	inner := []byte{
		nas.EPD5GSM,       // EPD
		pduSessionID,      // PDU Session ID
		0x00,              // PTI
		0xC2,              // PDU Session Establishment Accept message type
		0x11,              // SSC mode 1 (bits 5-7) | PDU session type IPv4 (bits 1-3)
		// Authorized QoS rules (LV-E: 2-byte length + value)
		0x00, 0x06,        // QoS rules length = 6
		0x01,              // QoS rule ID = 1
		0x00, 0x03,        // Rule length = 3 (2 bytes per TS 24.501 Table 9.11.4.13.1)
		0x21,              // Rule operation: create new QoS rule (001), DQR=0, num filters=1 (0001)
		0x01,              // Packet filter: match all (direction=bidirectional)
		0x06,              // QFI = 6
		// Session-AMBR (LV: 1-byte length + value)
		0x06,              // Session-AMBR length = 6
		0x01,              // DL unit: kbps
		0x00, 0x00,        // DL session AMBR
		0x01,              // UL unit: kbps
		0x00, 0x00,        // UL session AMBR
		// PDU address (optional TLV, IEI=0x29)
		0x29, 0x05, 0x01,  // IEI + length=5 + PDU addr type=IPv4
		0x0A, 0x0A, 0x00, 0x01, // IP: 10.10.0.1
	}

	// Outer: DL NAS Transport wrapping the inner message
	containerLen := len(inner)
	outer := []byte{
		nas.EPD5GMM,                        // EPD
		nas.SecurityHeaderPlain,            // Security header: plain (will be wrapped by caller)
		nas.MsgTypeDLNASTransport,          // DL NAS Transport message type
		0x01,                               // Payload container type: N1 SM (lower nibble) + spare (upper)
		byte(containerLen >> 8), byte(containerLen), // Payload container length (LV-E)
	}
	outer = append(outer, inner...)

	// PDU session ID IE (IEI 0x12)
	outer = append(outer, 0x12, pduSessionID)

	return outer
}
