package crypto

import (
	"encoding/hex"
	"testing"

	f5gSec "github.com/free5gc/nas/security"
)

// TestCompareWithFree5GC compares our MAC computation against free5GC NIA2 using actual proxy values.
func TestCompareWithFree5GC(t *testing.T) {
	// Exact values from proxy debug log
	knasIntHex := "568906f82c8305297f2d58a6c25e6c02"
	knasIntSlice, _ := hex.DecodeString(knasIntHex)
	var knasIntArr [16]byte
	copy(knasIntArr[:], knasIntSlice)

	count := uint32(0)
	bearer := uint8(1)
	direction := uint8(1)
	// MESSAGE = SQN(0x00) || plainNAS(7e005d020004f0f0f0f0)
	message, _ := hex.DecodeString("007e005d020004f0f0f0f0")

	// Our implementation
	ourMAC, err := ComputeNASMAC(knasIntSlice, count, bearer, direction, message)
	if err != nil {
		t.Fatalf("our ComputeNASMAC: %v", err)
	}
	t.Logf("Our MAC:     %x", ourMAC)

	// free5GC NIA2 implementation
	f5gMAC, err := f5gSec.NIA2(knasIntArr, count, bearer, direction, message)
	if err != nil {
		t.Fatalf("free5GC NIA2: %v", err)
	}
	t.Logf("free5GC MAC: %x", f5gMAC)

	if hex.EncodeToString(ourMAC) != hex.EncodeToString(f5gMAC) {
		t.Errorf("MAC MISMATCH! ours=%x free5gc=%x", ourMAC, f5gMAC)
	} else {
		t.Logf("MACs match: %x", ourMAC)
	}

	// Also test with direction=0 (in case UERANSIM uses 0 for DL verification)
	ourMAC0, _ := ComputeNASMAC(knasIntSlice, count, bearer, 0, message)
	f5gMAC0, _ := f5gSec.NIA2(knasIntArr, count, bearer, 0, message)
	t.Logf("Direction=0: ours=%x free5gc=%x match=%v", ourMAC0, f5gMAC0, hex.EncodeToString(ourMAC0) == hex.EncodeToString(f5gMAC0))

	// Test with bearer=0
	ourMACb0, _ := ComputeNASMAC(knasIntSlice, count, 0, direction, message)
	f5gMACb0, _ := f5gSec.NIA2(knasIntArr, count, 0, direction, message)
	t.Logf("Bearer=0: ours=%x free5gc=%x match=%v", ourMACb0, f5gMACb0, hex.EncodeToString(ourMACb0) == hex.EncodeToString(f5gMACb0))
}
