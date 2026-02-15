// Package statemachine implements 5G UE state transitions per TS 23.502.
// It enforces RM (Registration Management) and CM (Connection Management)
// state machines as specified in TS 23.502 Section 4.2.2 and Section 4.2.3.
package statemachine

import (
	"fmt"
	"time"
)

// RMState represents the Registration Management state (TS 23.502 Section 4.2.2).
type RMState int

const (
	RMDeregistered RMState = iota
	RMRegistered
)

func (s RMState) String() string {
	switch s {
	case RMDeregistered:
		return "RM-DEREGISTERED"
	case RMRegistered:
		return "RM-REGISTERED"
	default:
		return fmt.Sprintf("RM-UNKNOWN(%d)", int(s))
	}
}

// StoreValue returns the value stored in UEContext.RegistrationState.
func (s RMState) StoreValue() string {
	switch s {
	case RMDeregistered:
		return "DEREGISTERED"
	case RMRegistered:
		return "REGISTERED"
	default:
		return "UNKNOWN"
	}
}

// CMState represents the Connection Management state (TS 23.502 Section 4.2.3).
type CMState int

const (
	CMIdle CMState = iota
	CMConnected
)

func (s CMState) String() string {
	switch s {
	case CMIdle:
		return "CM-IDLE"
	case CMConnected:
		return "CM-CONNECTED"
	default:
		return fmt.Sprintf("CM-UNKNOWN(%d)", int(s))
	}
}

// StoreValue returns the value stored in UEContext.CmState.
func (s CMState) StoreValue() string {
	switch s {
	case CMIdle:
		return "IDLE"
	case CMConnected:
		return "CONNECTED"
	default:
		return "UNKNOWN"
	}
}

// TransitionRecord logs a single state transition with timestamp.
type TransitionRecord struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Timestamp time.Time `json:"timestamp"`
}

// UEStateMachine tracks RM and CM state for a single UE per TS 23.502.
type UEStateMachine struct {
	SUPI            string             `json:"supi"`
	RMState         RMState            `json:"rm_state"`
	CMState         CMState            `json:"cm_state"`
	RMStateChangedAt time.Time         `json:"rm_state_changed_at"`
	CMStateChangedAt time.Time         `json:"cm_state_changed_at"`
	Transitions     []TransitionRecord `json:"transitions"`
}

// NewUEStateMachine creates a state machine in the initial state:
// RM-DEREGISTERED, CM-IDLE (TS 23.502 Section 4.2.2.1).
func NewUEStateMachine(supi string) *UEStateMachine {
	return &UEStateMachine{
		SUPI:    supi,
		RMState: RMDeregistered,
		CMState: CMIdle,
	}
}

// TransitionRM performs an RM state transition with validation.
// Valid transitions per TS 23.502 Section 4.2.2:
//   - RM-DEREGISTERED -> RM-REGISTERED (Initial/Mobility/Emergency Registration)
//   - RM-REGISTERED -> RM-DEREGISTERED (UE/Network-initiated Deregistration)
//
// Transitioning to RM-DEREGISTERED also forces CM to CM-IDLE.
func (sm *UEStateMachine) TransitionRM(target RMState) error {
	if sm.RMState == target {
		return fmt.Errorf("already in %s", target)
	}

	now := time.Now()
	sm.Transitions = append(sm.Transitions, TransitionRecord{
		From:      sm.RMState.String(),
		To:        target.String(),
		Timestamp: now,
	})

	sm.RMState = target
	sm.RMStateChangedAt = now

	// TS 23.502: When transitioning to RM-DEREGISTERED, the CM state
	// implicitly transitions to CM-IDLE.
	if target == RMDeregistered && sm.CMState != CMIdle {
		sm.CMState = CMIdle
		sm.CMStateChangedAt = now
		sm.Transitions = append(sm.Transitions, TransitionRecord{
			From:      CMConnected.String(),
			To:        CMIdle.String(),
			Timestamp: now,
		})
	}

	return nil
}

// TransitionCM performs a CM state transition with validation.
// Valid transitions per TS 23.502 Section 4.2.3:
//   - CM-IDLE -> CM-CONNECTED (Service Request, NAS signalling connection established)
//   - CM-CONNECTED -> CM-IDLE (NAS signalling connection released)
//
// Precondition: UE must be in RM-REGISTERED state.
func (sm *UEStateMachine) TransitionCM(target CMState) error {
	if sm.RMState != RMRegistered {
		return fmt.Errorf("CM transition requires RM-REGISTERED, current: %s", sm.RMState)
	}
	if sm.CMState == target {
		return fmt.Errorf("already in %s", target)
	}

	now := time.Now()
	sm.Transitions = append(sm.Transitions, TransitionRecord{
		From:      sm.CMState.String(),
		To:        target.String(),
		Timestamp: now,
	})

	sm.CMState = target
	sm.CMStateChangedAt = now
	return nil
}

// IsRegistered returns true if the UE is in RM-REGISTERED state.
func (sm *UEStateMachine) IsRegistered() bool {
	return sm.RMState == RMRegistered
}

// IsConnected returns true if the UE is in CM-CONNECTED state.
func (sm *UEStateMachine) IsConnected() bool {
	return sm.CMState == CMConnected
}
