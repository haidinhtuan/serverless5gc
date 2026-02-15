package nas

import "fmt"

// causeStrings maps 5GMM cause codes to human-readable descriptions
// per TS 24.501 Table 9.11.3.2.1 (Annex A).
var causeStrings = map[uint8]string{
	CauseIllegalUE:                      "Illegal UE",
	CausePEINotAccepted:                 "PEI not accepted",
	CauseIllegalME:                      "Illegal ME",
	Cause5GSServicesNotAllowed:          "5GS services not allowed",
	CauseUEIdentityCannotBeDerived:      "UE identity cannot be derived from the network",
	CauseImplicitlyDeregistered:         "Implicitly de-registered",
	CausePLMNNotAllowed:                 "PLMN not allowed",
	CauseTrackingAreaNotAllowed:         "Tracking area not allowed",
	CauseRoamingNotAllowed:              "Roaming not allowed in this tracking area",
	CauseNoSuitableCells:                "No suitable cells in tracking area",
	CauseMACFailure:                     "MAC failure",
	CauseSynchFailure:                   "Synch failure",
	CauseCongestion:                     "Congestion",
	CauseUESecurityCapMismatch:          "UE security capabilities mismatch",
	CauseSecurityModeRejected:           "Security mode rejected, unspecified",
	CauseN1ModeNotAllowed:               "N1 mode not allowed",
	CauseRestrictedServiceArea:          "Restricted service area",
	CauseLADNNotAvailable:               "LADN not available",
	CauseMaxPDUSessionsReached:          "Maximum number of PDU sessions reached",
	CauseInsufficientResourcesForSlice:  "Insufficient resources for specific slice and DNN",
	CausePayloadNotForwarded:            "Payload was not forwarded",
	CauseDNNNotSupportedOrNotSubscribed: "DNN not supported or not subscribed in the slice",
}

// CauseString returns the human-readable description of a 5GMM cause code
// per TS 24.501 Annex A.
func CauseString(code uint8) string {
	if s, ok := causeStrings[code]; ok {
		return s
	}
	return fmt.Sprintf("Unknown 5GMM cause (%d)", code)
}

// IsRetryableCause returns true if the cause code indicates a temporary condition
// that the UE may retry after a backoff period (TS 24.501 Section 5.5.1.2.5).
func IsRetryableCause(code uint8) bool {
	switch code {
	case CauseCongestion, CauseInsufficientResourcesForSlice:
		return true
	default:
		return false
	}
}

// CauseIsProtocolError returns true if the cause code indicates a NAS protocol
// or security error (TS 24.501 Section 5.5.1.2.5, causes #20-#24).
func CauseIsProtocolError(code uint8) bool {
	switch code {
	case CauseMACFailure, CauseSynchFailure, CauseUESecurityCapMismatch, CauseSecurityModeRejected:
		return true
	default:
		return false
	}
}
