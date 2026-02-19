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
	RawBytes           []byte // raw bytes from Registration Request for exact replay in SMC
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
	// Header: EPD + SecHdr + MsgType + 5GS Registration Result (LV: length=1 + value)
	msg := []byte{
		EPD5GMM,
		SecurityHeaderPlain,
		MsgTypeRegistrationAccept,
		0x01,                      // 5GS registration result length
		accept.RegistrationResult, // 5GS registration result value
	}

	// 5G-GUTI (optional IE, tag 0x77, TLV-E format: IEI + 2-byte length + value)
	if accept.GUTI != "" {
		gutiBytes := encodeGUTIValue(accept.GUTI)
		gutiLen := len(gutiBytes)
		msg = append(msg, IETag5GGUTI)
		msg = append(msg, byte(gutiLen>>8), byte(gutiLen))
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

// EncodeAuthenticationRequest builds a NAS Authentication Request wire message.
// (TS 24.501 Section 8.2.1)
func EncodeAuthenticationRequest(rand []byte, autn []byte) []byte {
	msg := []byte{
		EPD5GMM,
		SecurityHeaderPlain,
		MsgTypeAuthenticationRequest,
		0x00, // ngKSI
		0x02, // ABBA length = 2 (LV format: 1-byte length)
		0x00, 0x00, // ABBA value
	}
	// Authentication parameter RAND (IEI 0x21, TV, 17 bytes)
	msg = append(msg, 0x21)
	msg = append(msg, rand[:16]...)
	// Authentication parameter AUTN (IEI 0x20, TLV)
	msg = append(msg, 0x20, 0x10)
	msg = append(msg, autn[:16]...)
	return msg
}

// DecodeAuthenticationResponse decodes a NAS Authentication Response and returns the RES*.
// (TS 24.501 Section 8.2.2)
// The auth response parameter is optional (IEI 0x2D, TLV-E format).
func DecodeAuthenticationResponse(data []byte) ([]byte, error) {
	if len(data) < 3 {
		return nil, fmt.Errorf("NAS authentication response too short: %d bytes", len(data))
	}
	if data[0] != EPD5GMM {
		return nil, fmt.Errorf("unexpected EPD: 0x%02x, want 0x%02x", data[0], EPD5GMM)
	}
	if data[2] != MsgTypeAuthenticationResponse {
		return nil, fmt.Errorf("unexpected message type: 0x%02x, want 0x%02x", data[2], MsgTypeAuthenticationResponse)
	}

	// Parse optional IEs after header (byte 3+)
	offset := 3
	for offset < len(data) {
		iei := data[offset]
		switch iei {
		case 0x2D: // Authentication response parameter (TLV: IEI + 1-byte length + value)
			offset++
			if offset >= len(data) {
				return nil, fmt.Errorf("auth response parameter truncated at length")
			}
			paramLen := int(data[offset])
			offset++
			if offset+paramLen > len(data) {
				return nil, fmt.Errorf("auth response parameter length %d exceeds data", paramLen)
			}
			res := make([]byte, paramLen)
			copy(res, data[offset:offset+paramLen])
			return res, nil
		case 0x78: // EAP message (TLV-E), skip
			offset++
			if offset+2 > len(data) {
				return nil, fmt.Errorf("EAP message truncated")
			}
			eapLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
			offset += 2 + eapLen
		default:
			return nil, fmt.Errorf("unknown IE 0x%02x in authentication response", iei)
		}
	}

	return nil, fmt.Errorf("no authentication response parameter found")
}

// DecodePDUSessionEstablishmentRequest decodes a NAS PDU Session Establishment Request.
// (TS 24.501 Section 8.3.1)
func DecodePDUSessionEstablishmentRequest(data []byte) (pduSessionID uint8, pduSessionType uint8, err error) {
	if len(data) < 6 {
		return 0, 0, fmt.Errorf("PDU session establishment request too short: %d bytes", len(data))
	}
	if data[0] != EPD5GSM {
		return 0, 0, fmt.Errorf("unexpected EPD: 0x%02x, want 0x%02x", data[0], EPD5GSM)
	}
	if data[3] != MsgTypePDUSessionEstablishmentRequest {
		return 0, 0, fmt.Errorf("unexpected message type: 0x%02x, want 0x%02x", data[3], MsgTypePDUSessionEstablishmentRequest)
	}

	pduSessionID = data[1]
	pduSessionType = 0x01 // default: IPv4

	// Skip header (4) + mandatory integrity protection max data rate (2)
	offset := 6
	for offset < len(data) {
		iei := data[offset]
		// Type 1 IEs (half-octet)
		switch iei >> 4 {
		case 0x09: // PDU session type
			pduSessionType = iei & 0x0F
			offset++
			continue
		case 0x0A: // SSC mode, skip
			offset++
			continue
		}
		// TLV IE: IEI(1) + Length(1) + Value(N)
		offset++
		if offset >= len(data) {
			break
		}
		offset += 1 + int(data[offset])
	}

	return pduSessionID, pduSessionType, nil
}

// DecodeULNASTransport decodes a NAS UL NAS Transport message.
// (TS 24.501 Section 8.2.10)
func DecodeULNASTransport(data []byte) (payloadContainerType uint8, pduSessionID uint8, payload []byte, dnn string, snssai *NSSAI, err error) {
	if len(data) < 6 {
		err = fmt.Errorf("UL NAS Transport too short: %d bytes", len(data))
		return
	}
	if data[0] != EPD5GMM {
		err = fmt.Errorf("unexpected EPD: 0x%02x, want 0x%02x", data[0], EPD5GMM)
		return
	}
	if data[2] != MsgTypeULNASTransport {
		err = fmt.Errorf("unexpected message type: 0x%02x, want 0x%02x", data[2], MsgTypeULNASTransport)
		return
	}

	payloadContainerType = data[3] & 0x0F

	// Payload container (LV-E: 2-byte length + value)
	containerLen := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 6+containerLen {
		err = fmt.Errorf("payload container length %d exceeds data", containerLen)
		return
	}
	payload = make([]byte, containerLen)
	copy(payload, data[6:6+containerLen])

	// Parse optional IEs
	offset := 6 + containerLen
	for offset < len(data) {
		iei := data[offset]

		// Type 1 IEs (half-octet)
		switch iei >> 4 {
		case 0x08: // Request type
			offset++
			continue
		case 0x0A: // MA PDU session info
			offset++
			continue
		case 0x0F: // Release assistance indication
			offset++
			continue
		}

		// Type 3 IEs (TV, fixed length)
		switch iei {
		case 0x12: // PDU session ID
			if offset+1 < len(data) {
				pduSessionID = data[offset+1]
			}
			offset += 2
			continue
		case 0x59: // Old PDU session ID
			offset += 2
			continue
		}

		// Type 4 IEs (TLV)
		offset++
		if offset >= len(data) {
			break
		}
		ieLen := int(data[offset])
		offset++
		if offset+ieLen > len(data) {
			break
		}

		switch iei {
		case 0x22: // S-NSSAI
			if ieLen >= 1 {
				n := NSSAI{SST: data[offset]}
				if ieLen >= 4 {
					n.HasSD = true
					copy(n.SD[:], data[offset+1:offset+4])
				}
				snssai = &n
			}
		case 0x25: // DNN
			if ieLen >= 1 {
				dnn = decodeDNN(data[offset : offset+ieLen])
			}
		}

		offset += ieLen
	}

	return
}

// StripSecurityHeader strips the NAS security header from a protected NAS message.
// If the message is plain (security header type = 0x00), returns it unchanged.
// For protected NAS: [EPD, SecHdr, MAC(4), SQN(1), InnerNAS...] → InnerNAS
func StripSecurityHeader(data []byte) []byte {
	if len(data) < 2 {
		return data
	}
	secHdrType := data[1] & 0x0F
	if secHdrType == SecurityHeaderPlain {
		return data
	}
	// Protected NAS: EPD(1) + SecHdr(1) + MAC(4) + SQN(1) = 7 bytes header
	if len(data) <= 7 {
		return data
	}
	return data[7:]
}

// WrapSecurityHeader wraps a plain NAS message with a security header.
// For null security (5GEA0/5GIA0), MAC is all zeros.
func WrapSecurityHeader(inner []byte, secHdrType byte, sqn byte) []byte {
	outer := make([]byte, 0, 7+len(inner))
	outer = append(outer, EPD5GMM, secHdrType)
	outer = append(outer, 0x00, 0x00, 0x00, 0x00) // MAC (zeros for null integrity)
	outer = append(outer, sqn)
	outer = append(outer, inner...)
	return outer
}

// --- Internal encoding/decoding helpers ---

func decodeSUCI(data []byte) string {
	// SUCI decoding per TS 24.501 Section 9.11.3.4:
	//   Byte 0: SUPI format (bits 7-4) | Type of identity (bits 3-0)
	//   Bytes 1-3: MCC/MNC in BCD (same encoding as PLMN identity)
	//   Bytes 4-5: Routing indicator (BCD)
	//   Byte 6: Protection scheme ID
	//   Byte 7: Home network public key identifier
	//   Bytes 8+: Scheme output (MSIN in BCD for null scheme)
	if len(data) < 9 {
		return "unknown-suci"
	}

	// MCC: digit1=low(byte1), digit2=high(byte1), digit3=low(byte2)
	mcc := fmt.Sprintf("%d%d%d", data[1]&0x0f, (data[1]>>4)&0x0f, data[2]&0x0f)

	// MNC: check if 2-digit (high nibble of byte2 == 0xF) or 3-digit
	mnc3 := (data[2] >> 4) & 0x0f
	var mnc string
	if mnc3 == 0x0f {
		mnc = fmt.Sprintf("%d%d", data[3]&0x0f, (data[3]>>4)&0x0f)
	} else {
		mnc = fmt.Sprintf("%d%d%d", data[3]&0x0f, (data[3]>>4)&0x0f, mnc3)
	}

	// MSIN starts at byte 8 (after Home Network Public Key Identifier at byte 7)
	msin := ""
	for i := 8; i < len(data); i++ {
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
	// Store raw bytes for exact replay in SMC
	cap.RawBytes = make([]byte, len(data))
	copy(cap.RawBytes, data)
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
	// Use raw bytes for exact replay if available
	if len(cap.RawBytes) > 0 {
		out := make([]byte, len(cap.RawBytes))
		copy(out, cap.RawBytes)
		return out
	}
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

func decodeDNN(data []byte) string {
	var result string
	i := 0
	for i < len(data) {
		labelLen := int(data[i])
		i++
		if i+labelLen > len(data) || labelLen == 0 {
			break
		}
		if result != "" {
			result += "."
		}
		result += string(data[i : i+labelLen])
		i += labelLen
	}
	return result
}

func encodeGUTIValue(guti string) []byte {
	// Encode 5G-GUTI per TS 24.501 Section 9.11.3.4:
	// Octet 1: spare(4 bits) | type(4 bits=0xF2 for 5G-GUTI: 1111 | 0010)
	// Octet 2-3: MCC digit2|MCC digit1 | MNC digit3|MCC digit3
	//            MNC digit2|MNC digit1
	// Octet 4: AMF Region ID
	// Octet 5-6: AMF Set ID (10 bits) | AMF Pointer (6 bits)
	// Octet 7-10: 5G-TMSI (4 bytes)
	// Use fixed values for PLMN 001/01, AMF region=0x01, set=0x01, pointer=0x00
	return []byte{
		0xF2,       // spare(1111) | identity type(0010 = 5G-GUTI)
		0x00, 0xF1, // MCC=001: d2(0)|d1(0), MNC d3(F)|MCC d3(1)
		0x10,       // MNC=01: d2(1)|d1(0)
		0x01,       // AMF Region ID
		0x00, 0x40, // AMF Set ID=1 (10 bits) | AMF Pointer=0 (6 bits)
		0x00, 0x00, 0x00, 0x01, // 5G-TMSI = 1
	}
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
