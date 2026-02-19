package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"

	"github.com/free5gc/util/ueauth"
)

// DeriveKSEAF derives KSEAF from KAUSF per TS 33.501 Annex A.6.
// FC = 0x6C, P0 = serving network name (as bytes), L0 = len(SNN).
func DeriveKSEAF(kausf []byte, snn string) ([]byte, error) {
	snnBytes := []byte(snn)
	kseaf, err := ueauth.GetKDFValue(
		kausf,
		ueauth.FC_FOR_KSEAF_DERIVATION,
		snnBytes, ueauth.KDFLen(snnBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("derive KSEAF: %w", err)
	}
	return kseaf, nil
}

// DeriveKAMF derives KAMF from KSEAF per TS 33.501 Annex A.7.
// FC = 0x6D, P0 = SUPI (as bytes), L0 = len(SUPI), P1 = ABBA, L1 = len(ABBA).
func DeriveKAMF(kseaf []byte, supi string, abba []byte) ([]byte, error) {
	supiBytes := []byte(supi)
	kamf, err := ueauth.GetKDFValue(
		kseaf,
		ueauth.FC_FOR_KAMF_DERIVATION,
		supiBytes, ueauth.KDFLen(supiBytes),
		abba, ueauth.KDFLen(abba),
	)
	if err != nil {
		return nil, fmt.Errorf("derive KAMF: %w", err)
	}
	return kamf, nil
}

// DeriveKNASKeys derives KNASenc and KNASint from KAMF per TS 33.501 Annex A.8.
// FC = 0x69, P0 = algorithm type distinguisher (1 byte), L0 = 0x0001,
// P1 = algorithm identity (1 byte), L1 = 0x0001.
// Algorithm type: 0x01 = NAS-enc, 0x02 = NAS-int.
func DeriveKNASKeys(kamf []byte, cipherAlgID, integAlgID uint8) (knasEnc, knasInt []byte, err error) {
	// Derive KNASenc (algorithm type = 0x01)
	algTypeEnc := []byte{0x01}
	algIDEnc := []byte{cipherAlgID}
	knasEnc, err = ueauth.GetKDFValue(
		kamf,
		ueauth.FC_FOR_ALGORITHM_KEY_DERIVATION,
		algTypeEnc, ueauth.KDFLen(algTypeEnc),
		algIDEnc, ueauth.KDFLen(algIDEnc),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("derive KNASenc: %w", err)
	}
	// Use last 16 bytes of 32-byte HMAC-SHA256 output
	knasEnc = knasEnc[len(knasEnc)-16:]

	// Derive KNASint (algorithm type = 0x02)
	algTypeInt := []byte{0x02}
	algIDInt := []byte{integAlgID}
	knasInt, err = ueauth.GetKDFValue(
		kamf,
		ueauth.FC_FOR_ALGORITHM_KEY_DERIVATION,
		algTypeInt, ueauth.KDFLen(algTypeInt),
		algIDInt, ueauth.KDFLen(algIDInt),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("derive KNASint: %w", err)
	}
	// Use last 16 bytes of 32-byte HMAC-SHA256 output
	knasInt = knasInt[len(knasInt)-16:]

	return knasEnc, knasInt, nil
}

// ComputeNASMAC computes the NAS MAC using 128-EIA2 (AES-CMAC) per TS 33.401 Annex B.2.3.
// The input to AES-CMAC is: COUNT(4 bytes) || BEARER(5 bits) || DIRECTION(1 bit) || 0(26 bits) || message.
// Returns the first 4 bytes of the AES-CMAC output as the MAC.
func ComputeNASMAC(knasInt []byte, count uint32, bearer uint8, direction uint8, message []byte) ([]byte, error) {
	if len(knasInt) != 16 {
		return nil, fmt.Errorf("KNASint must be 16 bytes, got %d", len(knasInt))
	}

	// Build the input: COUNT(4) || BEARER(5 bits) || DIRECTION(1 bit) || zeros(26 bits) || message
	// The 4-byte field after COUNT: BEARER in bits 7-3, DIRECTION in bit 2, zeros in bits 1-0 and next 3 bytes
	input := make([]byte, 8+len(message))
	binary.BigEndian.PutUint32(input[0:4], count)
	input[4] = (bearer << 3) | (direction << 2)
	// input[5], input[6], input[7] are already zero
	copy(input[8:], message)

	mac, err := aesCMAC(knasInt, input)
	if err != nil {
		return nil, fmt.Errorf("AES-CMAC: %w", err)
	}

	return mac[:4], nil
}

// aesCMAC implements AES-CMAC (RFC 4493) and returns the full 16-byte tag.
func aesCMAC(key, message []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Step 1: Generate subkeys K1, K2
	k1, k2 := generateSubkeys(block)

	// Step 2: Determine number of blocks
	n := (len(message) + aes.BlockSize - 1) / aes.BlockSize
	if n == 0 {
		n = 1
	}

	// Step 3: Check if the last block is complete
	lastBlockComplete := (len(message) > 0) && (len(message)%aes.BlockSize == 0)

	// Step 4: Prepare the last block
	lastBlock := make([]byte, aes.BlockSize)
	if lastBlockComplete {
		// XOR last block of message with K1
		start := (n - 1) * aes.BlockSize
		for i := 0; i < aes.BlockSize; i++ {
			lastBlock[i] = message[start+i] ^ k1[i]
		}
	} else {
		// Pad last block and XOR with K2
		start := (n - 1) * aes.BlockSize
		remaining := len(message) - start
		var padded [aes.BlockSize]byte
		copy(padded[:], message[start:start+remaining])
		padded[remaining] = 0x80
		for i := 0; i < aes.BlockSize; i++ {
			lastBlock[i] = padded[i] ^ k2[i]
		}
	}

	// Step 5: CBC-MAC
	x := make([]byte, aes.BlockSize) // zero IV
	for i := 0; i < n-1; i++ {
		start := i * aes.BlockSize
		// XOR message block with X
		for j := 0; j < aes.BlockSize; j++ {
			x[j] ^= message[start+j]
		}
		// Encrypt
		block.Encrypt(x, x)
	}

	// Process the last (prepared) block
	for j := 0; j < aes.BlockSize; j++ {
		x[j] ^= lastBlock[j]
	}
	block.Encrypt(x, x)

	return x, nil
}

// generateSubkeys derives K1 and K2 from the AES cipher block per RFC 4493 Section 2.3.
func generateSubkeys(block cipher.Block) (k1, k2 []byte) {
	const rb = 0x87 // constant Rb for 128-bit block

	// L = AES-128(key, 0^128)
	l := make([]byte, aes.BlockSize)
	block.Encrypt(l, l)

	// K1 = L << 1; if MSB(L) == 1, K1 ^= Rb
	k1 = shiftLeft(l)
	if l[0]&0x80 != 0 {
		k1[aes.BlockSize-1] ^= rb
	}

	// K2 = K1 << 1; if MSB(K1) == 1, K2 ^= Rb
	k2 = shiftLeft(k1)
	if k1[0]&0x80 != 0 {
		k2[aes.BlockSize-1] ^= rb
	}

	return k1, k2
}

// shiftLeft performs a left shift by 1 bit on a byte slice.
func shiftLeft(data []byte) []byte {
	out := make([]byte, len(data))
	for i := 0; i < len(data)-1; i++ {
		out[i] = (data[i] << 1) | (data[i+1] >> 7)
	}
	out[len(data)-1] = data[len(data)-1] << 1
	return out
}
