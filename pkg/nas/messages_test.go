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
