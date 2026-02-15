package nas

import (
	"testing"
	"time"
)

func TestTimerConstants(t *testing.T) {
	// T3512: 54 minutes periodic registration update (TS 24.501 Section 8.2.7.17)
	if T3512Default != 3240 {
		t.Errorf("T3512Default = %d, want 3240 (54 minutes)", T3512Default)
	}
	// T3510: 15 seconds registration guard timer (TS 24.501 Section 8.2.6.2)
	if T3510Value != 15 {
		t.Errorf("T3510Value = %d, want 15", T3510Value)
	}
	// T3502: 12 minutes registration retry timer
	if T3502Default != 720 {
		t.Errorf("T3502Default = %d, want 720 (12 minutes)", T3502Default)
	}
	// T3580: 5 seconds PDU session establishment guard timer (TS 24.501 Section 8.3.1)
	if T3580Value != 5 {
		t.Errorf("T3580Value = %d, want 5", T3580Value)
	}
	// T3513: 10 seconds paging timer (TS 24.501 Section 8.2.3.4)
	if T3513Value != 10 {
		t.Errorf("T3513Value = %d, want 10", T3513Value)
	}
	// T3522: 6 seconds deregistration timer (TS 24.501 Section 8.2.12)
	if T3522Value != 6 {
		t.Errorf("T3522Value = %d, want 6", T3522Value)
	}
	// T3550: 6 seconds registration accept retransmission (TS 24.501 Section 8.2.7.4)
	if T3550Value != 6 {
		t.Errorf("T3550Value = %d, want 6", T3550Value)
	}
	// T3560: 6 seconds security mode command retransmission (TS 24.501 Section 8.2.25.4)
	if T3560Value != 6 {
		t.Errorf("T3560Value = %d, want 6", T3560Value)
	}
}

func TestNewTimerConfig(t *testing.T) {
	cfg := NewTimerConfig("T3512", T3512Default)
	if cfg.Name != "T3512" {
		t.Errorf("Name = %q, want T3512", cfg.Name)
	}
	if cfg.Duration != time.Duration(T3512Default)*time.Second {
		t.Errorf("Duration = %v, want %v", cfg.Duration, time.Duration(T3512Default)*time.Second)
	}
	if cfg.MaxRetries != 0 {
		t.Errorf("MaxRetries = %d, want 0 (no retries by default)", cfg.MaxRetries)
	}
}

func TestNewTimerConfigWithRetries(t *testing.T) {
	cfg := NewTimerConfigWithRetries("T3510", T3510Value, 4)
	if cfg.Name != "T3510" {
		t.Errorf("Name = %q, want T3510", cfg.Name)
	}
	if cfg.Duration != time.Duration(T3510Value)*time.Second {
		t.Errorf("Duration = %v, want %v", cfg.Duration, time.Duration(T3510Value)*time.Second)
	}
	if cfg.MaxRetries != 4 {
		t.Errorf("MaxRetries = %d, want 4", cfg.MaxRetries)
	}
}

func TestTimerIsExpired(t *testing.T) {
	cfg := NewTimerConfig("T3510", T3510Value)

	// A start time well in the past should be expired
	past := time.Now().Add(-time.Duration(T3510Value+1) * time.Second)
	if !cfg.IsExpired(past) {
		t.Error("expected timer started 16s ago with 15s duration to be expired")
	}

	// A start time just now should not be expired
	now := time.Now()
	if cfg.IsExpired(now) {
		t.Error("expected timer started just now to not be expired")
	}
}

func TestRegistrationTimers(t *testing.T) {
	timers := RegistrationTimers()
	// Should contain T3510, T3512, T3550
	expected := map[string]bool{"T3510": false, "T3512": false, "T3550": false}
	for _, tc := range timers {
		if _, ok := expected[tc.Name]; ok {
			expected[tc.Name] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("RegistrationTimers() missing %s", name)
		}
	}
}

func TestPDUSessionTimers(t *testing.T) {
	timers := PDUSessionTimers()
	found := false
	for _, tc := range timers {
		if tc.Name == "T3580" {
			found = true
			if tc.Duration != 5*time.Second {
				t.Errorf("T3580 duration = %v, want 5s", tc.Duration)
			}
		}
	}
	if !found {
		t.Error("PDUSessionTimers() missing T3580")
	}
}
