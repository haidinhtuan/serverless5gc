// Package nas provides 5G NAS (Non-Access Stratum) message types and constants
// per 3GPP TS 24.501 (5GS Mobility Management).
package nas

// Extended Protocol Discriminator (TS 24.501 Table 9.2.1)
const (
	EPD5GMM = 0x7E // 5GS Mobility Management
	EPD5GSM = 0x2E // 5GS Session Management
)

// Security Header Type (TS 24.501 Table 9.3.1)
const (
	SecurityHeaderPlain                          = 0x00
	SecurityHeaderIntegrityProtected             = 0x01
	SecurityHeaderIntegrityProtectedCiphered     = 0x02
	SecurityHeaderIntegrityProtectedNewCtx       = 0x03
	SecurityHeaderIntegrityProtectedCipheredNew  = 0x04
)

// 5GMM Message Types (TS 24.501 Table 9.7.1)
const (
	MsgTypeRegistrationRequest      = 0x41
	MsgTypeRegistrationAccept       = 0x42
	MsgTypeRegistrationComplete     = 0x43
	MsgTypeRegistrationReject       = 0x44
	MsgTypeDeregistrationRequestUE  = 0x45
	MsgTypeDeregistrationAcceptUE   = 0x46
	MsgTypeDeregistrationRequestNW  = 0x47
	MsgTypeDeregistrationAcceptNW   = 0x48
	MsgTypeServiceRequest           = 0x4C
	MsgTypeServiceAccept            = 0x4E
	MsgTypeServiceReject            = 0x4D
	MsgTypeAuthenticationRequest    = 0x56
	MsgTypeAuthenticationResponse   = 0x57
	MsgTypeAuthenticationReject     = 0x58
	MsgTypeAuthenticationFailure    = 0x59
	MsgTypeAuthenticationResult     = 0x5A
	MsgTypeSecurityModeCommand      = 0x5D
	MsgTypeSecurityModeComplete     = 0x5E
	MsgTypeSecurityModeReject       = 0x5F
	MsgTypeULNASTransport           = 0x67
	MsgTypeDLNASTransport           = 0x68
	MsgTypeIdentityRequest          = 0x5B
	MsgTypeIdentityResponse         = 0x5C
)

// 5GSM Message Types (TS 24.501 Table 9.8.1)
const (
	MsgTypePDUSessionEstablishmentRequest = 0xC1
)

// 5GMM Cause codes (TS 24.501 Table 9.11.3.2.1)
const (
	CauseIllegalUE                     = 3
	CauseIllegalME                     = 6
	CausePEINotAccepted                = 5
	Cause5GSServicesNotAllowed         = 7
	CauseUEIdentityCannotBeDerived     = 9
	CauseImplicitlyDeregistered        = 10
	CausePLMNNotAllowed                = 11
	CauseTrackingAreaNotAllowed        = 12
	CauseRoamingNotAllowed             = 13
	CauseNoSuitableCells               = 15
	CauseMACFailure                    = 20
	CauseSynchFailure                  = 21
	CauseCongestion                    = 22
	CauseUESecurityCapMismatch         = 23
	CauseSecurityModeRejected          = 24
	CauseN1ModeNotAllowed              = 27
	CauseRestrictedServiceArea         = 28
	CauseLADNNotAvailable              = 43
	CauseMaxPDUSessionsReached         = 65
	CauseInsufficientResourcesForSlice = 67
	CausePayloadNotForwarded           = 90
	CauseDNNNotSupportedOrNotSubscribed = 91
)

// 5GS Registration Type (TS 24.501 Table 9.11.3.7.1)
const (
	RegTypeInitialRegistration   = 0x01
	RegTypeMobilityRegistration  = 0x02
	RegTypePeriodicRegistration  = 0x03
	RegTypeEmergencyRegistration = 0x04
)

// 5GS Mobile Identity Type (TS 24.501 Table 9.11.3.4.1)
const (
	MobileIdentitySUCI    = 0x01
	MobileIdentity5GGUTI  = 0x02
	MobileIdentityIMEI    = 0x03
	MobileIdentity5GSTMSI = 0x04
	MobileIdentityImeisv  = 0x05
)

// 5G NAS Security Algorithms (TS 24.501 Table 9.11.3.34.1)
const (
	// Ciphering algorithms
	CipherAlg5GEA0 = 0x00 // Null ciphering
	CipherAlg5GEA1 = 0x01 // 128-5G-EA1 (SNOW 3G based)
	CipherAlg5GEA2 = 0x02 // 128-5G-EA2 (AES based)
	CipherAlg5GEA3 = 0x03 // 128-5G-EA3 (ZUC based)

	// Integrity protection algorithms
	IntegAlg5GIA0 = 0x00 // Null integrity
	IntegAlg5GIA1 = 0x01 // 128-5G-IA1 (SNOW 3G based)
	IntegAlg5GIA2 = 0x02 // 128-5G-IA2 (AES based)
	IntegAlg5GIA3 = 0x03 // 128-5G-IA3 (ZUC based)
)

// Registration Result (TS 24.501 Table 9.11.3.6.1)
const (
	RegResult3GPPAccess    = 0x01
	RegResultNon3GPPAccess = 0x02
	RegResult3GPPAndNon    = 0x03
)

// Timer Values (TS 24.501)
const (
	// T3512 - Periodic registration update timer (TS 24.501 Section 8.2.7.17)
	// Default: 54 minutes = 3240 seconds
	T3512Default = 3240

	// T3510 - Registration procedure guard timer (TS 24.501 Section 8.2.6.2)
	// Value: 15 seconds
	T3510Value = 15

	// T3502 - Registration retry timer
	// Default: 12 minutes = 720 seconds
	T3502Default = 720

	// T3580 - PDU session establishment guard timer (TS 24.501 Section 8.3.1)
	// Value: 5 seconds (network retransmits PDU Session Establishment Accept)
	T3580Value = 5

	// T3513 - Paging timer (TS 24.501 Section 8.2.3.4)
	// Value: 10 seconds
	T3513Value = 10

	// T3522 - Deregistration timer (TS 24.501 Section 8.2.12)
	// Value: 6 seconds (network retransmits Deregistration Request)
	T3522Value = 6

	// T3550 - Registration Accept retransmission timer (TS 24.501 Section 8.2.7.4)
	// Value: 6 seconds
	T3550Value = 6

	// T3560 - Security Mode Command retransmission timer (TS 24.501 Section 8.2.25.4)
	// Value: 6 seconds
	T3560Value = 6
)

// NAS IE Tags for optional IEs in Registration Accept (TS 24.501 Table 8.2.7.1)
const (
	IETag5GGUTI         = 0x77
	IETagAllowedNSSAI   = 0x15
	IETagRejectedNSSAI  = 0x11
	IETagT3512Value     = 0x5E
	IETagEquivPLMNs     = 0x4A
	IETagEmergencyNums  = 0x34
)
