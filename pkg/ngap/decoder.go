package ngap

import "fmt"

// NGAP procedure codes from 3GPP TS 38.413 Section 9.4.
const (
	ProcedureCodeHandoverPreparation        = 0
	ProcedureCodeHandoverResourceAllocation = 1
	ProcedureCodePathSwitchRequest          = 3
	ProcedureCodeDownlinkNASTransport       = 4
	ProcedureCodeInitialContextSetup        = 14
	ProcedureCodeInitialUEMessage           = 15
	ProcedureCodeNGReset                    = 20
	ProcedureCodeNGSetup                    = 21
	ProcedureCodePDUSessionResourceSetup    = 26
	ProcedureCodePDUSessionResourceRelease  = 28
	ProcedureCodeUEContextRelease           = 41
	ProcedureCodeUEContextReleaseRequest    = 42
	ProcedureCodeUplinkNASTransport         = 46
)

// NGAP-PDU CHOICE indices (APER-encoded in byte 0).
const (
	MessageTypeInitiating   = 0
	MessageTypeSuccessful   = 1
	MessageTypeUnsuccessful = 2
)

// NGAP IE IDs from 3GPP TS 38.413 Section 9.3.
const (
	IEID_AMF_UE_NGAP_ID    = 10
	IEID_RAN_UE_NGAP_ID    = 85
	IEID_NAS_PDU           = 38
	IEID_UserLocationInfo  = 121
	IEID_RRCEstablishCause = 90
	IEID_FiveG_S_TMSI      = 26
)

// MessageRoute maps an NGAP message to its target OpenFaaS function.
type MessageRoute struct {
	FunctionName  string
	ProcedureCode int
	MessageType   int
}

// NGAPContext carries extracted NGAP IE values alongside routing info.
// These fields are populated by ParseNGAPMessage from the wire-format PDU
// per TS 38.413 Section 9.2 (InitialUEMessage, UplinkNASTransport, etc.).
type NGAPContext struct {
	Route          MessageRoute
	RANUeNgapID    int64  // TS 38.413 Section 9.3.3.2
	AMFUeNgapID    int64  // TS 38.413 Section 9.3.3.1
	NASPDU         []byte // TS 38.413 Section 9.3.3.5
	UserLocationInfo []byte // TS 38.413 Section 9.3.1.16 (raw)
}

// routingTable maps (messageType, procedureCode) to an OpenFaaS function name.
var routingTable = map[[2]int]string{
	{MessageTypeInitiating, ProcedureCodeInitialUEMessage}:        "amf-initial-registration",
	{MessageTypeInitiating, ProcedureCodeUEContextReleaseRequest}: "amf-deregistration",
	{MessageTypeInitiating, ProcedureCodeUplinkNASTransport}:      "amf-uplink-nas",
	{MessageTypeInitiating, ProcedureCodeNGSetup}:                 "amf-ng-setup",
	{MessageTypeInitiating, ProcedureCodeHandoverPreparation}:     "amf-handover",
	{MessageTypeInitiating, ProcedureCodePathSwitchRequest}:       "amf-path-switch",
	{MessageTypeInitiating, ProcedureCodeUEContextRelease}:        "amf-context-release",
	{MessageTypeInitiating, ProcedureCodeDownlinkNASTransport}:   "amf-downlink-nas",
}

// ParseNGAPMessage decodes an NGAP PDU and extracts both routing information and
// IE values per TS 38.413 Section 9.2. The simplified wire format used here:
//
//	Byte 0: NGAP-PDU choice byte (same as RouteNGAP)
//	Byte 1: procedureCode
//	Byte 2: criticality
//	Byte 3-4: number of IEs (big-endian)
//	Then for each IE:
//	  Byte 0-1: IE ID (big-endian)
//	  Byte 2: criticality
//	  Byte 3-4: value length (big-endian)
//	  Byte 5+: value
//
// Extracts: RAN-UE-NGAP-ID, AMF-UE-NGAP-ID, NAS-PDU, UserLocationInfo.
func ParseNGAPMessage(data []byte) (*NGAPContext, error) {
	if len(data) < 3 {
		return nil, fmt.Errorf("ngap pdu too short: %d bytes", len(data))
	}

	route, err := RouteNGAP(data)
	if err != nil {
		return nil, err
	}

	ctx := &NGAPContext{
		Route: *route,
	}

	// Parse IE list if enough data is present
	if len(data) < 5 {
		return ctx, nil
	}

	numIEs := int(data[3])<<8 | int(data[4])
	offset := 5

	for i := 0; i < numIEs && offset+5 <= len(data); i++ {
		ieID := int(data[offset])<<8 | int(data[offset+1])
		// skip criticality byte
		vLen := int(data[offset+3])<<8 | int(data[offset+4])
		offset += 5

		if offset+vLen > len(data) {
			break
		}
		ieValue := data[offset : offset+vLen]
		offset += vLen

		switch ieID {
		case IEID_RAN_UE_NGAP_ID:
			if len(ieValue) >= 4 {
				ctx.RANUeNgapID = int64(ieValue[0])<<24 | int64(ieValue[1])<<16 |
					int64(ieValue[2])<<8 | int64(ieValue[3])
			}
		case IEID_AMF_UE_NGAP_ID:
			if len(ieValue) >= 4 {
				ctx.AMFUeNgapID = int64(ieValue[0])<<24 | int64(ieValue[1])<<16 |
					int64(ieValue[2])<<8 | int64(ieValue[3])
			}
		case IEID_NAS_PDU:
			ctx.NASPDU = make([]byte, len(ieValue))
			copy(ctx.NASPDU, ieValue)
		case IEID_UserLocationInfo:
			ctx.UserLocationInfo = make([]byte, len(ieValue))
			copy(ctx.UserLocationInfo, ieValue)
		}
	}

	return ctx, nil
}

// BuildDownlinkNASTransport constructs a simplified DownlinkNASTransport NGAP PDU
// containing AMF-UE-NGAP-ID, RAN-UE-NGAP-ID, and NAS-PDU IEs.
// Used by the AMF to send NAS messages to the UE via the gNB (TS 38.413 Section 9.2.3.1).
func BuildDownlinkNASTransport(amfUeNgapID, ranUeNgapID int64, nasPDU []byte) []byte {
	// Header: initiatingMessage, DownlinkNASTransport, criticality
	msg := []byte{
		0x00,                                    // initiatingMessage
		byte(ProcedureCodeDownlinkNASTransport), // procedureCode 4
		0x00,                                    // criticality: reject
	}

	// Number of IEs = 3
	msg = append(msg, 0x00, 0x03)

	// IE: AMF-UE-NGAP-ID
	msg = append(msg, byte(IEID_AMF_UE_NGAP_ID>>8), byte(IEID_AMF_UE_NGAP_ID&0xFF))
	msg = append(msg, 0x00) // criticality
	msg = append(msg, 0x00, 0x04) // length = 4
	msg = append(msg, byte(amfUeNgapID>>24), byte(amfUeNgapID>>16), byte(amfUeNgapID>>8), byte(amfUeNgapID))

	// IE: RAN-UE-NGAP-ID
	msg = append(msg, byte(IEID_RAN_UE_NGAP_ID>>8), byte(IEID_RAN_UE_NGAP_ID&0xFF))
	msg = append(msg, 0x00) // criticality
	msg = append(msg, 0x00, 0x04) // length = 4
	msg = append(msg, byte(ranUeNgapID>>24), byte(ranUeNgapID>>16), byte(ranUeNgapID>>8), byte(ranUeNgapID))

	// IE: NAS-PDU
	msg = append(msg, byte(IEID_NAS_PDU>>8), byte(IEID_NAS_PDU&0xFF))
	msg = append(msg, 0x00) // criticality
	nasLen := len(nasPDU)
	msg = append(msg, byte(nasLen>>8), byte(nasLen&0xFF))
	msg = append(msg, nasPDU...)

	return msg
}

// RouteNGAP decodes the APER-encoded NGAP-PDU header and returns routing info.
// Only the first two bytes are needed: message type and procedure code.
//
// NGAP-PDU wire format (APER):
//
//	Byte 0: [ext(1) | choice_index(2) | padding(5)]
//	  choice: 0=initiatingMessage, 1=successfulOutcome, 2=unsuccessfulOutcome
//	Byte 1: procedureCode (INTEGER 0..255)
func RouteNGAP(data []byte) (*MessageRoute, error) {
	if len(data) < 3 {
		return nil, fmt.Errorf("ngap pdu too short: %d bytes", len(data))
	}

	msgType := int((data[0] >> 5) & 0x03)
	procedureCode := int(data[1])

	key := [2]int{msgType, procedureCode}
	funcName, ok := routingTable[key]
	if !ok {
		return nil, fmt.Errorf("no route for message type %d, procedure code %d", msgType, procedureCode)
	}

	return &MessageRoute{
		FunctionName:  funcName,
		ProcedureCode: procedureCode,
		MessageType:   msgType,
	}, nil
}
