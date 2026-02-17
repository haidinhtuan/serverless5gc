package nas

import (
	"testing"
)

func TestDecodeRegistrationRequest(t *testing.T) {
	// Build a minimal Registration Request:
	// EPD(0x7E) | SecHdr(0x00) | MsgType(0x41) | RegType+ngKSI
	// | MobileIdentity(len=5, type=SUCI, dummy data)
	data := []byte{
		0x7E,       // EPD: 5GMM
		0x00,       // Security Header: plain
		0x41,       // Message Type: Registration Request
		0x01,       // Registration Type: Initial(1) | ngKSI=0
		0x00, 0x08, // Mobile Identity Length = 8
		0x01,                                     // Type: SUCI
		0x10, 0x10, 0x10,                         // MCC=001, MNC=01 (BCD)
		0x00, 0x00,                               // Routing indicator
		0x00,                                     // Protection scheme
		0x10, 0x32, 0x54, 0x76, 0x98, 0x10, 0xF0, // MSIN digits (BCD) - extra bytes
	}
	// Pad to minimum length
	data = append(data, make([]byte, 4)...)

	req, err := DecodeRegistrationRequest(data[:14])
	if err != nil {
		t.Fatalf("DecodeRegistrationRequest error: %v", err)
	}
	if req.RegistrationType != RegTypeInitialRegistration {
		t.Errorf("RegistrationType = %d, want %d", req.RegistrationType, RegTypeInitialRegistration)
	}
	if req.MobileIdentity.Type != MobileIdentitySUCI {
		t.Errorf("MobileIdentity.Type = %d, want %d", req.MobileIdentity.Type, MobileIdentitySUCI)
	}
	if req.NgKSI != 0 {
		t.Errorf("NgKSI = %d, want 0", req.NgKSI)
	}
}

func TestDecodeRegistrationRequest_InvalidEPD(t *testing.T) {
	data := []byte{0xFF, 0x00, 0x41, 0x01, 0x00, 0x01, 0x01}
	_, err := DecodeRegistrationRequest(data)
	if err == nil {
		t.Fatal("expected error for invalid EPD")
	}
}

func TestDecodeRegistrationRequest_TooShort(t *testing.T) {
	_, err := DecodeRegistrationRequest([]byte{0x7E, 0x00, 0x41})
	if err == nil {
		t.Fatal("expected error for too-short message")
	}
}

func TestDecodeRegistrationRequest_WrongMessageType(t *testing.T) {
	data := []byte{0x7E, 0x00, 0x42, 0x01, 0x00, 0x01, 0x01}
	_, err := DecodeRegistrationRequest(data)
	if err == nil {
		t.Fatal("expected error for wrong message type")
	}
}

func TestEncodeRegistrationAccept(t *testing.T) {
	accept := &RegistrationAccept{
		RegistrationResult: RegResult3GPPAccess,
		GUTI:               "test-guti-001",
		AllowedNSSAI: []NSSAI{
			{SST: 1, HasSD: false},
		},
		T3512Value: T3512Default,
	}

	data := EncodeRegistrationAccept(accept)

	// Verify header
	if data[0] != EPD5GMM {
		t.Errorf("EPD = 0x%02x, want 0x%02x", data[0], EPD5GMM)
	}
	if data[1] != SecurityHeaderPlain {
		t.Errorf("SecurityHeader = 0x%02x, want 0x%02x", data[1], SecurityHeaderPlain)
	}
	if data[2] != MsgTypeRegistrationAccept {
		t.Errorf("MessageType = 0x%02x, want 0x%02x", data[2], MsgTypeRegistrationAccept)
	}
	if data[3] != RegResult3GPPAccess {
		t.Errorf("RegistrationResult = 0x%02x, want 0x%02x", data[3], RegResult3GPPAccess)
	}

	// Verify GUTI IE is present (tag 0x77)
	found := false
	for i := 4; i < len(data); i++ {
		if data[i] == IETag5GGUTI {
			found = true
			break
		}
	}
	if !found {
		t.Error("5G-GUTI IE not found in encoded message")
	}
}

func TestEncodeRegistrationReject(t *testing.T) {
	reject := &RegistrationReject{CauseCode: CauseIllegalUE}
	data := EncodeRegistrationReject(reject)

	if len(data) != 4 {
		t.Fatalf("length = %d, want 4", len(data))
	}
	if data[0] != EPD5GMM {
		t.Errorf("EPD = 0x%02x, want 0x%02x", data[0], EPD5GMM)
	}
	if data[2] != MsgTypeRegistrationReject {
		t.Errorf("MessageType = 0x%02x, want 0x%02x", data[2], MsgTypeRegistrationReject)
	}
	if data[3] != CauseIllegalUE {
		t.Errorf("CauseCode = %d, want %d", data[3], CauseIllegalUE)
	}
}

func TestEncodeSecurityModeCommand(t *testing.T) {
	smc := &SecurityModeCommand{
		SelectedCiphering: CipherAlg5GEA2,
		SelectedIntegrity: IntegAlg5GIA2,
		NgKSI:             1,
		ReplayedUESecCap:  &UESecurityCapability{EA0: true, EA1: true, EA2: true, IA0: true, IA1: true, IA2: true},
	}

	data := EncodeSecurityModeCommand(smc)

	if data[0] != EPD5GMM {
		t.Errorf("EPD = 0x%02x, want 0x%02x", data[0], EPD5GMM)
	}
	if data[2] != MsgTypeSecurityModeCommand {
		t.Errorf("MessageType = 0x%02x, want 0x%02x", data[2], MsgTypeSecurityModeCommand)
	}
	// Security algorithms byte: ciphering(upper 4) | integrity(lower 4)
	expectedAlg := byte((CipherAlg5GEA2 << 4) | IntegAlg5GIA2)
	if data[3] != expectedAlg {
		t.Errorf("algorithms = 0x%02x, want 0x%02x", data[3], expectedAlg)
	}
	if data[4] != 1 {
		t.Errorf("NgKSI = %d, want 1", data[4])
	}
}

func TestEncodeGPRSTimer3(t *testing.T) {
	tests := []struct {
		seconds uint32
		want    byte
	}{
		{30, 0x8F & 0xFF},  // Some value in 30s range
		{T3512Default, 0},  // 3240s = 54 min = 5.4 * 10min units
	}

	for _, tt := range tests {
		got := encodeGPRSTimer3(tt.seconds)
		// Just verify it doesn't panic and returns a byte
		if got == 0 && tt.seconds > 0 && tt.seconds < 62 {
			t.Errorf("encodeGPRSTimer3(%d) = 0, unexpected", tt.seconds)
		}
	}

	// Verify specific known values
	// 30 seconds = 15 units of 2s → 0x60 | 15 = 0x6F
	if got := encodeGPRSTimer3(30); got != 0x6F {
		t.Errorf("encodeGPRSTimer3(30) = 0x%02x, want 0x6F", got)
	}

	// 54 minutes = 3240s = 5.4 → 5 units of 10min → 0x00 | 5 = 0x05
	if got := encodeGPRSTimer3(3240); got != 0x05 {
		t.Errorf("encodeGPRSTimer3(3240) = 0x%02x, want 0x05", got)
	}
}

func TestDecodeRegistrationRequest_WithUESecCap(t *testing.T) {
	// Build Registration Request with UE Security Capability optional IE
	data := []byte{
		0x7E,       // EPD: 5GMM
		0x00,       // Security Header: plain
		0x41,       // Message Type: Registration Request
		0x01,       // Registration Type: Initial(1) | ngKSI=0
		0x00, 0x08, // Mobile Identity Length = 8
		0x01,                   // Type: SUCI
		0x10, 0x10, 0x10,     // MCC/MNC
		0x00, 0x00,            // Routing indicator
		0x00,                  // Protection scheme
		0x10,                  // MSIN
		// Optional IE: UE Security Capability (Tag 0x2E)
		0x2E,       // Tag
		0x02,       // Length = 2
		0xE0,       // EA0=1, EA1=1, EA2=1, EA3=0
		0xE0,       // IA0=1, IA1=1, IA2=1, IA3=0
	}

	req, err := DecodeRegistrationRequest(data)
	if err != nil {
		t.Fatalf("DecodeRegistrationRequest error: %v", err)
	}
	if req.UESecCap == nil {
		t.Fatal("UESecCap is nil, expected parsed value")
	}
	if !req.UESecCap.EA0 || !req.UESecCap.EA1 || !req.UESecCap.EA2 {
		t.Errorf("EA0/EA1/EA2 should be true, got %+v", req.UESecCap)
	}
	if req.UESecCap.EA3 {
		t.Error("EA3 should be false")
	}
	if !req.UESecCap.IA0 || !req.UESecCap.IA1 || !req.UESecCap.IA2 {
		t.Errorf("IA0/IA1/IA2 should be true, got %+v", req.UESecCap)
	}
}

func TestDecodeRegistrationRequest_WithRequestedNSSAI(t *testing.T) {
	// Build Registration Request with Requested NSSAI optional IE
	data := []byte{
		0x7E,       // EPD: 5GMM
		0x00,       // Security Header: plain
		0x41,       // Message Type: Registration Request
		0x01,       // Registration Type: Initial(1) | ngKSI=0
		0x00, 0x08, // Mobile Identity Length = 8
		0x01,                   // Type: SUCI
		0x10, 0x10, 0x10,     // MCC/MNC
		0x00, 0x00,            // Routing indicator
		0x00,                  // Protection scheme
		0x10,                  // MSIN
		// Optional IE: Requested NSSAI (Tag 0x15)
		0x15,       // Tag
		0x07,       // Length = 7 (one SST-only: 2 bytes, one SST+SD: 5 bytes)
		0x01, 0x01, // S-NSSAI: len=1, SST=1
		0x04, 0x02, 0x01, 0x02, 0x03, // S-NSSAI: len=4, SST=2, SD=010203
	}

	req, err := DecodeRegistrationRequest(data)
	if err != nil {
		t.Fatalf("DecodeRegistrationRequest error: %v", err)
	}
	if len(req.RequestedNSSAI) != 2 {
		t.Fatalf("RequestedNSSAI len = %d, want 2", len(req.RequestedNSSAI))
	}
	if req.RequestedNSSAI[0].SST != 1 {
		t.Errorf("RequestedNSSAI[0].SST = %d, want 1", req.RequestedNSSAI[0].SST)
	}
	if req.RequestedNSSAI[0].HasSD {
		t.Error("RequestedNSSAI[0].HasSD should be false")
	}
	if req.RequestedNSSAI[1].SST != 2 {
		t.Errorf("RequestedNSSAI[1].SST = %d, want 2", req.RequestedNSSAI[1].SST)
	}
	if !req.RequestedNSSAI[1].HasSD {
		t.Error("RequestedNSSAI[1].HasSD should be true")
	}
	if req.RequestedNSSAI[1].SD != [3]byte{0x01, 0x02, 0x03} {
		t.Errorf("RequestedNSSAI[1].SD = %v, want [1 2 3]", req.RequestedNSSAI[1].SD)
	}
}

func TestDecodeSecurityModeComplete(t *testing.T) {
	// Build Security Mode Complete:
	// EPD(0x7E) | SecHdr(0x04) | MsgType(0x5E)
	data := []byte{
		0x7E,       // EPD: 5GMM
		0x04,       // Security Header: integrity protected with new context
		0x5E,       // Message Type: Security Mode Complete
	}

	smc, err := DecodeSecurityModeComplete(data)
	if err != nil {
		t.Fatalf("DecodeSecurityModeComplete error: %v", err)
	}
	if smc == nil {
		t.Fatal("result is nil")
	}
}

func TestDecodeSecurityModeComplete_WithIMEISV(t *testing.T) {
	// Security Mode Complete with IMEISV optional IE (tag 0x77)
	data := []byte{
		0x7E,       // EPD: 5GMM
		0x04,       // Security Header
		0x5E,       // Message Type: Security Mode Complete
		// Optional IE: IMEISV (tag 0x77)
		0x77,       // Tag
		0x09,       // Length = 9
		0x05,       // Type: IMEISV
		0x98, 0x76, 0x54, 0x32, 0x10, 0x98, 0x76, 0x50, // IMEISV BCD digits
	}

	smc, err := DecodeSecurityModeComplete(data)
	if err != nil {
		t.Fatalf("DecodeSecurityModeComplete error: %v", err)
	}
	if smc.IMEISV == "" {
		t.Error("IMEISV should not be empty when IE is present")
	}
}

func TestDecodeSecurityModeComplete_InvalidEPD(t *testing.T) {
	data := []byte{0xFF, 0x04, 0x5E}
	_, err := DecodeSecurityModeComplete(data)
	if err == nil {
		t.Fatal("expected error for invalid EPD")
	}
}

func TestDecodeSecurityModeComplete_WrongMsgType(t *testing.T) {
	data := []byte{0x7E, 0x04, 0x5D} // 0x5D = Security Mode Command, not Complete
	_, err := DecodeSecurityModeComplete(data)
	if err == nil {
		t.Fatal("expected error for wrong message type")
	}
}

func TestDecodeSecurityModeComplete_TooShort(t *testing.T) {
	_, err := DecodeSecurityModeComplete([]byte{0x7E, 0x04})
	if err == nil {
		t.Fatal("expected error for too-short message")
	}
}

func TestEncodeRegistrationAccept_AllIEs(t *testing.T) {
	accept := &RegistrationAccept{
		RegistrationResult: RegResult3GPPAccess,
		GUTI:               "5g-guti-test-001",
		AllowedNSSAI: []NSSAI{
			{SST: 1, HasSD: false},
			{SST: 2, SD: [3]byte{0x01, 0x02, 0x03}, HasSD: true},
		},
		T3512Value: T3512Default,
	}

	data := EncodeRegistrationAccept(accept)

	// Verify all IEs are present
	hasGUTI, hasNSSAI, hasT3512 := false, false, false
	for i := 4; i < len(data); i++ {
		switch data[i] {
		case IETag5GGUTI:
			hasGUTI = true
		case IETagAllowedNSSAI:
			hasNSSAI = true
		case IETagT3512Value:
			hasT3512 = true
		}
	}
	if !hasGUTI {
		t.Error("missing 5G-GUTI IE")
	}
	if !hasNSSAI {
		t.Error("missing Allowed NSSAI IE")
	}
	if !hasT3512 {
		t.Error("missing T3512 timer IE")
	}
}

func TestEncodeRegistrationAccept_NoOptionalIEs(t *testing.T) {
	accept := &RegistrationAccept{
		RegistrationResult: RegResult3GPPAccess,
	}

	data := EncodeRegistrationAccept(accept)
	// Should be exactly 4 bytes: EPD + SecHdr + MsgType + RegResult
	if len(data) != 4 {
		t.Errorf("length = %d, want 4 (header only)", len(data))
	}
}

func TestDecodeEncodeNSSAI(t *testing.T) {
	original := []NSSAI{
		{SST: 1, HasSD: false},
		{SST: 2, SD: [3]byte{0x01, 0x02, 0x03}, HasSD: true},
	}

	encoded := encodeNSSAIList(original)
	decoded := decodeNSSAIList(encoded)

	if len(decoded) != len(original) {
		t.Fatalf("decoded len = %d, want %d", len(decoded), len(original))
	}
	if decoded[0].SST != 1 {
		t.Errorf("decoded[0].SST = %d, want 1", decoded[0].SST)
	}
	if decoded[1].SST != 2 {
		t.Errorf("decoded[1].SST = %d, want 2", decoded[1].SST)
	}
	if !decoded[1].HasSD {
		t.Error("decoded[1].HasSD = false, want true")
	}
	if decoded[1].SD != [3]byte{0x01, 0x02, 0x03} {
		t.Errorf("decoded[1].SD = %v, want [1 2 3]", decoded[1].SD)
	}
}

func TestEncodeAuthenticationRequest(t *testing.T) {
	rand := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
	autn := []byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F, 0x20}

	data := EncodeAuthenticationRequest(rand, autn)

	// Total: 3(header) + 1(ngKSI) + 4(ABBA) + 1(RAND IEI) + 16(RAND) + 1(AUTN IEI) + 1(AUTN len) + 16(AUTN) = 43
	if len(data) != 43 {
		t.Fatalf("total length = %d, want 43", len(data))
	}
	if data[0] != EPD5GMM {
		t.Errorf("EPD = 0x%02x, want 0x%02x", data[0], EPD5GMM)
	}
	if data[1] != SecurityHeaderPlain {
		t.Errorf("SecurityHeader = 0x%02x, want 0x%02x", data[1], SecurityHeaderPlain)
	}
	if data[2] != MsgTypeAuthenticationRequest {
		t.Errorf("MessageType = 0x%02x, want 0x%02x", data[2], MsgTypeAuthenticationRequest)
	}
	if data[3] != 0x00 {
		t.Errorf("ngKSI = 0x%02x, want 0x00", data[3])
	}
	// ABBA: length=0x0002, value=0x0000
	if data[4] != 0x00 || data[5] != 0x02 || data[6] != 0x00 || data[7] != 0x00 {
		t.Errorf("ABBA = %v, want [00 02 00 00]", data[4:8])
	}
	// RAND: IEI 0x21 + 16 bytes
	if data[8] != 0x21 {
		t.Errorf("RAND IEI = 0x%02x, want 0x21", data[8])
	}
	for i := 0; i < 16; i++ {
		if data[9+i] != rand[i] {
			t.Errorf("RAND[%d] = 0x%02x, want 0x%02x", i, data[9+i], rand[i])
		}
	}
	// AUTN: IEI 0x20 + length 0x10 + 16 bytes
	if data[25] != 0x20 {
		t.Errorf("AUTN IEI = 0x%02x, want 0x20", data[25])
	}
	if data[26] != 0x10 {
		t.Errorf("AUTN length = 0x%02x, want 0x10", data[26])
	}
	for i := 0; i < 16; i++ {
		if data[27+i] != autn[i] {
			t.Errorf("AUTN[%d] = 0x%02x, want 0x%02x", i, data[27+i], autn[i])
		}
	}
}

func TestDecodeAuthenticationResponse(t *testing.T) {
	res := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22,
		0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0x00}
	data := []byte{
		0x7E,       // EPD
		0x00,       // Security header
		0x57,       // Message type: Authentication Response
		0x00, 0x10, // Length = 16
	}
	data = append(data, res...)

	got, err := DecodeAuthenticationResponse(data)
	if err != nil {
		t.Fatalf("DecodeAuthenticationResponse error: %v", err)
	}
	if len(got) != 16 {
		t.Fatalf("RES* length = %d, want 16", len(got))
	}
	for i := 0; i < 16; i++ {
		if got[i] != res[i] {
			t.Errorf("RES*[%d] = 0x%02x, want 0x%02x", i, got[i], res[i])
		}
	}
}

func TestDecodeAuthenticationResponse_TooShort(t *testing.T) {
	_, err := DecodeAuthenticationResponse([]byte{0x7E, 0x00, 0x57})
	if err == nil {
		t.Fatal("expected error for too-short message")
	}
}

func TestDecodeAuthenticationResponse_InvalidEPD(t *testing.T) {
	data := []byte{0xFF, 0x00, 0x57, 0x00, 0x01, 0xAA}
	_, err := DecodeAuthenticationResponse(data)
	if err == nil {
		t.Fatal("expected error for invalid EPD")
	}
}

func TestDecodeAuthenticationResponse_WrongMsgType(t *testing.T) {
	data := []byte{0x7E, 0x00, 0x56, 0x00, 0x01, 0xAA}
	_, err := DecodeAuthenticationResponse(data)
	if err == nil {
		t.Fatal("expected error for wrong message type")
	}
}

func TestDecodePDUSessionEstablishmentRequest(t *testing.T) {
	data := []byte{
		0x2E, // EPD: 5GSM
		0x05, // PDU session ID = 5
		0x01, // PTI
		0xC1, // Message type: PDU Session Establishment Request
		0x00, 0x00, // Integrity protection max data rate
		0x91, // PDU session type: IPv4 (IEI 0x09, value 0x01)
	}

	sessID, sessType, err := DecodePDUSessionEstablishmentRequest(data)
	if err != nil {
		t.Fatalf("DecodePDUSessionEstablishmentRequest error: %v", err)
	}
	if sessID != 5 {
		t.Errorf("pduSessionID = %d, want 5", sessID)
	}
	if sessType != 0x01 {
		t.Errorf("pduSessionType = 0x%02x, want 0x01", sessType)
	}
}

func TestDecodePDUSessionEstablishmentRequest_DefaultType(t *testing.T) {
	data := []byte{
		0x2E, // EPD: 5GSM
		0x01, // PDU session ID = 1
		0x00, // PTI
		0xC1, // Message type
		0x00, 0x00, // Integrity protection max data rate
	}

	sessID, sessType, err := DecodePDUSessionEstablishmentRequest(data)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if sessID != 1 {
		t.Errorf("pduSessionID = %d, want 1", sessID)
	}
	if sessType != 0x01 { // default IPv4
		t.Errorf("pduSessionType = 0x%02x, want 0x01 (default IPv4)", sessType)
	}
}

func TestDecodePDUSessionEstablishmentRequest_InvalidEPD(t *testing.T) {
	data := []byte{0xFF, 0x01, 0x00, 0xC1, 0x00, 0x00}
	_, _, err := DecodePDUSessionEstablishmentRequest(data)
	if err == nil {
		t.Fatal("expected error for invalid EPD")
	}
}

func TestDecodePDUSessionEstablishmentRequest_TooShort(t *testing.T) {
	_, _, err := DecodePDUSessionEstablishmentRequest([]byte{0x2E, 0x01, 0x00})
	if err == nil {
		t.Fatal("expected error for too-short message")
	}
}

func TestDecodeULNASTransport(t *testing.T) {
	// Inner PDU Session Establishment Request
	innerPDU := []byte{0x2E, 0x05, 0x01, 0xC1, 0x00, 0x00, 0x91}

	data := []byte{
		0x7E,       // EPD
		0x00,       // Security header
		0x67,       // Message type: UL NAS Transport
		0x01,       // Payload container type = 1 (N1 SM)
	}
	// Payload container length (2 bytes, big-endian)
	data = append(data, byte(len(innerPDU)>>8), byte(len(innerPDU)))
	data = append(data, innerPDU...)
	// PDU session ID (IEI 0x12 + value)
	data = append(data, 0x12, 0x05)
	// Request type (IEI 0x08, value 0x01 = initial)
	data = append(data, 0x81)
	// S-NSSAI (IEI 0x22, TLV)
	data = append(data, 0x22, 0x04, 0x01, 0xAA, 0xBB, 0xCC)
	// DNN (IEI 0x25, TLV) = "internet"
	dnnLabel := []byte("internet")
	data = append(data, 0x25, byte(1+len(dnnLabel)), byte(len(dnnLabel)))
	data = append(data, dnnLabel...)

	pctType, sessID, payload, dnn, snssai, err := DecodeULNASTransport(data)
	if err != nil {
		t.Fatalf("DecodeULNASTransport error: %v", err)
	}
	if pctType != 0x01 {
		t.Errorf("payloadContainerType = 0x%02x, want 0x01", pctType)
	}
	if sessID != 0x05 {
		t.Errorf("pduSessionID = %d, want 5", sessID)
	}
	if len(payload) != len(innerPDU) {
		t.Errorf("payload length = %d, want %d", len(payload), len(innerPDU))
	}
	if dnn != "internet" {
		t.Errorf("dnn = %q, want %q", dnn, "internet")
	}
	if snssai == nil {
		t.Fatal("snssai is nil")
	}
	if snssai.SST != 0x01 {
		t.Errorf("snssai.SST = %d, want 1", snssai.SST)
	}
	if !snssai.HasSD {
		t.Error("snssai.HasSD should be true")
	}
	if snssai.SD != [3]byte{0xAA, 0xBB, 0xCC} {
		t.Errorf("snssai.SD = %v, want [AA BB CC]", snssai.SD)
	}
}

func TestDecodeULNASTransport_TooShort(t *testing.T) {
	_, _, _, _, _, err := DecodeULNASTransport([]byte{0x7E, 0x00, 0x67})
	if err == nil {
		t.Fatal("expected error for too-short message")
	}
}

func TestDecodeULNASTransport_InvalidEPD(t *testing.T) {
	data := []byte{0xFF, 0x00, 0x67, 0x01, 0x00, 0x01, 0xAA}
	_, _, _, _, _, err := DecodeULNASTransport(data)
	if err == nil {
		t.Fatal("expected error for invalid EPD")
	}
}

func TestDecodeULNASTransport_WrongMsgType(t *testing.T) {
	data := []byte{0x7E, 0x00, 0x41, 0x01, 0x00, 0x01, 0xAA}
	_, _, _, _, _, err := DecodeULNASTransport(data)
	if err == nil {
		t.Fatal("expected error for wrong message type")
	}
}

func TestDecodeDNN(t *testing.T) {
	// "internet" = 0x08 + "internet"
	data := []byte{0x08, 'i', 'n', 't', 'e', 'r', 'n', 'e', 't'}
	got := decodeDNN(data)
	if got != "internet" {
		t.Errorf("decodeDNN = %q, want %q", got, "internet")
	}

	// "example.com" = 0x07 "example" 0x03 "com"
	data = []byte{0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 0x03, 'c', 'o', 'm'}
	got = decodeDNN(data)
	if got != "example.com" {
		t.Errorf("decodeDNN = %q, want %q", got, "example.com")
	}
}
