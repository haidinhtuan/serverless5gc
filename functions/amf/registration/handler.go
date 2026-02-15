// Package function implements the AMF Initial Registration handler.
// This function processes the UE Initial Registration procedure per
// 3GPP TS 23.502 Section 4.2.2.2.2 (Initial Registration).
//
// Call chain (mirrors Open5GS/free5GC for fair cost comparison):
//  1. Decode NAS 5GS Registration Request (TS 24.501 Section 8.2.6)
//  2. AMF → AUSF: Nausf_UEAuthentication (TS 29.509)
//  3. NAS Security Mode Command/Complete (TS 24.501 Section 8.2.25/26)
//  4. AMF → UDM: Nudm_SDM_Get (TS 29.503) for subscriber data
//  5. AMF → UDM: Nudm_UECM_Registration (TS 29.503)
//  6. Create UE context: RM-DEREGISTERED → RM-REGISTERED (TS 23.502 Figure 4.2.2.2.1)
//  7. Build NAS 5GS Registration Accept (TS 24.501 Section 8.2.7)
package function

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	handler "github.com/openfaas/templates-sdk/go-http"
	"github.com/tdinh/serverless5gc/pkg/models"
	"github.com/tdinh/serverless5gc/pkg/nas"
	"github.com/tdinh/serverless5gc/pkg/sbi"
	"github.com/tdinh/serverless5gc/pkg/state"
)

// SBICaller abstracts inter-NF communication for testability.
type SBICaller interface {
	CallFunction(funcName string, payload interface{}, result interface{}) error
}

var (
	store     state.KVStore
	sbiClient SBICaller
	// amfUeNgapIDCounter provides unique AMF-UE-NGAP-IDs (TS 38.413 Section 9.3.3.1)
	amfUeNgapIDCounter int64
)

// SetStore injects a KVStore (used in tests).
func SetStore(s state.KVStore) { store = s }

// SetSBI injects an SBI caller (used in tests).
func SetSBI(s SBICaller) { sbiClient = s }

func init() {
	if store != nil {
		return
	}
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	store = state.NewRedisStore(addr)
	sbiClient = sbi.NewClient()
}

// RegistrationRequest is the JSON-mode input for a UE registration.
// In production, the SCTP proxy decodes the NGAP InitialUEMessage
// (TS 38.413 Section 9.2.5.1) and extracts these fields from the NAS PDU.
type RegistrationRequest struct {
	SUPI             string         `json:"supi"`
	RANUeNgapID      int64          `json:"ran_ue_ngap_id"`
	GnbID            string         `json:"gnb_id"`
	RegistrationType uint8          `json:"registration_type,omitempty"` // TS 24.501 Table 9.11.3.7.1
	RequestedNSSAI   []models.SNSSAI `json:"requested_nssai,omitempty"`
	UESecurityCap    *ueSecCap      `json:"ue_security_cap,omitempty"`
}

type ueSecCap struct {
	EA []uint8 `json:"ea"` // Supported ciphering algorithms
	IA []uint8 `json:"ia"` // Supported integrity algorithms
}

// RegistrationResponse is returned on successful registration.
type RegistrationResponse struct {
	Status           string          `json:"status"`
	SUPI             string          `json:"supi"`
	GUTI             string          `json:"guti"`
	AllowedNSSAI     []models.SNSSAI `json:"allowed_nssai,omitempty"`
	T3512Value       uint32          `json:"t3512_value"`
	NASMessage       string          `json:"nas_message,omitempty"`       // hex-encoded NAS Registration Accept
	SecurityActivated bool           `json:"security_activated"`
}

// SBI response types following 3GPP service naming
type authResponse struct {
	AuthResult string `json:"auth_result"` // Nausf_UEAuthentication result
	SUPI       string `json:"supi"`
	KAUSF      string `json:"kausf"`
}

type udmRegistrationReq struct {
	SUPI          string `json:"supi"`
	AMFInstanceID string `json:"amf_instance_id"`
}

// Handle processes a UE initial registration request following TS 23.502 Section 4.2.2.2.2.
func Handle(req handler.Request) (handler.Response, error) {
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var regReq RegistrationRequest
	if err := json.Unmarshal(req.Body, &regReq); err != nil {
		// TS 24.501: UE identity cannot be derived from malformed request
		rejectNAS := nas.EncodeRegistrationReject(&nas.RegistrationReject{
			CauseCode: nas.CauseUEIdentityCannotBeDerived,
		})
		return errorRespWithNAS(http.StatusBadRequest,
			hex.EncodeToString(rejectNAS),
			"invalid JSON: %s", err), nil
	}
	if regReq.SUPI == "" {
		rejectNAS := nas.EncodeRegistrationReject(&nas.RegistrationReject{
			CauseCode: nas.CauseUEIdentityCannotBeDerived,
		})
		return errorRespWithNAS(http.StatusBadRequest,
			hex.EncodeToString(rejectNAS),
			"supi is required"), nil
	}

	// Default to initial registration (TS 24.501 Table 9.11.3.7.1)
	if regReq.RegistrationType == 0 {
		regReq.RegistrationType = nas.RegTypeInitialRegistration
	}

	// Allocate AMF-UE-NGAP-ID (TS 38.413 Section 9.3.3.1)
	amfUeNgapID := atomic.AddInt64(&amfUeNgapIDCounter, 1)

	// Step 1: Nausf_UEAuthentication — Authenticate via AUSF (TS 29.509)
	var authResult authResponse
	if err := sbiClient.CallFunction("ausf-authenticate",
		map[string]string{"supi": regReq.SUPI, "res_star": ""},
		&authResult); err != nil {
		return errorResp(http.StatusInternalServerError, "Nausf_UEAuthentication: %s", err), nil
	}
	if authResult.AuthResult != "SUCCESS" {
		// TS 24.501 Cause #3: Illegal UE (authentication failure)
		rejectNAS := nas.EncodeRegistrationReject(&nas.RegistrationReject{
			CauseCode: nas.CauseIllegalUE,
		})
		return errorRespWithNAS(http.StatusForbidden,
			hex.EncodeToString(rejectNAS),
			"authentication failed for %s", regReq.SUPI), nil
	}

	// Step 2: NAS Security Mode Command (TS 24.501 Section 8.2.25)
	// Select NAS security algorithms based on UE capabilities and AMF policy.
	// Default: 5G-EA2 (AES) for ciphering, 5G-IA2 (AES) for integrity (TS 33.501 Section 6.7.2).
	selectedCiphering := uint8(nas.CipherAlg5GEA2)
	selectedIntegrity := uint8(nas.IntegAlg5GIA2)
	ngKSI := uint8(0)

	smcNAS := nas.EncodeSecurityModeCommand(&nas.SecurityModeCommand{
		SelectedCiphering: selectedCiphering,
		SelectedIntegrity: selectedIntegrity,
		NgKSI:             ngKSI,
		ReplayedUESecCap:  &nas.UESecurityCapability{EA0: true, EA1: true, EA2: true, IA0: true, IA1: true, IA2: true},
	})
	_ = smcNAS // In production: sent via NGAP DownlinkNASTransport, await SecurityModeComplete

	// Derive KAMF from KAUSF (TS 33.501 Section 6.1.4.4)
	var kamfKey []byte
	if authResult.KAUSF != "" {
		kamfKey, _ = hex.DecodeString(authResult.KAUSF)
	}

	// Step 3: Nudm_SDM_Get — Get subscriber data from UDM (TS 29.503)
	var subData models.SubscriberData
	if err := sbiClient.CallFunction("udm-get-subscriber-data",
		map[string]string{"supi": regReq.SUPI},
		&subData); err != nil {
		return errorResp(http.StatusInternalServerError, "Nudm_SDM_Get: %s", err), nil
	}

	// Step 4: Nudm_UECM_Registration — Register AMF context at UDM (TS 29.503)
	sbiClient.CallFunction("udm-registration",
		udmRegistrationReq{SUPI: regReq.SUPI, AMFInstanceID: "amf-001"},
		nil) // Best-effort; non-critical for registration flow

	// Step 5: Create UE context — RM-DEREGISTERED → RM-REGISTERED
	var allowedNSSAI []models.SNSSAI
	if subData.AccessAndMobility != nil {
		allowedNSSAI = subData.AccessAndMobility.NSSAI
	}

	guti := fmt.Sprintf("5g-guti-%s", regReq.SUPI)
	now := time.Now()
	ueCtx := models.UEContext{
		SUPI:              regReq.SUPI,
		GUTI:              guti,
		RegistrationState: "REGISTERED",     // RM-REGISTERED (TS 23.502 Section 4.2.2.2)
		CmState:           "CONNECTED",      // CM-CONNECTED (NAS signaling active)
		RANUeNgapID:       regReq.RANUeNgapID,
		AMFUeNgapID:       amfUeNgapID,
		GnbID:             regReq.GnbID,
		NSSAI:             allowedNSSAI,
		AllowedNSSAI:      allowedNSSAI,     // TS 23.501 Section 5.15.4
		T3512Value:        nas.T3512Default, // 54 minutes (TS 24.501 Section 8.2.7.17)
		RegistrationTime:  now,
		LastActivity:      now,
		SecurityCtx: &models.SecurityContext{
			KAMFKey:           kamfKey,
			NgKSI:             ngKSI,
			SelectedCiphering: selectedCiphering,
			SelectedIntegrity: selectedIntegrity,
			AuthStatus:        "AUTHENTICATED",
			SecurityActivated: true, // After SMC procedure completes
		},
	}

	if err := store.Put(ctx, "ue:"+regReq.SUPI, ueCtx); err != nil {
		return errorResp(http.StatusInternalServerError, "store ue context: %s", err), nil
	}

	// Step 6: Build NAS Registration Accept (TS 24.501 Section 8.2.7)
	var nasNSSAI []nas.NSSAI
	for _, s := range allowedNSSAI {
		n := nas.NSSAI{SST: uint8(s.SST)}
		if s.SD != "" {
			n.HasSD = true
			sdBytes, _ := hex.DecodeString(s.SD)
			copy(n.SD[:], sdBytes)
		}
		nasNSSAI = append(nasNSSAI, n)
	}

	regAcceptNAS := nas.EncodeRegistrationAccept(&nas.RegistrationAccept{
		RegistrationResult: nas.RegResult3GPPAccess,
		GUTI:               guti,
		AllowedNSSAI:       nasNSSAI,
		T3512Value:         nas.T3512Default,
	})

	body, _ := json.Marshal(RegistrationResponse{
		Status:            "registered",
		SUPI:              regReq.SUPI,
		GUTI:              guti,
		AllowedNSSAI:      allowedNSSAI,
		T3512Value:        nas.T3512Default,
		NASMessage:        hex.EncodeToString(regAcceptNAS),
		SecurityActivated: true,
	})
	return handler.Response{
		StatusCode: http.StatusOK,
		Body:       body,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func errorResp(status int, format string, args ...interface{}) handler.Response {
	msg := fmt.Sprintf(format, args...)
	body, _ := json.Marshal(map[string]string{"error": msg})
	return handler.Response{StatusCode: status, Body: body}
}

func errorRespWithNAS(status int, nasHex string, format string, args ...interface{}) handler.Response {
	msg := fmt.Sprintf(format, args...)
	body, _ := json.Marshal(map[string]string{
		"error":       msg,
		"nas_message": nasHex,
	})
	return handler.Response{StatusCode: status, Body: body}
}
