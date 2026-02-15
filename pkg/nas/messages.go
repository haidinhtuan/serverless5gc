package nas

import (
	"encoding/binary"
	"fmt"
)

// RegistrationRequest represents a decoded 5GMM Registration Request
// (TS 24.501 Section 8.2.6).
type RegistrationRequest struct {
	RegistrationType uint8
	NgKSI            uint8
	MobileIdentity   MobileIdentity
	UESecCap         *UESecurityCapability
	RequestedNSSAI   []NSSAI
}

// MobileIdentity represents a 5GS Mobile Identity IE (TS 24.501 Section 9.11.3.4).
type MobileIdentity struct {
	Type  uint8  // MobileIdentitySUCI, MobileIdentity5GGUTI, etc.
	Value string // SUPI, GUTI, or IMEI as string representation
}

// UESecurityCapability represents UE security capabilities (TS 24.501 Section 9.11.3.54).
type UESecurityCapability struct {
	EA0, EA1, EA2, EA3 bool // 5G-EA ciphering algorithms supported
	IA0, IA1, IA2, IA3 bool // 5G-IA integrity algorithms supported
}

// NSSAI represents an S-NSSAI for NAS messages.
type NSSAI struct {
	SST uint8
	SD  [3]byte
	HasSD bool
}

// RegistrationAccept represents a 5GMM Registration Accept to encode
// (TS 24.501 Section 8.2.7).
type RegistrationAccept struct {
	RegistrationResult uint8
	GUTI               string
	AllowedNSSAI       []NSSAI
	T3512Value         uint32 // seconds
}

// RegistrationReject represents a 5GMM Registration Reject
// (TS 24.501 Section 8.2.8).
type RegistrationReject struct {
	CauseCode uint8
}

// SecurityModeCommand represents a NAS Security Mode Command
// (TS 24.501 Section 8.2.25).
type SecurityModeCommand struct {
	SelectedCiphering uint8 // 5G-EA algorithm
	SelectedIntegrity uint8 // 5G-IA algorithm
	NgKSI             uint8
	ReplayedUESecCap  *UESecurityCapability
}

// SecurityModeComplete represents a NAS Security Mode Complete
// (TS 24.501 Section 8.2.26).
type SecurityModeComplete struct {
	IMEISV string // optional
}

// DecodeRegistrationRequest decodes a NAS Registration Request from wire bytes.
// Wire format (TS 24.501 Section 8.2.6):
//
//	Byte 0: Extended Protocol Discriminator (0x7E)
//	Byte 1: Security Header Type (upper 4 bits) | Spare (lower 4 bits)
//	Byte 2: Message Type (0x41)
//	Byte 3: Registration Type (lower 3 bits) | ngKSI (upper 4 bits)
//	Byte 4-5: 5GS Mobile Identity length
//	Byte 6+: 5GS Mobile Identity value
func DecodeRegistrationRequest(data []byte) (*RegistrationRequest, error) {
	if len(data) < 7 {
		return nil, fmt.Errorf("NAS registration request too short: %d bytes", len(data))
	}

	if data[0] != EPD5GMM {
		return nil, fmt.Errorf("unexpected EPD: 0x%02x, want 0x%02x", data[0], EPD5GMM)
	}
	if data[2] != MsgTypeRegistrationRequest {
		return nil, fmt.Errorf("unexpected message type: 0x%02x, want 0x%02x", data[2], MsgTypeRegistrationRequest)
	}

	req := &RegistrationRequest{
		RegistrationType: data[3] & 0x07,
		NgKSI:            (data[3] >> 4) & 0x07,
	}

	// 5GS Mobile Identity: length (2 bytes) + value
	idLen := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 6+idLen || idLen < 1 {
		return nil, fmt.Errorf("mobile identity length %d exceeds data", idLen)
	}

	idData := data[6 : 6+idLen]
	req.MobileIdentity.Type = idData[0] & 0x07

	// Extract identity value based on type
	switch req.MobileIdentity.Type {
	case MobileIdentitySUCI:
		req.MobileIdentity.Value = decodeSUCI(idData)
	case MobileIdentity5GGUTI:
		req.MobileIdentity.Value = decodeGUTI(idData)
	default:
		req.MobileIdentity.Value = fmt.Sprintf("type-%d", req.MobileIdentity.Type)
	}

	// Parse optional IEs after mobile identity
	offset := 6 + idLen
	for offset < len(data)-1 {
		ieTag := data[offset]
		offset++
		if offset >= len(data) {
			break
		}
		ieLen := int(data[offset])
		offset++
		if offset+ieLen > len(data) {
			break
		}
		ieVal := data[offset : offset+ieLen]
		offset += ieLen

		switch ieTag {
		case 0x2E: // UE Security Capability (Tag 0x2E, TS 24.501 Table 8.2.6.1)
			if len(ieVal) >= 2 {
				req.UESecCap = decodeUESecCap(ieVal)
			}
		case IETagAllowedNSSAI: // Requested NSSAI uses same tag format
			req.RequestedNSSAI = decodeNSSAIList(ieVal)
		}
	}

	return req, nil
}

// EncodeRegistrationAccept builds a NAS Registration Accept wire message.
func EncodeRegistrationAccept(accept *RegistrationAccept) []byte {
	// Header
	msg := []byte{
		EPD5GMM,
		SecurityHeaderPlain,
		MsgTypeRegistrationAccept,
		accept.RegistrationResult,
	}

	// 5G-GUTI (optional IE, tag 0x77)
	if accept.GUTI != "" {
		gutiBytes := encodeGUTIValue(accept.GUTI)
		msg = append(msg, IETag5GGUTI)
		msg = append(msg, byte(len(gutiBytes)))
		msg = append(msg, gutiBytes...)
	}

	// Allowed NSSAI (optional IE, tag 0x15)
	if len(accept.AllowedNSSAI) > 0 {
		nssaiBytes := encodeNSSAIList(accept.AllowedNSSAI)
		msg = append(msg, IETagAllowedNSSAI)
		msg = append(msg, byte(len(nssaiBytes)))
		msg = append(msg, nssaiBytes...)
	}

	// T3512 value (optional IE, tag 0x5E)
	if accept.T3512Value > 0 {
		msg = append(msg, IETagT3512Value)
		msg = append(msg, 0x01)                           // length
		msg = append(msg, encodeGPRSTimer3(accept.T3512Value)) // timer value
	}

	return msg
}

// EncodeRegistrationReject builds a NAS Registration Reject wire message.
func EncodeRegistrationReject(reject *RegistrationReject) []byte {
	return []byte{
		EPD5GMM,
		SecurityHeaderPlain,
		MsgTypeRegistrationReject,
		reject.CauseCode,
	}
}

// EncodeSecurityModeCommand builds a NAS Security Mode Command wire message.
// (TS 24.501 Section 8.2.25)
func EncodeSecurityModeCommand(smc *SecurityModeCommand) []byte {
	msg := []byte{
		EPD5GMM,
		SecurityHeaderPlain,
		MsgTypeSecurityModeCommand,
		// Selected NAS security algorithms (TS 24.501 Section 9.11.3.34)
		(smc.SelectedCiphering << 4) | smc.SelectedIntegrity,
		// NAS key set identifier
		smc.NgKSI & 0x07,
	}

	// Replayed UE security capabilities
	if smc.ReplayedUESecCap != nil {
		capBytes := encodeUESecCap(smc.ReplayedUESecCap)
		msg = append(msg, byte(len(capBytes)))
		msg = append(msg, capBytes...)
	}

	return msg
}

// DecodeSecurityModeComplete decodes a NAS Security Mode Complete from wire bytes.
// Wire format (TS 24.501 Section 8.2.26):
//
//	Byte 0: Extended Protocol Discriminator (0x7E)
//	Byte 1: Security Header Type
//	Byte 2: Message Type (0x5E)
//	Byte 3+: Optional IEs (IMEISV via 5GS Mobile Identity, NAS message container)
func DecodeSecurityModeComplete(data []byte) (*SecurityModeComplete, error) {
	if len(data) < 3 {
		return nil, fmt.Errorf("NAS Security Mode Complete too short: %d bytes", len(data))
	}

	if data[0] != EPD5GMM {
		return nil, fmt.Errorf("unexpected EPD: 0x%02x, want 0x%02x", data[0], EPD5GMM)
	}
	if data[2] != MsgTypeSecurityModeComplete {
		return nil, fmt.Errorf("unexpected message type: 0x%02x, want 0x%02x", data[2], MsgTypeSecurityModeComplete)
	}

	smc := &SecurityModeComplete{}

	// Parse optional IEs after header
	offset := 3
	for offset < len(data)-1 {
		ieTag := data[offset]
		offset++
		if offset >= len(data) {
			break
		}
		ieLen := int(data[offset])
		offset++
		if offset+ieLen > len(data) {
			break
		}
		ieVal := data[offset : offset+ieLen]
		offset += ieLen

		switch ieTag {
		case IETag5GGUTI: // IMEISV uses the 5GS Mobile Identity IE container (tag 0x77)
			if len(ieVal) >= 1 && (ieVal[0]&0x07) == MobileIdentityImeisv {
				smc.IMEISV = decodeIMEISV(ieVal)
			}
		}
	}

	return smc, nil
}

// --- Internal encoding/decoding helpers ---

func decodeSUCI(data []byte) string {
	// Simplified SUCI decoding: extract IMSI digits from BCD-encoded bytes
	// Real implementation: TS 24.501 Section 9.11.3.4, SUCI structure
	if len(data) < 8 {
		return "unknown-suci"
	}
	// SUCI: type(1) + MCC/MNC(3) + routing_indicator(2) + scheme(1) + scheme_output
	mcc := fmt.Sprintf("%d%d%d", (data[1]>>4)&0x0f, data[2]&0x0f, (data[2]>>4)&0x0f)
	mnc := fmt.Sprintf("%d%d", data[3]&0x0f, (data[3]>>4)&0x0f)
	// Extract MSIN from remaining BCD digits
	msin := ""
	for i := 7; i < len(data); i++ {
		msin += fmt.Sprintf("%d", data[i]&0x0f)
		if (data[i]>>4)&0x0f != 0x0f {
			msin += fmt.Sprintf("%d", (data[i]>>4)&0x0f)
		}
	}
	return fmt.Sprintf("imsi-%s%s%s", mcc, mnc, msin)
}

func decodeIMEISV(data []byte) string {
	// IMEISV: type(1) + BCD-encoded digits
	if len(data) < 2 {
		return ""
	}
	imeisv := ""
	for i := 1; i < len(data); i++ {
		lo := data[i] & 0x0F
		hi := (data[i] >> 4) & 0x0F
		imeisv += fmt.Sprintf("%d", lo)
		if hi != 0x0F {
			imeisv += fmt.Sprintf("%d", hi)
		}
	}
	return imeisv
}

func decodeGUTI(data []byte) string {
	if len(data) < 10 {
		return "unknown-guti"
	}
	return fmt.Sprintf("5g-guti-%02x%02x%02x%02x%02x",
		data[1], data[2], data[3], data[4], data[5])
}

func decodeUESecCap(data []byte) *UESecurityCapability {
	cap := &UESecurityCapability{}
	if len(data) >= 1 {
		cap.EA0 = (data[0] & 0x80) != 0
		cap.EA1 = (data[0] & 0x40) != 0
		cap.EA2 = (data[0] & 0x20) != 0
		cap.EA3 = (data[0] & 0x10) != 0
	}
	if len(data) >= 2 {
		cap.IA0 = (data[1] & 0x80) != 0
		cap.IA1 = (data[1] & 0x40) != 0
		cap.IA2 = (data[1] & 0x20) != 0
		cap.IA3 = (data[1] & 0x10) != 0
	}
	return cap
}

func encodeUESecCap(cap *UESecurityCapability) []byte {
	var ea, ia byte
	if cap.EA0 { ea |= 0x80 }
	if cap.EA1 { ea |= 0x40 }
	if cap.EA2 { ea |= 0x20 }
	if cap.EA3 { ea |= 0x10 }
	if cap.IA0 { ia |= 0x80 }
	if cap.IA1 { ia |= 0x40 }
	if cap.IA2 { ia |= 0x20 }
	if cap.IA3 { ia |= 0x10 }
	return []byte{ea, ia}
}

func decodeNSSAIList(data []byte) []NSSAI {
	var list []NSSAI
	i := 0
	for i < len(data) {
		if i >= len(data) {
			break
		}
		sLen := int(data[i])
		i++
		if i+sLen > len(data) || sLen < 1 {
			break
		}
		n := NSSAI{SST: data[i]}
		if sLen >= 4 {
			n.HasSD = true
			copy(n.SD[:], data[i+1:i+4])
		}
		list = append(list, n)
		i += sLen
	}
	return list
}

func encodeNSSAIList(nssais []NSSAI) []byte {
	var out []byte
	for _, n := range nssais {
		if n.HasSD {
			out = append(out, 4, n.SST, n.SD[0], n.SD[1], n.SD[2])
		} else {
			out = append(out, 1, n.SST)
		}
	}
	return out
}

func encodeGUTIValue(guti string) []byte {
	// Simplified: return GUTI as raw bytes with type prefix
	out := []byte{MobileIdentity5GGUTI}
	out = append(out, []byte(guti)...)
	return out
}

// encodeGPRSTimer3 encodes a duration (seconds) into GPRS Timer 3 format
// (TS 24.008 Table 10.5.163a): bits 7-5 = unit, bits 4-0 = value.
// Units: 000=10min, 001=1hr, 010=10hr, 011=2s, 100=30s, 101=1min, 110=320hr
func encodeGPRSTimer3(seconds uint32) byte {
	switch {
	case seconds <= 62: // 2-second increments (max 31*2=62s)
		return 0x60 | byte(seconds/2)
	case seconds <= 930: // 30-second increments (max 31*30=930s)
		return 0x80 | byte(seconds/30)
	case seconds <= 1860: // 1-minute increments (max 31*60=1860s)
		return 0xA0 | byte(seconds/60)
	default: // 10-minute increments (max 31*600=18600s)
		return 0x00 | byte(seconds/600)
	}
}
