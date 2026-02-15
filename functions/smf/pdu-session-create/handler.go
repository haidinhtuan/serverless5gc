// Package function implements the SMF Nsmf_PDUSession_CreateSMContext service operation
// per 3GPP TS 29.502 and TS 23.502 Section 4.3.2.
package function

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/models"
	"github.com/tdinh/serverless5gc/pkg/pfcp"
	"github.com/tdinh/serverless5gc/pkg/state"
)

// SBICaller abstracts inter-NF communication for testability.
type SBICaller interface {
	CallFunction(funcName string, payload interface{}, result interface{}) error
}

// PFCPSessionManager abstracts PFCP operations for testability.
type PFCPSessionManager interface {
	EstablishSession(seid uint64, ueIP string, teid uint32) error
}

// Store is the backing KV store. Override in tests via SetStore.
var Store state.KVStore

// SBI is the inter-NF client. Override in tests via SetSBI.
var SBI SBICaller

// PFCP is the PFCP client for UPF communication. Override in tests via SetPFCP.
var PFCP PFCPSessionManager

func SetStore(s state.KVStore) { Store = s }
func SetSBI(s SBICaller) { SBI = s }
func SetPFCP(p PFCPSessionManager) { PFCP = p }

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

// CreateSMContextRequest per TS 29.502 Section 6.1.6.2.4 (Nsmf_PDUSession_CreateSMContext).
// In production this would carry the NAS PDU Session Establishment Request (TS 24.501 Section 8.3.1)
// containing PDU Session ID, PDU Session Type, SSC Mode, and requested QoS rules.
type CreateSMContextRequest struct {
	SUPI       string        `json:"supi"`
	SNSSAI     models.SNSSAI `json:"snssai"`
	DNN        string        `json:"dnn"`
	PDUType    string        `json:"pdu_session_type,omitempty"` // TS 24.501: IPv4, IPv6, IPv4v6
	SSCMode    uint8         `json:"ssc_mode,omitempty"`         // TS 24.501: SSC mode 1/2/3
	SessionID  uint8         `json:"pdu_session_id,omitempty"`   // TS 24.501: 1-15
	SessionAMBRUL uint64    `json:"session_ambr_ul,omitempty"`   // from subscription or request
	SessionAMBRDL uint64    `json:"session_ambr_dl,omitempty"`
}

// CreateSMContextResponse per TS 29.502 Section 6.1.6.2.4.
// In production this would include the NAS PDU Session Establishment Accept (TS 24.501 Section 8.3.2)
// containing allocated QoS rules, Session-AMBR, PDU address, DNN, S-NSSAI.
type CreateSMContextResponse struct {
	SessionID string `json:"session_id"`
	UEAddress string `json:"ue_address"`
	State     string `json:"state"`
	QFI       uint8  `json:"qfi"`
	AMBRUL    uint64 `json:"session_ambr_ul"`
	AMBRDL    uint64 `json:"session_ambr_dl"`
	DNN       string `json:"dnn"`
}

// SmPolicyCreateRequest sent to PCF per TS 29.512 (Npcf_SMPolicyControl_Create).
type SmPolicyCreateRequest struct {
	SUPI   string        `json:"supi"`
	SNSSAI models.SNSSAI `json:"snssai"`
	DNN    string        `json:"dnn"`
}

// SmPolicyDecision returned from PCF per TS 29.512 Section 5.6.2.2.
type SmPolicyDecision struct {
	PolicyID string        `json:"policy_id"`
	QFI      uint8         `json:"qfi"`
	AMBRUL   uint64        `json:"ambr_ul"`
	AMBRDL   uint64        `json:"ambr_dl"`
	FiveQI   int           `json:"5qi"`
	SessRules map[string]SessionRule `json:"sess_rules,omitempty"`
}

// SessionRule per TS 29.512 Section 5.6.2.7.
type SessionRule struct {
	SessionAMBR *AMBR `json:"sess_ambr,omitempty"`
}

// AMBR per TS 29.571.
type AMBR struct {
	Uplink   uint64 `json:"uplink"`
	Downlink uint64 `json:"downlink"`
}

// IP pool management - tracks allocated IPs in Redis to prevent duplicates
// per TS 29.244 Section 5.21.
var (
	ipPoolMu  sync.Mutex
	ipPoolIdx uint32 = 1
)

// IPPoolBase is the base network for UE address allocation (configurable via UE_IP_POOL env).
// Default: 10.45.0.0/16 per common 5GC deployments.
var IPPoolBase = "10.45.0"

func init() {
	if base := os.Getenv("UE_IP_POOL"); base != "" {
		IPPoolBase = base
	}
}

// allocateIP assigns a UE IPv4 address from the pool and records it in Redis.
func allocateIP(ctx context.Context) (string, error) {
	ipPoolMu.Lock()
	defer ipPoolMu.Unlock()

	// Try up to 254 addresses in the current /24 block
	for attempts := 0; attempts < 254; attempts++ {
		ip := fmt.Sprintf("%s.%d", IPPoolBase, ipPoolIdx)
		ipPoolIdx++
		if ipPoolIdx > 254 {
			ipPoolIdx = 1
		}

		// Check if IP is already allocated (tracked in Redis)
		key := "ip-pool/allocated/" + ip
		var existing string
		if err := Store.Get(ctx, key, &existing); err != nil {
			// Key not found = IP is available
			if err := Store.Put(ctx, key, ip); err != nil {
				return "", fmt.Errorf("record IP allocation: %w", err)
			}
			return ip, nil
		}
		// IP already allocated, try next
	}
	return "", fmt.Errorf("IP pool exhausted")
}

// releaseIP returns an IP address to the pool.
func releaseIP(ctx context.Context, ip string) {
	Store.Delete(ctx, "ip-pool/allocated/"+ip)
}

// ResetIPPool resets the IP pool counter (for testing).
func ResetIPPool() {
	ipPoolMu.Lock()
	defer ipPoolMu.Unlock()
	ipPoolIdx = 1
}

var seidCounter uint64
var seidMu sync.Mutex

func nextSEID() uint64 {
	seidMu.Lock()
	defer seidMu.Unlock()
	seidCounter++
	return seidCounter
}

// Handle processes Nsmf_PDUSession_CreateSMContext (TS 29.502 Section 6.1.6.2.4).
// Follows the PDU Session Establishment procedure per TS 23.502 Section 4.3.2:
//  1. Receive CreateSMContext from AMF (Nsmf_PDUSession)
//  2. Call PCF for SM policy (Npcf_SMPolicyControl_Create, TS 29.512)
//  3. Allocate UE IP address from pool (TS 29.244 Section 5.21)
//  4. Send PFCP Session Establishment to UPF (N4, TS 29.244 Section 7.5.2)
//  5. Store PDU session in Redis
//  6. Return SM context response to AMF
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var smReq CreateSMContextRequest
	if err := json.Unmarshal(req.Body, &smReq); err != nil {
		return errorResp(http.StatusBadRequest, "invalid JSON: %s", err), nil
	}
	if smReq.SUPI == "" {
		return errorResp(http.StatusBadRequest, "supi is required"), nil
	}
	if smReq.DNN == "" {
		smReq.DNN = "internet"
	}
	if smReq.PDUType == "" {
		smReq.PDUType = "IPv4"
	}

	// Step 2: Call PCF for SM policy (Npcf_SMPolicyControl_Create, TS 29.512)
	var policyResp SmPolicyDecision
	if SBI != nil {
		pcfReq := SmPolicyCreateRequest{
			SUPI:   smReq.SUPI,
			SNSSAI: smReq.SNSSAI,
			DNN:    smReq.DNN,
		}
		if err := SBI.CallFunction("pcf-policy-create", pcfReq, &policyResp); err != nil {
			return errorResp(http.StatusInternalServerError, "Npcf_SMPolicyControl_Create: %s", err), nil
		}
	} else {
		// Default policy per TS 23.501 Section 5.7: QFI=1, 5QI=9 (best effort)
		policyResp = SmPolicyDecision{QFI: 1, AMBRUL: 1000000, AMBRDL: 5000000, FiveQI: 9}
	}

	// Step 3: Allocate UE IP address from pool (TS 29.244 Section 5.21)
	ueIP, err := allocateIP(ctx)
	if err != nil {
		return errorResp(http.StatusInternalServerError, "IP allocation: %s", err), nil
	}
	if net.ParseIP(ueIP) == nil {
		releaseIP(ctx, ueIP)
		return errorResp(http.StatusInternalServerError, "invalid allocated IP: %s", ueIP), nil
	}

	seid := nextSEID()
	teid := uint32(seid)

	// Step 4: Send PFCP Session Establishment to UPF (N4, TS 29.244 Section 7.5.2)
	if PFCP != nil {
		if err := PFCP.EstablishSession(seid, ueIP, teid); err != nil {
			releaseIP(ctx, ueIP) // release IP on PFCP failure
			return errorResp(http.StatusInternalServerError, "N4 Session Establishment: %s", err), nil
		}
	}

	// Use policy QoS values, override with subscription AMBR if provided
	ambrUL := policyResp.AMBRUL
	ambrDL := policyResp.AMBRDL
	if smReq.SessionAMBRUL > 0 {
		ambrUL = smReq.SessionAMBRUL
	}
	if smReq.SessionAMBRDL > 0 {
		ambrDL = smReq.SessionAMBRDL
	}

	// Step 5: Store PDU session in Redis
	sessionID := fmt.Sprintf("pdu-%s-%d", smReq.SUPI, seid)
	session := models.PDUSession{
		ID:        sessionID,
		SUPI:      smReq.SUPI,
		SNSSAI:    smReq.SNSSAI,
		DNN:       smReq.DNN,
		PDUType:   smReq.PDUType,
		UEAddress: ueIP,
		UPFID:     os.Getenv("UPF_PFCP_ADDR"),
		State:     "ACTIVE",
		QFI:       policyResp.QFI,
		AMBRUL:    ambrUL,
		AMBRDL:    ambrDL,
		CreatedAt: time.Now(),
	}

	key := "pdu-sessions/" + sessionID
	if err := Store.Put(ctx, key, session); err != nil {
		releaseIP(ctx, ueIP)
		return errorResp(http.StatusInternalServerError, "store session: %s", err), nil
	}

	// Step 6: Return SM context response to AMF
	// In production this would include the NAS PDU Session Establishment Accept
	// (TS 24.501 Section 8.3.2) with QoS rules, Session-AMBR, PDU address
	resp := CreateSMContextResponse{
		SessionID: sessionID,
		UEAddress: ueIP,
		State:     "ACTIVE",
		QFI:       policyResp.QFI,
		AMBRUL:    ambrUL,
		AMBRDL:    ambrDL,
		DNN:       smReq.DNN,
	}
	body, _ := json.Marshal(resp)
	return handler.Response{
		StatusCode: http.StatusCreated,
		Body:       body,
		Header:     jsonHeader(),
	}, nil
}

func errorResp(code int, format string, args ...interface{}) handler.Response {
	msg := fmt.Sprintf(format, args...)
	return handler.Response{
		StatusCode: code,
		Body:       []byte(fmt.Sprintf(`{"error":"%s"}`, msg)),
		Header:     jsonHeader(),
	}
}

func jsonHeader() http.Header {
	return http.Header{"Content-Type": []string{"application/json"}}
}

// Ensure pfcp.Client satisfies PFCPSessionManager.
var _ PFCPSessionManager = (*pfcp.Client)(nil)
