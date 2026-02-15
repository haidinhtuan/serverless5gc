package nas

import "time"

// TimerConfig holds configuration for a 3GPP NAS timer per TS 24.501.
type TimerConfig struct {
	Name       string        // Timer identifier (e.g., "T3512")
	Duration   time.Duration // Timer duration
	MaxRetries int           // Maximum retransmissions (0 = no retransmission)
}

// NewTimerConfig creates a timer configuration from a name and seconds value.
func NewTimerConfig(name string, seconds uint32) TimerConfig {
	return TimerConfig{
		Name:     name,
		Duration: time.Duration(seconds) * time.Second,
	}
}

// NewTimerConfigWithRetries creates a timer with retransmission support.
func NewTimerConfigWithRetries(name string, seconds uint32, maxRetries int) TimerConfig {
	tc := NewTimerConfig(name, seconds)
	tc.MaxRetries = maxRetries
	return tc
}

// IsExpired returns true if the timer started at 'start' has expired.
func (tc TimerConfig) IsExpired(start time.Time) bool {
	return time.Since(start) > tc.Duration
}

// RegistrationTimers returns the set of timers used during the UE registration
// procedure (TS 24.501 Section 8.2).
func RegistrationTimers() []TimerConfig {
	return []TimerConfig{
		NewTimerConfigWithRetries("T3510", T3510Value, 4),    // Registration guard (UE side)
		NewTimerConfig("T3512", T3512Default),                  // Periodic registration update
		NewTimerConfigWithRetries("T3550", T3550Value, 4),    // Registration Accept retransmission (NW side)
	}
}

// PDUSessionTimers returns the set of timers used during PDU session procedures
// (TS 24.501 Section 8.3).
func PDUSessionTimers() []TimerConfig {
	return []TimerConfig{
		NewTimerConfigWithRetries("T3580", T3580Value, 4), // PDU session establishment guard
	}
}
