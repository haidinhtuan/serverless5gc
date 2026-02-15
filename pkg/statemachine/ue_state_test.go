package statemachine

import (
	"testing"
	"time"
)

func TestRMStateString(t *testing.T) {
	if RMDeregistered.String() != "RM-DEREGISTERED" {
		t.Errorf("got %q, want RM-DEREGISTERED", RMDeregistered.String())
	}
	if RMRegistered.String() != "RM-REGISTERED" {
		t.Errorf("got %q, want RM-REGISTERED", RMRegistered.String())
	}
}

func TestCMStateString(t *testing.T) {
	if CMIdle.String() != "CM-IDLE" {
		t.Errorf("got %q, want CM-IDLE", CMIdle.String())
	}
	if CMConnected.String() != "CM-CONNECTED" {
		t.Errorf("got %q, want CM-CONNECTED", CMConnected.String())
	}
}

func TestNewUEStateMachine(t *testing.T) {
	sm := NewUEStateMachine("imsi-001010000000001")
	if sm.SUPI != "imsi-001010000000001" {
		t.Errorf("SUPI = %q, want imsi-001010000000001", sm.SUPI)
	}
	if sm.RMState != RMDeregistered {
		t.Errorf("initial RM state = %s, want RM-DEREGISTERED", sm.RMState)
	}
	if sm.CMState != CMIdle {
		t.Errorf("initial CM state = %s, want CM-IDLE", sm.CMState)
	}
	if len(sm.Transitions) != 0 {
		t.Errorf("initial transitions = %d, want 0", len(sm.Transitions))
	}
}

func TestRMTransition_DeregisteredToRegistered(t *testing.T) {
	sm := NewUEStateMachine("imsi-001010000000001")

	err := sm.TransitionRM(RMRegistered)
	if err != nil {
		t.Fatalf("transition error: %v", err)
	}
	if sm.RMState != RMRegistered {
		t.Errorf("RM state = %s, want RM-REGISTERED", sm.RMState)
	}
	if sm.RMStateChangedAt.IsZero() {
		t.Error("RMStateChangedAt should not be zero after transition")
	}
	if len(sm.Transitions) != 1 {
		t.Fatalf("transitions = %d, want 1", len(sm.Transitions))
	}
	tr := sm.Transitions[0]
	if tr.From != "RM-DEREGISTERED" || tr.To != "RM-REGISTERED" {
		t.Errorf("transition = %s->%s, want RM-DEREGISTERED->RM-REGISTERED", tr.From, tr.To)
	}
}

func TestRMTransition_RegisteredToDeregistered(t *testing.T) {
	sm := NewUEStateMachine("imsi-001010000000001")
	sm.TransitionRM(RMRegistered) // move to REGISTERED first

	err := sm.TransitionRM(RMDeregistered)
	if err != nil {
		t.Fatalf("transition error: %v", err)
	}
	if sm.RMState != RMDeregistered {
		t.Errorf("RM state = %s, want RM-DEREGISTERED", sm.RMState)
	}
	if len(sm.Transitions) != 2 {
		t.Fatalf("transitions = %d, want 2", len(sm.Transitions))
	}
}

func TestRMTransition_SameState_Error(t *testing.T) {
	sm := NewUEStateMachine("imsi-001010000000001")

	err := sm.TransitionRM(RMDeregistered)
	if err == nil {
		t.Fatal("expected error for same-state transition")
	}
}

func TestCMTransition_IdleToConnected(t *testing.T) {
	sm := NewUEStateMachine("imsi-001010000000001")
	sm.TransitionRM(RMRegistered) // must be registered for CM transitions

	err := sm.TransitionCM(CMConnected)
	if err != nil {
		t.Fatalf("transition error: %v", err)
	}
	if sm.CMState != CMConnected {
		t.Errorf("CM state = %s, want CM-CONNECTED", sm.CMState)
	}
	if sm.CMStateChangedAt.IsZero() {
		t.Error("CMStateChangedAt should not be zero after transition")
	}
}

func TestCMTransition_ConnectedToIdle(t *testing.T) {
	sm := NewUEStateMachine("imsi-001010000000001")
	sm.TransitionRM(RMRegistered)
	sm.TransitionCM(CMConnected)

	err := sm.TransitionCM(CMIdle)
	if err != nil {
		t.Fatalf("transition error: %v", err)
	}
	if sm.CMState != CMIdle {
		t.Errorf("CM state = %s, want CM-IDLE", sm.CMState)
	}
}

func TestCMTransition_WhileDeregistered_Error(t *testing.T) {
	sm := NewUEStateMachine("imsi-001010000000001")

	err := sm.TransitionCM(CMConnected)
	if err == nil {
		t.Fatal("expected error for CM transition while RM-DEREGISTERED")
	}
}

func TestCMTransition_SameState_Error(t *testing.T) {
	sm := NewUEStateMachine("imsi-001010000000001")
	sm.TransitionRM(RMRegistered)

	err := sm.TransitionCM(CMIdle) // already in CM-IDLE
	if err == nil {
		t.Fatal("expected error for same-state CM transition")
	}
}

func TestRMTransition_DeregisterForcesIdleCM(t *testing.T) {
	sm := NewUEStateMachine("imsi-001010000000001")
	sm.TransitionRM(RMRegistered)
	sm.TransitionCM(CMConnected)

	// Deregistration should force CM back to IDLE (TS 23.502)
	sm.TransitionRM(RMDeregistered)
	if sm.CMState != CMIdle {
		t.Errorf("CM state after deregistration = %s, want CM-IDLE", sm.CMState)
	}
}

func TestTransitionTimestamps(t *testing.T) {
	sm := NewUEStateMachine("imsi-001010000000001")
	before := time.Now()
	sm.TransitionRM(RMRegistered)
	after := time.Now()

	if sm.RMStateChangedAt.Before(before) || sm.RMStateChangedAt.After(after) {
		t.Error("RMStateChangedAt should be between before and after")
	}
}

func TestIsRegistered(t *testing.T) {
	sm := NewUEStateMachine("imsi-001010000000001")
	if sm.IsRegistered() {
		t.Error("should not be registered initially")
	}
	sm.TransitionRM(RMRegistered)
	if !sm.IsRegistered() {
		t.Error("should be registered after transition")
	}
}

func TestIsConnected(t *testing.T) {
	sm := NewUEStateMachine("imsi-001010000000001")
	if sm.IsConnected() {
		t.Error("should not be connected initially")
	}
	sm.TransitionRM(RMRegistered)
	sm.TransitionCM(CMConnected)
	if !sm.IsConnected() {
		t.Error("should be connected after transition")
	}
}

func TestRMStateValues(t *testing.T) {
	// Verify the string values match what UEContext expects
	if RMRegistered.StoreValue() != "REGISTERED" {
		t.Errorf("RMRegistered.StoreValue() = %q, want REGISTERED", RMRegistered.StoreValue())
	}
	if RMDeregistered.StoreValue() != "DEREGISTERED" {
		t.Errorf("RMDeregistered.StoreValue() = %q, want DEREGISTERED", RMDeregistered.StoreValue())
	}
	if CMIdle.StoreValue() != "IDLE" {
		t.Errorf("CMIdle.StoreValue() = %q, want IDLE", CMIdle.StoreValue())
	}
	if CMConnected.StoreValue() != "CONNECTED" {
		t.Errorf("CMConnected.StoreValue() = %q, want CONNECTED", CMConnected.StoreValue())
	}
}
