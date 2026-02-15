package nas

import "testing"

func TestCauseString(t *testing.T) {
	tests := []struct {
		code uint8
		want string
	}{
		{CauseIllegalUE, "Illegal UE"},
		{CausePEINotAccepted, "PEI not accepted"},
		{CauseIllegalME, "Illegal ME"},
		{Cause5GSServicesNotAllowed, "5GS services not allowed"},
		{CauseUEIdentityCannotBeDerived, "UE identity cannot be derived from the network"},
		{CauseImplicitlyDeregistered, "Implicitly de-registered"},
		{CausePLMNNotAllowed, "PLMN not allowed"},
		{CauseTrackingAreaNotAllowed, "Tracking area not allowed"},
		{CauseRoamingNotAllowed, "Roaming not allowed in this tracking area"},
		{CauseNoSuitableCells, "No suitable cells in tracking area"},
		{CauseMACFailure, "MAC failure"},
		{CauseSynchFailure, "Synch failure"},
		{CauseCongestion, "Congestion"},
		{CauseUESecurityCapMismatch, "UE security capabilities mismatch"},
		{CauseSecurityModeRejected, "Security mode rejected, unspecified"},
		{CauseN1ModeNotAllowed, "N1 mode not allowed"},
		{CauseRestrictedServiceArea, "Restricted service area"},
		{CauseLADNNotAvailable, "LADN not available"},
		{CauseMaxPDUSessionsReached, "Maximum number of PDU sessions reached"},
		{CauseInsufficientResourcesForSlice, "Insufficient resources for specific slice and DNN"},
		{CausePayloadNotForwarded, "Payload was not forwarded"},
		{CauseDNNNotSupportedOrNotSubscribed, "DNN not supported or not subscribed in the slice"},
		{0xFF, "Unknown 5GMM cause (255)"},
	}

	for _, tt := range tests {
		got := CauseString(tt.code)
		if got != tt.want {
			t.Errorf("CauseString(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestIsRetryableCause(t *testing.T) {
	retryable := []uint8{CauseCongestion, CauseInsufficientResourcesForSlice}
	for _, code := range retryable {
		if !IsRetryableCause(code) {
			t.Errorf("IsRetryableCause(%d) = false, want true", code)
		}
	}

	nonRetryable := []uint8{
		CauseIllegalUE, CausePEINotAccepted, Cause5GSServicesNotAllowed,
		CauseUEIdentityCannotBeDerived, CauseMACFailure, CauseSecurityModeRejected,
	}
	for _, code := range nonRetryable {
		if IsRetryableCause(code) {
			t.Errorf("IsRetryableCause(%d) = true, want false", code)
		}
	}
}

func TestCauseIsProtocolError(t *testing.T) {
	protoErrors := []uint8{CauseMACFailure, CauseSynchFailure, CauseUESecurityCapMismatch, CauseSecurityModeRejected}
	for _, code := range protoErrors {
		if !CauseIsProtocolError(code) {
			t.Errorf("CauseIsProtocolError(%d) = false, want true", code)
		}
	}

	notProto := []uint8{CauseIllegalUE, CauseCongestion, CausePLMNNotAllowed}
	for _, code := range notProto {
		if CauseIsProtocolError(code) {
			t.Errorf("CauseIsProtocolError(%d) = true, want false", code)
		}
	}
}
