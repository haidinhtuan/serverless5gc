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

// CreateSMContextRequest is the input from the AMF.
type CreateSMContextRequest struct {
	SUPI   string       `json:"supi"`
	SNSSAI models.SNSSAI `json:"snssai"`
	DNN    string       `json:"dnn"`
}

// CreateSMContextResponse is returned to the AMF.
type CreateSMContextResponse struct {
	SessionID string `json:"session_id"`
	UEAddress string `json:"ue_address"`
	State     string `json:"state"`
}

// PolicyCreateRequest is sent to PCF.
type PolicyCreateRequest struct {
	SUPI   string       `json:"supi"`
	SNSSAI models.SNSSAI `json:"snssai"`
	DNN    string       `json:"dnn"`
}

// PolicyCreateResponse is returned from PCF.
type PolicyCreateResponse struct {
	PolicyID string `json:"policy_id"`
	QFI      uint8  `json:"qfi"`
	AMBRUL   uint64 `json:"ambr_ul"`
	AMBRDL   uint64 `json:"ambr_dl"`
}

// IP pool management
var (
	ipPoolMu  sync.Mutex
	ipPoolIdx uint32 = 1
)

// IPPoolBase is the base IP for UE address allocation. Configurable via UE_IP_POOL env.
var IPPoolBase = "10.45.0"

func init() {
	if base := os.Getenv("UE_IP_POOL"); base != "" {
		IPPoolBase = base
	}
}

func allocateIP() string {
	ipPoolMu.Lock()
	defer ipPoolMu.Unlock()
	ip := fmt.Sprintf("%s.%d", IPPoolBase, ipPoolIdx)
	ipPoolIdx++
	if ipPoolIdx > 254 {
		ipPoolIdx = 1
	}
	return ip
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

// Handle processes a CreateSMContext request from the AMF.
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

	// Allocate UE IP address
	ueIP := allocateIP()
	if net.ParseIP(ueIP) == nil {
		return errorResp(http.StatusInternalServerError, "invalid allocated IP: %s", ueIP), nil
	}

	// Call PCF for QoS policy
	var policyResp PolicyCreateResponse
	if SBI != nil {
		pcfReq := PolicyCreateRequest{
			SUPI:   smReq.SUPI,
			SNSSAI: smReq.SNSSAI,
			DNN:    smReq.DNN,
		}
		if err := SBI.CallFunction("pcf-policy-create", pcfReq, &policyResp); err != nil {
			return errorResp(http.StatusInternalServerError, "pcf-policy-create: %s", err), nil
		}
	} else {
		// Default policy if SBI not available
		policyResp = PolicyCreateResponse{QFI: 9, AMBRUL: 1000000, AMBRDL: 5000000}
	}

	seid := nextSEID()
	teid := uint32(seid)

	// Send PFCP Session Establishment to UPF
	if PFCP != nil {
		if err := PFCP.EstablishSession(seid, ueIP, teid); err != nil {
			return errorResp(http.StatusInternalServerError, "pfcp establish: %s", err), nil
		}
	}

	// Store PDU session in Redis
	sessionID := fmt.Sprintf("pdu-%s-%d", smReq.SUPI, seid)
	session := models.PDUSession{
		ID:        sessionID,
		SUPI:      smReq.SUPI,
		SNSSAI:    smReq.SNSSAI,
		DNN:       smReq.DNN,
		PDUType:   "IPv4",
		UEAddress: ueIP,
		UPFID:     os.Getenv("UPF_PFCP_ADDR"),
		State:     "ACTIVE",
		QFI:       policyResp.QFI,
		AMBRUL:    policyResp.AMBRUL,
		AMBRDL:    policyResp.AMBRDL,
		CreatedAt: time.Now(),
	}

	key := "pdu-sessions/" + sessionID
	if err := Store.Put(ctx, key, session); err != nil {
		return errorResp(http.StatusInternalServerError, "store session: %s", err), nil
	}

	resp := CreateSMContextResponse{
		SessionID: sessionID,
		UEAddress: ueIP,
		State:     "ACTIVE",
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
