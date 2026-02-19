package main

import (
	"encoding/hex"
	"fmt"

	"github.com/free5gc/util/milenage"
	"github.com/free5gc/util/ueauth"
	"github.com/haidinhtuan/serverless5gc/pkg/crypto"
)

func verifyCapture(label, randHex, autnHex, smcPlainHex, wireMACHex string) {
	fmt.Printf("\n=== %s ===\n", label)

	k, _ := hex.DecodeString("465B5CE8B199B49FAA5F0A2EE238A6BC")
	opc, _ := hex.DecodeString("E8ED289DEBA952E4283B54E88E6183CA")
	randVal, _ := hex.DecodeString(randHex)
	autnWire, _ := hex.DecodeString(autnHex)

	sqnHE, _, ik, ck, _, err := milenage.GenerateKeysWithAUTN(opc, k, randVal, autnWire)
	if err != nil {
		fmt.Printf("GenerateKeysWithAUTN error: %v\n", err)
		return
	}
	sqnXorAK := autnWire[:6]
	fmt.Printf("SQN=%x SQN^AK=%x\n", sqnHE, sqnXorAK)

	snn := "5G:mnc001.mcc001.3gppnetwork.org"
	snnBytes := []byte(snn)

	key := make([]byte, 32)
	copy(key, ck)
	copy(key[16:], ik)

	// 5G-AKA: KAUSF = KDF(CK||IK, FC=0x6A, SNN, SQN⊕AK)
	kausf, _ := ueauth.GetKDFValue(key, ueauth.FC_FOR_KAUSF_DERIVATION,
		snnBytes, ueauth.KDFLen(snnBytes), sqnXorAK, ueauth.KDFLen(sqnXorAK))
	fmt.Printf("KAUSF=%x\n", kausf)

	kseaf, _ := crypto.DeriveKSEAF(kausf, snn)
	fmt.Printf("KSEAF=%x\n", kseaf)

	kamf, _ := crypto.DeriveKAMF(kseaf, "001010000000001", []byte{0x00, 0x00})
	fmt.Printf("KAMF=%x\n", kamf)

	knasEnc, knasInt, _ := crypto.DeriveKNASKeys(kamf, 0x00, 0x02) // NEA0, NIA2
	fmt.Printf("KNASint=%x KNASenc=%x\n", knasInt, knasEnc)

	// Compute MAC over SQN(0) || plainNAS
	smcPlain, _ := hex.DecodeString(smcPlainHex)
	macInput := make([]byte, 1+len(smcPlain))
	macInput[0] = 0x00 // SQN=0
	copy(macInput[1:], smcPlain)
	mac, _ := crypto.ComputeNASMAC(knasInt, 0, 0x01, 0x01, macInput) // count=0, bearer=1, dir=1
	fmt.Printf("Computed MAC=%x\n", mac)
	fmt.Printf("Wire MAC    =%s\n", wireMACHex)
	fmt.Printf("MAC match: %v\n", hex.EncodeToString(mac) == wireMACHex)
}

func main() {
	// Test 1: Open5GS capture
	// Auth Request: 7e00560002000021ec6a117c3390b2d7ab0f7378ebb6f9c22010cde755cee8d98000fe625770fe2f929f
	// SMC: 7e038736f2ab007e005d020004f0f0f0f0e1360102
	verifyCapture("Open5GS",
		"ec6a117c3390b2d7ab0f7378ebb6f9c2", // RAND
		"cde755cee8d98000fe625770fe2f929f", // AUTN
		"7e005d020004f0f0f0f0e1360102",     // plainNAS (includes IMEISV req + Additional 5G sec info)
		"8736f2ab",                          // wire MAC
	)

	// Test 2: Our proxy capture
	// Auth Request: 7e0056000200002149a91f595f84c7388f409f2728f5f9022010295be99dec21800032f1b4317f2a6a12
	// SMC: 7e0346593a3b007e005d020004f0f0f0f0
	verifyCapture("Our Proxy",
		"49a91f595f84c7388f409f2728f5f902", // RAND
		"295be99dec21800032f1b4317f2a6a12", // AUTN
		"7e005d020004f0f0f0f0",             // plainNAS (no optional IEs)
		"46593a3b",                          // wire MAC
	)
}
