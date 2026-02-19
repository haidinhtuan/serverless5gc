package crypto

import (
	"encoding/hex"
	"testing"
)

// TestAESCMAC_RFC4493 verifies our AES-CMAC implementation against RFC 4493 test vectors.
func TestAESCMAC_RFC4493(t *testing.T) {
	key, _ := hex.DecodeString("2b7e151628aed2a6abf7158809cf4f3c")

	tests := []struct {
		name    string
		msg     string
		wantMAC string
	}{
		{
			name:    "Example1_empty",
			msg:     "",
			wantMAC: "bb1d6929e95937287fa37d129b756746",
		},
		{
			name:    "Example2_16bytes",
			msg:     "6bc1bee22e409f96e93d7e117393172a",
			wantMAC: "070a16b46b4d4144f79bdd9dd04a287c",
		},
		{
			name:    "Example3_40bytes",
			msg:     "6bc1bee22e409f96e93d7e117393172aae2d8a571e03ac9c9eb76fac45af8e5130c81c46a35ce411",
			wantMAC: "dfa66747de9ae63030ca32611497c827",
		},
		{
			name:    "Example4_64bytes",
			msg:     "6bc1bee22e409f96e93d7e117393172aae2d8a571e03ac9c9eb76fac45af8e5130c81c46a35ce411e5fbc1191a0a52eff69f2445df4f9b17ad2b417be66c3710",
			wantMAC: "51f0bebf7e3b9d92fc49741779363cfe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, _ := hex.DecodeString(tt.msg)
			wantMAC, _ := hex.DecodeString(tt.wantMAC)
			gotMAC, err := aesCMAC(key, msg)
			if err != nil {
				t.Fatalf("aesCMAC error: %v", err)
			}
			if len(gotMAC) != len(wantMAC) {
				t.Fatalf("MAC length mismatch: got %d, want %d", len(gotMAC), len(wantMAC))
			}
			for i := range wantMAC {
				if gotMAC[i] != wantMAC[i] {
					t.Errorf("MAC mismatch at byte %d: got %x, want %x\nFull got:  %x\nFull want: %x",
						i, gotMAC[i], wantMAC[i], gotMAC, wantMAC)
					break
				}
			}
		})
	}
}

// TestComputeNASMAC_KnownValues tests the full NAS MAC computation with known input.
func TestComputeNASMAC_KnownValues(t *testing.T) {
	// Values from our debug log
	knasInt, _ := hex.DecodeString("8009a29d61aa7de9bb5de95751a0cc9a")
	count := uint32(0)
	bearer := uint8(1)
	direction := uint8(1)
	// MESSAGE = SQN(0x00) || plainNAS(7e005d020004f0f0f0f0)
	message, _ := hex.DecodeString("007e005d020004f0f0f0f0")

	mac, err := ComputeNASMAC(knasInt, count, bearer, direction, message)
	if err != nil {
		t.Fatalf("ComputeNASMAC error: %v", err)
	}
	t.Logf("Computed MAC: %x", mac)
	// We expect 0e902366 based on our proxy log
	expectedMAC, _ := hex.DecodeString("0e902366")
	for i := range expectedMAC {
		if mac[i] != expectedMAC[i] {
			t.Errorf("MAC mismatch: got %x, want %x", mac, expectedMAC)
			break
		}
	}
}
