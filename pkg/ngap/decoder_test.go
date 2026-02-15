package ngap

import "testing"

func TestRouteNGAP_InitialUEMessage(t *testing.T) {
	// Byte 0: 0x00 = initiatingMessage, Byte 1: 0x0f = procedureCode 15
	pdu := []byte{0x00, 0x0f, 0x40, 0x01, 0x00}
	route, err := RouteNGAP(pdu)
	if err != nil {
		t.Fatalf("RouteNGAP error: %v", err)
	}
	if route.FunctionName != "amf-initial-registration" {
		t.Errorf("FunctionName = %q, want %q", route.FunctionName, "amf-initial-registration")
	}
	if route.ProcedureCode != ProcedureCodeInitialUEMessage {
		t.Errorf("ProcedureCode = %d, want %d", route.ProcedureCode, ProcedureCodeInitialUEMessage)
	}
	if route.MessageType != MessageTypeInitiating {
		t.Errorf("MessageType = %d, want %d", route.MessageType, MessageTypeInitiating)
	}
}

func TestRouteNGAP_UEContextReleaseRequest(t *testing.T) {
	// procedureCode 42 = 0x2a
	pdu := []byte{0x00, 0x2a, 0x40, 0x01, 0x00}
	route, err := RouteNGAP(pdu)
	if err != nil {
		t.Fatalf("RouteNGAP error: %v", err)
	}
	if route.FunctionName != "amf-deregistration" {
		t.Errorf("FunctionName = %q, want %q", route.FunctionName, "amf-deregistration")
	}
	if route.ProcedureCode != ProcedureCodeUEContextReleaseRequest {
		t.Errorf("ProcedureCode = %d, want %d", route.ProcedureCode, ProcedureCodeUEContextReleaseRequest)
	}
}

func TestRouteNGAP_UplinkNASTransport(t *testing.T) {
	// procedureCode 46 = 0x2e
	pdu := []byte{0x00, 0x2e, 0x00, 0x01, 0x00}
	route, err := RouteNGAP(pdu)
	if err != nil {
		t.Fatalf("RouteNGAP error: %v", err)
	}
	if route.FunctionName != "amf-uplink-nas" {
		t.Errorf("FunctionName = %q, want %q", route.FunctionName, "amf-uplink-nas")
	}
}

func TestRouteNGAP_NGSetup(t *testing.T) {
	// procedureCode 21 = 0x15
	pdu := []byte{0x00, 0x15, 0x00, 0x01, 0x00}
	route, err := RouteNGAP(pdu)
	if err != nil {
		t.Fatalf("RouteNGAP error: %v", err)
	}
	if route.FunctionName != "amf-ng-setup" {
		t.Errorf("FunctionName = %q, want %q", route.FunctionName, "amf-ng-setup")
	}
}

func TestRouteNGAP_HandoverPreparation(t *testing.T) {
	// procedureCode 0 = 0x00
	pdu := []byte{0x00, 0x00, 0x00, 0x01, 0x00}
	route, err := RouteNGAP(pdu)
	if err != nil {
		t.Fatalf("RouteNGAP error: %v", err)
	}
	if route.FunctionName != "amf-handover" {
		t.Errorf("FunctionName = %q, want %q", route.FunctionName, "amf-handover")
	}
}

func TestRouteNGAP_TooShort(t *testing.T) {
	_, err := RouteNGAP([]byte{0x00, 0x0f})
	if err == nil {
		t.Fatal("expected error for PDU shorter than 3 bytes")
	}
}

func TestRouteNGAP_EmptyInput(t *testing.T) {
	_, err := RouteNGAP(nil)
	if err == nil {
		t.Fatal("expected error for nil input")
	}
}

func TestRouteNGAP_UnknownProcedureCode(t *testing.T) {
	pdu := []byte{0x00, 0xff, 0x00, 0x00, 0x00}
	_, err := RouteNGAP(pdu)
	if err == nil {
		t.Fatal("expected error for unknown procedure code 255")
	}
}

func TestRouteNGAP_SuccessfulOutcomeNoRoute(t *testing.T) {
	// Byte 0: 0x20 = successfulOutcome (choice index 1)
	pdu := []byte{0x20, 0x15, 0x00, 0x00, 0x00}
	_, err := RouteNGAP(pdu)
	if err == nil {
		t.Fatal("expected error for successful outcome without registered route")
	}
}

func TestParseNGAPMessage_InitialUEMessage(t *testing.T) {
	// Construct a simplified InitialUEMessage with IEs encoded in our TLV format:
	// [NGAP header: 3 bytes] [criticality: 1 byte] [numIEs: 2 bytes]
	// Each IE: [ID: 2 bytes] [criticality: 1 byte] [length: 2 bytes] [value]
	pdu := buildTestNGAPMessage(
		0x00,                           // initiatingMessage
		ProcedureCodeInitialUEMessage, // procedureCode 15
		[]testIE{
			{id: IEID_RAN_UE_NGAP_ID, value: []byte{0x00, 0x00, 0x00, 0x2A}}, // RAN-UE-NGAP-ID = 42
			{id: IEID_NAS_PDU, value: []byte{0x7E, 0x00, 0x41}},               // NAS PDU (Registration Request header)
			{id: IEID_UserLocationInfo, value: []byte{0x01, 0x02, 0x03, 0x04}}, // User Location Info (raw)
		},
	)

	ctx, err := ParseNGAPMessage(pdu)
	if err != nil {
		t.Fatalf("ParseNGAPMessage error: %v", err)
	}

	if ctx.Route.FunctionName != "amf-initial-registration" {
		t.Errorf("FunctionName = %q, want amf-initial-registration", ctx.Route.FunctionName)
	}
	if ctx.RANUeNgapID != 42 {
		t.Errorf("RANUeNgapID = %d, want 42", ctx.RANUeNgapID)
	}
	if len(ctx.NASPDU) != 3 {
		t.Errorf("NAS-PDU len = %d, want 3", len(ctx.NASPDU))
	}
	if ctx.NASPDU[0] != 0x7E {
		t.Errorf("NAS-PDU[0] = 0x%02x, want 0x7E", ctx.NASPDU[0])
	}
	if len(ctx.UserLocationInfo) != 4 {
		t.Errorf("UserLocationInfo len = %d, want 4", len(ctx.UserLocationInfo))
	}
}

func TestParseNGAPMessage_UplinkNASTransport(t *testing.T) {
	pdu := buildTestNGAPMessage(
		0x00,                              // initiatingMessage
		ProcedureCodeUplinkNASTransport, // procedureCode 46
		[]testIE{
			{id: IEID_AMF_UE_NGAP_ID, value: []byte{0x00, 0x00, 0x00, 0x05}}, // AMF-UE-NGAP-ID = 5
			{id: IEID_RAN_UE_NGAP_ID, value: []byte{0x00, 0x00, 0x00, 0x03}}, // RAN-UE-NGAP-ID = 3
			{id: IEID_NAS_PDU, value: []byte{0x7E, 0x00, 0x5E}},               // NAS PDU (SMC Complete)
		},
	)

	ctx, err := ParseNGAPMessage(pdu)
	if err != nil {
		t.Fatalf("ParseNGAPMessage error: %v", err)
	}

	if ctx.AMFUeNgapID != 5 {
		t.Errorf("AMFUeNgapID = %d, want 5", ctx.AMFUeNgapID)
	}
	if ctx.RANUeNgapID != 3 {
		t.Errorf("RANUeNgapID = %d, want 3", ctx.RANUeNgapID)
	}
	if len(ctx.NASPDU) != 3 {
		t.Errorf("NAS-PDU len = %d, want 3", len(ctx.NASPDU))
	}
}

func TestParseNGAPMessage_NoIEs(t *testing.T) {
	// Minimal NGAP message with no IEs
	pdu := buildTestNGAPMessage(
		0x00,
		ProcedureCodeNGSetup,
		nil, // no IEs
	)

	ctx, err := ParseNGAPMessage(pdu)
	if err != nil {
		t.Fatalf("ParseNGAPMessage error: %v", err)
	}
	if ctx.RANUeNgapID != 0 {
		t.Errorf("RANUeNgapID = %d, want 0 (unset)", ctx.RANUeNgapID)
	}
	if ctx.NASPDU != nil {
		t.Errorf("NAS-PDU should be nil, got %v", ctx.NASPDU)
	}
}

func TestParseNGAPMessage_TooShort(t *testing.T) {
	_, err := ParseNGAPMessage([]byte{0x00, 0x0f})
	if err == nil {
		t.Fatal("expected error for too-short message")
	}
}

func TestBuildDownlinkNASTransport(t *testing.T) {
	nasPDU := []byte{0x7E, 0x00, 0x42, 0x01} // Registration Accept header
	pdu := BuildDownlinkNASTransport(100, 42, nasPDU)

	if len(pdu) == 0 {
		t.Fatal("BuildDownlinkNASTransport returned empty PDU")
	}

	// Parse the result and verify IEs
	ctx, err := ParseNGAPMessage(pdu)
	if err != nil {
		t.Fatalf("parsing built message: %v", err)
	}
	if ctx.AMFUeNgapID != 100 {
		t.Errorf("AMF-UE-NGAP-ID = %d, want 100", ctx.AMFUeNgapID)
	}
	if ctx.RANUeNgapID != 42 {
		t.Errorf("RAN-UE-NGAP-ID = %d, want 42", ctx.RANUeNgapID)
	}
	if len(ctx.NASPDU) != len(nasPDU) {
		t.Errorf("NAS-PDU len = %d, want %d", len(ctx.NASPDU), len(nasPDU))
	}
}

// --- Test helpers ---

type testIE struct {
	id    int
	value []byte
}

// buildTestNGAPMessage constructs a simplified NGAP PDU for testing.
// Format: [msgType(1)][procedureCode(1)][criticality(1)][numIEs(2)][IEs...]
// Each IE: [id(2)][criticality(1)][length(2)][value...]
func buildTestNGAPMessage(msgTypeByte byte, procedureCode int, ies []testIE) []byte {
	header := []byte{
		msgTypeByte,          // NGAP PDU choice byte
		byte(procedureCode), // procedure code
		0x00,                 // criticality: reject
	}

	// Number of IEs (2 bytes big-endian)
	numIEs := len(ies)
	header = append(header, byte(numIEs>>8), byte(numIEs&0xFF))

	for _, ie := range ies {
		// IE ID (2 bytes big-endian)
		header = append(header, byte(ie.id>>8), byte(ie.id&0xFF))
		// Criticality
		header = append(header, 0x00)
		// Value length (2 bytes big-endian)
		vLen := len(ie.value)
		header = append(header, byte(vLen>>8), byte(vLen&0xFF))
		// Value
		header = append(header, ie.value...)
	}

	return header
}
