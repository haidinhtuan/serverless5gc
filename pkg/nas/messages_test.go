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
