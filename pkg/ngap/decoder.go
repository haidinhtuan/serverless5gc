package ngap

import (
	"fmt"

	"github.com/free5gc/aper"
	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/ngapType"
)

// NGAPContext carries extracted NGAP IE values from a decoded NGAP PDU.
type NGAPContext struct {
	PDU             *ngapType.NGAPPDU
	ProcedureCode   int64
	MessageType     int // 0=initiating, 1=successful, 2=unsuccessful
	RANUeNgapID     int64
	AMFUeNgapID     int64
	NASPDU          []byte
	UserLocationInfo []byte // raw for pass-through
}

// ParseNGAPMessage decodes an APER-encoded NGAP PDU and extracts common IEs.
func ParseNGAPMessage(data []byte) (*NGAPContext, error) {
	pdu, err := ngap.Decoder(data)
	if err != nil {
		return nil, fmt.Errorf("ngap aper decode: %w", err)
	}

	ctx := &NGAPContext{PDU: pdu}

	switch pdu.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		if pdu.InitiatingMessage == nil {
			return nil, fmt.Errorf("initiating message is nil")
		}
		ctx.ProcedureCode = pdu.InitiatingMessage.ProcedureCode.Value
		ctx.MessageType = 0
		extractInitiatingIEs(pdu.InitiatingMessage, ctx)
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		if pdu.SuccessfulOutcome == nil {
			return nil, fmt.Errorf("successful outcome is nil")
		}
		ctx.ProcedureCode = pdu.SuccessfulOutcome.ProcedureCode.Value
		ctx.MessageType = 1
	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		if pdu.UnsuccessfulOutcome == nil {
			return nil, fmt.Errorf("unsuccessful outcome is nil")
		}
		ctx.ProcedureCode = pdu.UnsuccessfulOutcome.ProcedureCode.Value
		ctx.MessageType = 2
	default:
		return nil, fmt.Errorf("unknown NGAP PDU present: %d", pdu.Present)
	}

	return ctx, nil
}

func extractInitiatingIEs(msg *ngapType.InitiatingMessage, ctx *NGAPContext) {
	switch msg.Value.Present {
	case ngapType.InitiatingMessagePresentInitialUEMessage:
		if m := msg.Value.InitialUEMessage; m != nil {
			for _, ie := range m.ProtocolIEs.List {
				switch ie.Value.Present {
				case ngapType.InitialUEMessageIEsPresentRANUENGAPID:
					if ie.Value.RANUENGAPID != nil {
						ctx.RANUeNgapID = ie.Value.RANUENGAPID.Value
					}
				case ngapType.InitialUEMessageIEsPresentNASPDU:
					if ie.Value.NASPDU != nil {
						ctx.NASPDU = ie.Value.NASPDU.Value
					}
				}
			}
		}
	case ngapType.InitiatingMessagePresentUplinkNASTransport:
		if m := msg.Value.UplinkNASTransport; m != nil {
			for _, ie := range m.ProtocolIEs.List {
				switch ie.Value.Present {
				case ngapType.UplinkNASTransportIEsPresentAMFUENGAPID:
					if ie.Value.AMFUENGAPID != nil {
						ctx.AMFUeNgapID = ie.Value.AMFUENGAPID.Value
					}
				case ngapType.UplinkNASTransportIEsPresentRANUENGAPID:
					if ie.Value.RANUENGAPID != nil {
						ctx.RANUeNgapID = ie.Value.RANUENGAPID.Value
					}
				case ngapType.UplinkNASTransportIEsPresentNASPDU:
					if ie.Value.NASPDU != nil {
						ctx.NASPDU = ie.Value.NASPDU.Value
					}
				}
			}
		}
	case ngapType.InitiatingMessagePresentDownlinkNASTransport:
		if m := msg.Value.DownlinkNASTransport; m != nil {
			for _, ie := range m.ProtocolIEs.List {
				switch ie.Value.Present {
				case ngapType.DownlinkNASTransportIEsPresentAMFUENGAPID:
					if ie.Value.AMFUENGAPID != nil {
						ctx.AMFUeNgapID = ie.Value.AMFUENGAPID.Value
					}
				case ngapType.DownlinkNASTransportIEsPresentRANUENGAPID:
					if ie.Value.RANUENGAPID != nil {
						ctx.RANUeNgapID = ie.Value.RANUENGAPID.Value
					}
				case ngapType.DownlinkNASTransportIEsPresentNASPDU:
					if ie.Value.NASPDU != nil {
						ctx.NASPDU = ie.Value.NASPDU.Value
					}
				}
			}
		}
	case ngapType.InitiatingMessagePresentNGSetupRequest:
		// NG Setup has no RAN/AMF UE NGAP IDs or NAS PDU
	}
}

// BuildNGSetupResponse builds an APER-encoded NGSetupResponse for the eval config.
// PLMN 001/01, S-NSSAI SST=1 SD=010203.
func BuildNGSetupResponse(plmnBytes []byte, sst byte, sd []byte) ([]byte, error) {
	sdVal := ngapType.SD{Value: sd}
	pdu := ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentSuccessfulOutcome,
		SuccessfulOutcome: &ngapType.SuccessfulOutcome{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeNGSetup},
			Criticality:   ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
			Value: ngapType.SuccessfulOutcomeValue{
				Present: ngapType.SuccessfulOutcomePresentNGSetupResponse,
				NGSetupResponse: &ngapType.NGSetupResponse{
					ProtocolIEs: ngapType.ProtocolIEContainerNGSetupResponseIEs{
						List: []ngapType.NGSetupResponseIEs{
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAMFName},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.NGSetupResponseIEsValue{
									Present: ngapType.NGSetupResponseIEsPresentAMFName,
									AMFName: &ngapType.AMFName{Value: "serverless5gc-amf"},
								},
							},
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDServedGUAMIList},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.NGSetupResponseIEsValue{
									Present:         ngapType.NGSetupResponseIEsPresentServedGUAMIList,
									ServedGUAMIList: buildServedGUAMIList(plmnBytes),
								},
							},
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRelativeAMFCapacity},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentIgnore},
								Value: ngapType.NGSetupResponseIEsValue{
									Present:             ngapType.NGSetupResponseIEsPresentRelativeAMFCapacity,
									RelativeAMFCapacity: &ngapType.RelativeAMFCapacity{Value: 255},
								},
							},
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDPLMNSupportList},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.NGSetupResponseIEsValue{
									Present: ngapType.NGSetupResponseIEsPresentPLMNSupportList,
									PLMNSupportList: &ngapType.PLMNSupportList{
										List: []ngapType.PLMNSupportItem{
											{
												PLMNIdentity: ngapType.PLMNIdentity{Value: plmnBytes},
												SliceSupportList: ngapType.SliceSupportList{
													List: []ngapType.SliceSupportItem{
														{
															SNSSAI: ngapType.SNSSAI{
																SST: ngapType.SST{Value: []byte{sst}},
																SD:  &sdVal,
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return ngap.Encoder(pdu)
}

func buildServedGUAMIList(plmnBytes []byte) *ngapType.ServedGUAMIList {
	return &ngapType.ServedGUAMIList{
		List: []ngapType.ServedGUAMIItem{
			{
				GUAMI: ngapType.GUAMI{
					PLMNIdentity: ngapType.PLMNIdentity{Value: plmnBytes},
					AMFRegionID:  ngapType.AMFRegionID{Value: aper.BitString{Bytes: []byte{0x01}, BitLength: 8}},
					AMFSetID:     ngapType.AMFSetID{Value: aper.BitString{Bytes: []byte{0x00, 0x40}, BitLength: 10}},
					AMFPointer:   ngapType.AMFPointer{Value: aper.BitString{Bytes: []byte{0x00}, BitLength: 6}},
				},
			},
		},
	}
}

// BuildDownlinkNASTransport builds an APER-encoded DownlinkNASTransport NGAP PDU.
func BuildDownlinkNASTransport(amfUeNgapID, ranUeNgapID int64, nasPDU []byte) ([]byte, error) {
	pdu := ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeDownlinkNASTransport},
			Criticality:   ngapType.Criticality{Value: ngapType.CriticalityPresentIgnore},
			Value: ngapType.InitiatingMessageValue{
				Present: ngapType.InitiatingMessagePresentDownlinkNASTransport,
				DownlinkNASTransport: &ngapType.DownlinkNASTransport{
					ProtocolIEs: ngapType.ProtocolIEContainerDownlinkNASTransportIEs{
						List: []ngapType.DownlinkNASTransportIEs{
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAMFUENGAPID},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.DownlinkNASTransportIEsValue{
									Present:     ngapType.DownlinkNASTransportIEsPresentAMFUENGAPID,
									AMFUENGAPID: &ngapType.AMFUENGAPID{Value: amfUeNgapID},
								},
							},
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRANUENGAPID},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.DownlinkNASTransportIEsValue{
									Present:     ngapType.DownlinkNASTransportIEsPresentRANUENGAPID,
									RANUENGAPID: &ngapType.RANUENGAPID{Value: ranUeNgapID},
								},
							},
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDNASPDU},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.DownlinkNASTransportIEsValue{
									Present: ngapType.DownlinkNASTransportIEsPresentNASPDU,
									NASPDU:  &ngapType.NASPDU{Value: nasPDU},
								},
							},
						},
					},
				},
			},
		},
	}
	return ngap.Encoder(pdu)
}

// BuildInitialContextSetupRequest builds an APER-encoded InitialContextSetupRequest
// containing the NAS Registration Accept.
func BuildInitialContextSetupRequest(amfUeNgapID, ranUeNgapID int64, nasPDU, plmnBytes []byte, sst byte, sd []byte) ([]byte, error) {
	sdVal := ngapType.SD{Value: sd}
	// Security key: 256 bits of zeros (dummy for eval)
	secKey := make([]byte, 32)

	pdu := ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeInitialContextSetup},
			Criticality:   ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
			Value: ngapType.InitiatingMessageValue{
				Present: ngapType.InitiatingMessagePresentInitialContextSetupRequest,
				InitialContextSetupRequest: &ngapType.InitialContextSetupRequest{
					ProtocolIEs: ngapType.ProtocolIEContainerInitialContextSetupRequestIEs{
						List: []ngapType.InitialContextSetupRequestIEs{
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAMFUENGAPID},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.InitialContextSetupRequestIEsValue{
									Present:     ngapType.InitialContextSetupRequestIEsPresentAMFUENGAPID,
									AMFUENGAPID: &ngapType.AMFUENGAPID{Value: amfUeNgapID},
								},
							},
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRANUENGAPID},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.InitialContextSetupRequestIEsValue{
									Present:     ngapType.InitialContextSetupRequestIEsPresentRANUENGAPID,
									RANUENGAPID: &ngapType.RANUENGAPID{Value: ranUeNgapID},
								},
							},
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDGUAMI},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.InitialContextSetupRequestIEsValue{
									Present: ngapType.InitialContextSetupRequestIEsPresentGUAMI,
									GUAMI: &ngapType.GUAMI{
										PLMNIdentity: ngapType.PLMNIdentity{Value: plmnBytes},
										AMFRegionID:  ngapType.AMFRegionID{Value: aper.BitString{Bytes: []byte{0x01}, BitLength: 8}},
										AMFSetID:     ngapType.AMFSetID{Value: aper.BitString{Bytes: []byte{0x00, 0x40}, BitLength: 10}},
										AMFPointer:   ngapType.AMFPointer{Value: aper.BitString{Bytes: []byte{0x00}, BitLength: 6}},
									},
								},
							},
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAllowedNSSAI},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.InitialContextSetupRequestIEsValue{
									Present: ngapType.InitialContextSetupRequestIEsPresentAllowedNSSAI,
									AllowedNSSAI: &ngapType.AllowedNSSAI{
										List: []ngapType.AllowedNSSAIItem{
											{
												SNSSAI: ngapType.SNSSAI{
													SST: ngapType.SST{Value: []byte{sst}},
													SD:  &sdVal,
												},
											},
										},
									},
								},
							},
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDUESecurityCapabilities},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.InitialContextSetupRequestIEsValue{
									Present: ngapType.InitialContextSetupRequestIEsPresentUESecurityCapabilities,
									UESecurityCapabilities: &ngapType.UESecurityCapabilities{
										NRencryptionAlgorithms:             ngapType.NRencryptionAlgorithms{Value: aper.BitString{Bytes: []byte{0xC0, 0x00}, BitLength: 16}},
										NRintegrityProtectionAlgorithms:    ngapType.NRintegrityProtectionAlgorithms{Value: aper.BitString{Bytes: []byte{0xC0, 0x00}, BitLength: 16}},
										EUTRAencryptionAlgorithms:          ngapType.EUTRAencryptionAlgorithms{Value: aper.BitString{Bytes: []byte{0xC0, 0x00}, BitLength: 16}},
										EUTRAintegrityProtectionAlgorithms: ngapType.EUTRAintegrityProtectionAlgorithms{Value: aper.BitString{Bytes: []byte{0xC0, 0x00}, BitLength: 16}},
									},
								},
							},
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDSecurityKey},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.InitialContextSetupRequestIEsValue{
									Present:     ngapType.InitialContextSetupRequestIEsPresentSecurityKey,
									SecurityKey: &ngapType.SecurityKey{Value: aper.BitString{Bytes: secKey, BitLength: 256}},
								},
							},
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDNASPDU},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentIgnore},
								Value: ngapType.InitialContextSetupRequestIEsValue{
									Present: ngapType.InitialContextSetupRequestIEsPresentNASPDU,
									NASPDU:  &ngapType.NASPDU{Value: nasPDU},
								},
							},
						},
					},
				},
			},
		},
	}
	return ngap.Encoder(pdu)
}

// PLMNBytes encodes MCC/MNC into 3-byte PLMN identity (3GPP TS 24.501).
// Example: PLMNBytes("001", "01") → [0x00, 0xF1, 0x10].
func PLMNBytes(mcc, mnc string) []byte {
	if len(mcc) != 3 {
		return []byte{0, 0, 0}
	}
	b0 := (mcc[1]-'0')<<4 | (mcc[0] - '0')
	var b1 byte
	if len(mnc) == 2 {
		b1 = 0xF0 | (mcc[2] - '0')
	} else {
		b1 = (mnc[0]-'0')<<4 | (mcc[2] - '0')
	}
	var b2 byte
	if len(mnc) == 2 {
		b2 = (mnc[1]-'0')<<4 | (mnc[0] - '0')
	} else {
		b2 = (mnc[2]-'0')<<4 | (mnc[1] - '0')
	}
	return []byte{b0, b1, b2}
}
