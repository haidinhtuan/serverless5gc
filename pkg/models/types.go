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
	PDUSessions       []string         `json:"pdu_sessions"` // PDU session IDs
	NSSAI             []SNSSAI         `json:"nssai"`
	LastActivity      time.Time        `json:"last_activity"`
}

// SecurityContext holds the UE's security parameters.
type SecurityContext struct {
	KAMFKey    []byte `json:"kamf_key"`
	NASCount   uint32 `json:"nas_count"`
	AuthStatus string `json:"auth_status"` // AUTHENTICATED, PENDING
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
