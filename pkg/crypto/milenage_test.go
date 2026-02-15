package crypto

import (
	"encoding/hex"
	"testing"
)

// TestGenerateAuthVectorWithRAND_TestSet1 uses 3GPP TS 35.208 Test Set 1 values.
func TestGenerateAuthVectorWithRAND_TestSet1(t *testing.T) {
	k, _ := hex.DecodeString("465b5ce8b199b49faa5f0a2ee238a6bc")
	opc, _ := hex.DecodeString("cd63cb71954a9f4e48a5994e37a02baf")
	sqn, _ := hex.DecodeString("ff9bb4d0b607")
	amf, _ := hex.DecodeString("b9b9")
	randVal, _ := hex.DecodeString("23553cbe9637a89d218ae64dae47bf35")
	snn := "5G:mnc001.mcc001.3gppnetwork.org"

	av, err := GenerateAuthVectorWithRAND(k, opc, sqn, amf, randVal, snn)
	if err != nil {
		t.Fatalf("GenerateAuthVectorWithRAND failed: %v", err)
	}

	// Verify RAND is passed through
	if hex.EncodeToString(av.RAND) != "23553cbe9637a89d218ae64dae47bf35" {
		t.Errorf("RAND mismatch: got %s", hex.EncodeToString(av.RAND))
	}

	// Verify AUTN is correctly generated
	// From free5gc milenage test: AUTN = (SQN⊕AK)||AMF||MAC-A
	// AK = aa689c648370, SQN = ff9bb4d0b607, AMF = b9b9
	// SQN⊕AK = 55f328b43577
	// MAC-A = 4a9ffac354dfafb3
	// AUTN = 55f328b43577 || b9b9 || 4a9ffac354dfafb3
	expectedAUTN := "55f328b43577b9b94a9ffac354dfafb3"
	if hex.EncodeToString(av.AUTN) != expectedAUTN {
		t.Errorf("AUTN mismatch: got %s, want %s", hex.EncodeToString(av.AUTN), expectedAUTN)
	}

	// Verify XRES* is 16 bytes
	if len(av.XRES) != 16 {
		t.Errorf("XRES* length: got %d, want 16", len(av.XRES))
	}

	// Verify KAUSF is 32 bytes
	if len(av.KAUSF) != 32 {
		t.Errorf("KAUSF length: got %d, want 32", len(av.KAUSF))
	}
}

// TestGenerateAuthVectorWithRAND_TestSet19 uses 3GPP TS 35.208 Test Set 19 values.
func TestGenerateAuthVectorWithRAND_TestSet19(t *testing.T) {
	k, _ := hex.DecodeString("5122250214c33e723a5dd523fc145fc0")
	opc, _ := hex.DecodeString("981d464c7c52eb6e5036234984ad0bcf")
	sqn, _ := hex.DecodeString("16f3b3f70fc2")
	amf, _ := hex.DecodeString("c3ab")
	randVal, _ := hex.DecodeString("81e92b6c0ee0e12ebceba8d92a99dfa5")
	snn := "5G:mnc001.mcc001.3gppnetwork.org"

	av, err := GenerateAuthVectorWithRAND(k, opc, sqn, amf, randVal, snn)
	if err != nil {
		t.Fatalf("GenerateAuthVectorWithRAND failed: %v", err)
	}

	// Verify AUTN matches the free5gc test expectation
	expectedAUTN := "bb52e91c747ac3ab2a5c23d15ee351d5"
	if hex.EncodeToString(av.AUTN) != expectedAUTN {
		t.Errorf("AUTN mismatch: got %s, want %s", hex.EncodeToString(av.AUTN), expectedAUTN)
	}

	if len(av.XRES) != 16 {
		t.Errorf("XRES* length: got %d, want 16", len(av.XRES))
	}
	if len(av.KAUSF) != 32 {
		t.Errorf("KAUSF length: got %d, want 32", len(av.KAUSF))
	}
}

// TestGenerateAuthVector_RandomRAND verifies that RAND is randomly generated.
func TestGenerateAuthVector_RandomRAND(t *testing.T) {
	k, _ := hex.DecodeString("465b5ce8b199b49faa5f0a2ee238a6bc")
	opc, _ := hex.DecodeString("cd63cb71954a9f4e48a5994e37a02baf")
	sqn, _ := hex.DecodeString("ff9bb4d0b607")
	amf, _ := hex.DecodeString("b9b9")
	snn := "5G:mnc001.mcc001.3gppnetwork.org"

	av1, err := GenerateAuthVector(k, opc, sqn, amf, snn)
	if err != nil {
		t.Fatalf("first GenerateAuthVector failed: %v", err)
	}

	av2, err := GenerateAuthVector(k, opc, sqn, amf, snn)
	if err != nil {
		t.Fatalf("second GenerateAuthVector failed: %v", err)
	}

	if hex.EncodeToString(av1.RAND) == hex.EncodeToString(av2.RAND) {
		t.Error("two calls produced identical RAND values")
	}
}

// TestGenerateAuthVectorWithRAND_InvalidInputs tests error cases.
func TestGenerateAuthVectorWithRAND_InvalidInputs(t *testing.T) {
	snn := "5G:mnc001.mcc001.3gppnetwork.org"

	// Wrong K length
	_, err := GenerateAuthVectorWithRAND(
		[]byte{0x01, 0x02}, // too short
		make([]byte, 16), make([]byte, 6), make([]byte, 2), make([]byte, 16), snn,
	)
	if err == nil {
		t.Error("expected error for invalid K length")
	}

	// Wrong OPc length
	_, err = GenerateAuthVectorWithRAND(
		make([]byte, 16),
		[]byte{0x01}, // too short
		make([]byte, 6), make([]byte, 2), make([]byte, 16), snn,
	)
	if err == nil {
		t.Error("expected error for invalid OPc length")
	}
}
