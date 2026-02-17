package models

import (
	"net/http"
	"time"
)

// ProblemDetails represents an error response per 3GPP TS 29.571 Section 5.2.3
// and RFC 7807. All 3GPP SBI error responses use this format.
type ProblemDetails struct {
	Type   string `json:"type,omitempty"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
	Cause  string `json:"cause,omitempty"` // 3GPP application-level cause (TS 29.500 Section 5.2.7.2)
}

// NewProblemDetails creates a ProblemDetails with standard HTTP title mapping.
func NewProblemDetails(status int, cause string, detail string) *ProblemDetails {
	return &ProblemDetails{
		Title:  httpStatusTitle(status),
		Status: status,
		Cause:  cause,
		Detail: detail,
	}
}

func httpStatusTitle(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "Bad Request"
	case http.StatusUnauthorized:
		return "Unauthorized"
	case http.StatusForbidden:
		return "Forbidden"
	case http.StatusNotFound:
		return "Not Found"
	case http.StatusConflict:
		return "Conflict"
	case http.StatusInternalServerError:
		return "Internal Server Error"
	case http.StatusBadGateway:
		return "Bad Gateway"
	case http.StatusServiceUnavailable:
		return "Service Unavailable"
	default:
		return "Error"
	}
}

// UEContext represents a UE's state in the AMF, stored in Redis.
type UEContext struct {
	SUPI              string           `json:"supi"`
	GUTI              string           `json:"guti"`
	FiveGTMSI         string           `json:"5g_tmsi"`
	RegistrationState string           `json:"registration_state"` // REGISTERED, DEREGISTERED
	CmState           string           `json:"cm_state"`           // IDLE, CONNECTED
	GnbID             string           `json:"gnb_id"`
	AMFUeNgapID       int64            `json:"amf_ue_ngap_id"`
	RANUeNgapID       int64            `json:"ran_ue_ngap_id"`
	SecurityCtx       *SecurityContext `json:"security_ctx,omitempty"`
	PDUSessions       []string         `json:"pdu_sessions"`        // PDU session IDs
	NSSAI             []SNSSAI         `json:"nssai"`               // Requested NSSAI
	AllowedNSSAI      []SNSSAI         `json:"allowed_nssai"`       // Allowed NSSAI from subscription (TS 23.501 Section 5.15.4)
	T3512Value        uint32           `json:"t3512_value"`         // Periodic registration update timer in seconds (TS 24.501)
	RegistrationTime  time.Time        `json:"registration_time"`   // When RM state became REGISTERED
	LastActivity      time.Time        `json:"last_activity"`
}

// SecurityContext holds the UE's NAS security parameters (TS 33.501 Section 6.7).
type SecurityContext struct {
	KAMFKey           []byte `json:"kamf_key"`
	NASCount          uint32 `json:"nas_count"`
	AuthStatus        string `json:"auth_status"`        // AUTHENTICATED, PENDING
	NgKSI             uint8  `json:"ng_ksi"`              // NAS key set identifier (TS 24.501 Section 9.11.3.32)
	SelectedCiphering uint8  `json:"selected_ciphering"`  // 5G-EA algorithm (TS 24.501 Section 9.11.3.34)
	SelectedIntegrity uint8  `json:"selected_integrity"`  // 5G-IA algorithm (TS 24.501 Section 9.11.3.34)
	SecurityActivated bool   `json:"security_activated"`  // true after Security Mode Complete
}

// SNSSAI represents a Single Network Slice Selection Assistance Information.
type SNSSAI struct {
	SST int32  `json:"sst"`
	SD  string `json:"sd,omitempty"`
}

// PDUSession represents a PDU session in the SMF, stored in Redis.
type PDUSession struct {
	ID           string    `json:"id"`
	SUPI         string    `json:"supi"`
	SNSSAI       SNSSAI    `json:"snssai"`
	DNN          string    `json:"dnn"`
	PDUType      string    `json:"pdu_type"` // IPv4, IPv6, IPv4v6
	UEAddress    string    `json:"ue_address"`
	UPFID        string    `json:"upf_id"`
	State        string    `json:"state"` // ACTIVE, INACTIVE, RELEASED
	QFI          uint8     `json:"qfi"`
	AMBRUL       uint64    `json:"ambr_ul"`
	AMBRDL       uint64    `json:"ambr_dl"`
	ChargingID   string    `json:"charging_id,omitempty"`    // R17: CHF charging session ID
	BSFBindingID string    `json:"bsf_binding_id,omitempty"` // R17: BSF PCF binding ID
	CreatedAt    time.Time `json:"created_at"`
}

// NFProfile represents an NF instance in the NRF per TS 29.510 Section 6.1.6.2.2.
// JSON field names follow 3GPP OpenAPI specs (camelCase).
type NFProfile struct {
	NFInstanceID   string      `json:"nfInstanceId"`
	NFType         string      `json:"nfType"`            // AMF, SMF, UDM, etc.
	NFStatus       string      `json:"nfStatus"`          // REGISTERED, SUSPENDED
	IPv4Addresses  []string    `json:"ipv4Addresses"`
	NFServices     []NFService `json:"nfServices"`
	PLMN           []PlmnID    `json:"plmnList"`
	HeartbeatTimer int         `json:"heartBeatTimer"`
}

// NFService represents a service offered by an NF per TS 29.510.
type NFService struct {
	ServiceInstanceID string   `json:"serviceInstanceId"`
	ServiceName       string   `json:"serviceName"`
	Versions          []string `json:"versions"`
	Scheme            string   `json:"scheme"`
	FQDN              string   `json:"fqdn"`
}

// PlmnID represents a Public Land Mobile Network identity.
type PlmnID struct {
	MCC string `json:"mcc"`
	MNC string `json:"mnc"`
}

// SubscriberData represents subscriber info in UDR, stored in Redis.
type SubscriberData struct {
	SUPI               string         `json:"supi"`
	AuthenticationData *AuthData      `json:"auth_data"`
	AccessAndMobility  *AccessMobData `json:"access_mobility_data"`
	SessionManagement  []SMPolicyData `json:"session_management"`
}

// AuthData holds subscriber authentication credentials.
type AuthData struct {
	AuthMethod   string `json:"auth_method"` // 5G_AKA, EAP_AKA
	PermanentKey []byte `json:"k"`
	OPc          []byte `json:"opc"`
	AMF          []byte `json:"amf"` // Authentication Management Field (not AMF NF)
	SQN          []byte `json:"sqn"`
}

// AccessMobData holds subscriber access and mobility subscription data.
type AccessMobData struct {
	NSSAI      []SNSSAI `json:"nssai"`
	DefaultDNN string   `json:"default_dnn"`
}

// SMPolicyData holds session management policy data for a slice/DNN.
type SMPolicyData struct {
	SNSSAI SNSSAI `json:"snssai"`
	DNN    string `json:"dnn"`
	QoSRef int    `json:"qos_ref"`
}

// --- R17 Model Types ---

// ChargingSession represents a converged charging session per TS 32.291.
type ChargingSession struct {
	ChargingID      string    `json:"charging_id"`
	SUPI            string    `json:"supi"`
	PDUSessionID    string    `json:"pdu_session_id"`
	DNN             string    `json:"dnn"`
	SNSSAI          SNSSAI    `json:"snssai"`
	ChargingType    string    `json:"charging_type"` // ONLINE, OFFLINE
	State           string    `json:"state"`         // ACTIVE, RELEASED
	VolumeUplink    uint64    `json:"volume_uplink"`
	VolumeDownlink  uint64    `json:"volume_downlink"`
	GrantedUnits    uint64    `json:"granted_units"`
	CreatedAt       time.Time `json:"created_at"`
	LastUpdated     time.Time `json:"last_updated"`
}

// ChargingDataRecord represents a finalized CDR per TS 32.298.
type ChargingDataRecord struct {
	RecordID       string    `json:"record_id"`
	ChargingID     string    `json:"charging_id"`
	SUPI           string    `json:"supi"`
	PDUSessionID   string    `json:"pdu_session_id"`
	DNN            string    `json:"dnn"`
	SNSSAI         SNSSAI    `json:"snssai"`
	VolumeUplink   uint64    `json:"volume_uplink"`
	VolumeDownlink uint64    `json:"volume_downlink"`
	Duration       int64     `json:"duration_seconds"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
}

// SliceAdmissionPolicy defines admission limits per S-NSSAI per TS 29.536.
type SliceAdmissionPolicy struct {
	SST           int32  `json:"sst"`
	SD            string `json:"sd,omitempty"`
	MaxUEs        int64  `json:"max_ues"`
	MaxSessions   int64  `json:"max_sessions"`
}

// SliceCounters tracks current UE/session counts per S-NSSAI for NSACF.
type SliceCounters struct {
	SST            int32  `json:"sst"`
	SD             string `json:"sd,omitempty"`
	CurrentUEs     int64  `json:"current_ues"`
	CurrentSessions int64 `json:"current_sessions"`
}

// PCFBinding represents a PCF-to-PDU-session binding per TS 29.521.
type PCFBinding struct {
	BindingID    string `json:"binding_id"`
	SUPI         string `json:"supi"`
	DNN          string `json:"dnn"`
	SNSSAI       SNSSAI `json:"snssai"`
	PCFAddress   string `json:"pcf_address"`
	UEAddress    string `json:"ue_address,omitempty"`
	PDUSessionID string `json:"pdu_session_id"`
}

// AnalyticsSubscription represents an analytics subscription per TS 29.520.
type AnalyticsSubscription struct {
	SubscriptionID string `json:"subscription_id"`
	EventID        string `json:"event_id"` // NF_LOAD, SLICE_LOAD, UE_MOBILITY
	TargetNF       string `json:"target_nf,omitempty"`
	SNSSAI         *SNSSAI `json:"snssai,omitempty"`
	NotificationURI string `json:"notification_uri,omitempty"`
}

// NFLoadInfo holds NF load metrics collected by NWDAF.
type NFLoadInfo struct {
	NFInstanceID string    `json:"nf_instance_id"`
	NFType       string    `json:"nf_type"`
	CPUUsage     float64   `json:"cpu_usage"`
	MemUsage     float64   `json:"mem_usage"`
	NfLoad       int       `json:"nf_load"` // 0-100 percentage
	Timestamp    time.Time `json:"timestamp"`
}

// SliceLoadInfo holds per-slice load metrics collected by NWDAF.
type SliceLoadInfo struct {
	SST            int32     `json:"sst"`
	SD             string    `json:"sd,omitempty"`
	CurrentUEs     int64     `json:"current_ues"`
	CurrentSessions int64    `json:"current_sessions"`
	MeanNFLoad     float64   `json:"mean_nf_load"`
	Timestamp      time.Time `json:"timestamp"`
}
