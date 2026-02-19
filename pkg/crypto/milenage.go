package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/free5gc/util/milenage"
	"github.com/free5gc/util/ueauth"
)

// AuthVector holds the 5G-AKA authentication vector.
type AuthVector struct {
	RAND  []byte `json:"rand"`
	AUTN  []byte `json:"autn"`
	XRES  []byte `json:"xres_star"`
	KAUSF []byte `json:"kausf"`
}

// GenerateAuthVector produces a 5G-AKA authentication vector.
// It generates a random RAND, runs Milenage to get IK/CK/RES/AUTN,
// then derives XRES* and KAUSF per 3GPP TS 33.501.
func GenerateAuthVector(k, opc, sqn, amf []byte, servingNetworkName string) (*AuthVector, error) {
	// Generate random RAND
	randVal := make([]byte, 16)
	if _, err := rand.Read(randVal); err != nil {
		return nil, fmt.Errorf("generate RAND: %w", err)
	}
	return GenerateAuthVectorWithRAND(k, opc, sqn, amf, randVal, servingNetworkName)
}

// GenerateAuthVectorWithRAND produces a 5G-AKA authentication vector using a provided RAND.
// This is useful for testing with known test vectors.
func GenerateAuthVectorWithRAND(k, opc, sqn, amf, randVal []byte, servingNetworkName string) (*AuthVector, error) {
	// Step 1: Run Milenage to get IK, CK, RES, AUTN
	ik, ck, res, autn, err := milenage.GenerateAKAParameters(opc, k, randVal, sqn, amf)
	if err != nil {
		return nil, fmt.Errorf("milenage: %w", err)
	}

	// Step 2: Derive XRES* per TS 33.501 Annex A.4
	xresStar, err := deriveXRESStar(ck, ik, servingNetworkName, randVal, res)
	if err != nil {
		return nil, fmt.Errorf("derive XRES*: %w", err)
	}

	// Step 3: Derive KAUSF per TS 33.501 Annex A.2
	kausf, err := deriveKAUSF(ck, ik, servingNetworkName, sqn, autn)
	if err != nil {
		return nil, fmt.Errorf("derive KAUSF: %w", err)
	}

	return &AuthVector{
		RAND:  randVal,
		AUTN:  autn,
		XRES:  xresStar,
		KAUSF: kausf,
	}, nil
}

// deriveXRESStar derives XRES* from CK, IK, serving network name, RAND, and RES.
// Per TS 33.501 Annex A.4: XRES* = KDF(CK||IK, FC=0x6B, P0=SNN, P1=RAND, P2=RES)
// Output is the last 16 bytes of the 32-byte KDF output.
func deriveXRESStar(ck, ik []byte, snn string, randVal, res []byte) ([]byte, error) {
	key := append(ck, ik...)
	snnBytes := []byte(snn)

	kdfVal, err := ueauth.GetKDFValue(
		key,
		ueauth.FC_FOR_RES_STAR_XRES_STAR_DERIVATION,
		snnBytes, ueauth.KDFLen(snnBytes),
		randVal, ueauth.KDFLen(randVal),
		res, ueauth.KDFLen(res),
	)
	if err != nil {
		return nil, err
	}

	// XRES* is the last 16 bytes of the 32-byte output
	return kdfVal[len(kdfVal)-16:], nil
}

// deriveKAUSF derives KAUSF from CK, IK, serving network name, and AUTN.
// Per TS 33.501 Annex A.2 (5G-AKA):
// KAUSF = KDF(CK||IK, FC=0x6A, P0=SNN, P1=SQN⊕AK)
func deriveKAUSF(ck, ik []byte, snn string, sqn, autn []byte) ([]byte, error) {
	sqnXorAK := autn[:6]
	snnBytes := []byte(snn)

	key := append(ck, ik...)
	kausf, err := ueauth.GetKDFValue(
		key,
		ueauth.FC_FOR_KAUSF_DERIVATION,
		snnBytes, ueauth.KDFLen(snnBytes),
		sqnXorAK, ueauth.KDFLen(sqnXorAK),
	)
	if err != nil {
		return nil, fmt.Errorf("derive KAUSF: %w", err)
	}

	return kausf, nil
}

// HexToBytes is a convenience helper that decodes a hex string.
func HexToBytes(s string) ([]byte, error) {
	return hex.DecodeString(s)
}
