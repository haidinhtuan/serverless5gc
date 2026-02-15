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

// MessageRoute maps an NGAP message to its target OpenFaaS function.
type MessageRoute struct {
	FunctionName  string
	ProcedureCode int
	MessageType   int
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
