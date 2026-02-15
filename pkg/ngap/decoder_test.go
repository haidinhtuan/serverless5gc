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
