package models

import "time"

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
	ID        string    `json:"id"`
	SUPI      string    `json:"supi"`
	SNSSAI    SNSSAI    `json:"snssai"`
	DNN       string    `json:"dnn"`
	PDUType   string    `json:"pdu_type"` // IPv4, IPv6, IPv4v6
	UEAddress string    `json:"ue_address"`
	UPFID     string    `json:"upf_id"`
	State     string    `json:"state"` // ACTIVE, INACTIVE, RELEASED
	QFI       uint8     `json:"qfi"`
	AMBRUL    uint64    `json:"ambr_ul"`
	AMBRDL    uint64    `json:"ambr_dl"`
	CreatedAt time.Time `json:"created_at"`
}

// NFProfile represents an NF instance in the NRF, stored in etcd.
type NFProfile struct {
	NFInstanceID   string      `json:"nf_instance_id"`
	NFType         string      `json:"nf_type"`   // AMF, SMF, UDM, etc.
	NFStatus       string      `json:"nf_status"` // REGISTERED, SUSPENDED
	IPv4Addresses  []string    `json:"ipv4_addresses"`
	NFServices     []NFService `json:"nf_services"`
	PLMN           []PlmnID    `json:"plmn_list"`
	HeartbeatTimer int         `json:"heartbeat_timer"`
}

// NFService represents a service offered by an NF.
type NFService struct {
	ServiceInstanceID string   `json:"service_instance_id"`
	ServiceName       string   `json:"service_name"`
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
