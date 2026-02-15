package pfcp

import (
	"sync"
	"testing"

	"github.com/wmnsk/go-pfcp/message"
)

// mockSender captures sent bytes for verification.
type mockSender struct {
	mu       sync.Mutex
	messages [][]byte
	closed   bool
}

func (m *mockSender) Send(b []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(b))
	copy(cp, b)
	m.messages = append(m.messages, cp)
	return nil
}

func (m *mockSender) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func TestBuildEstablishSession(t *testing.T) {
	msg := BuildEstablishSession(0x1234, "10.0.0.1", 100, 1)

	if msg.SEID() != 0x1234 {
		t.Errorf("SEID = %d, want 0x1234", msg.SEID())
	}
	if msg.Sequence() != 1 {
		t.Errorf("Sequence = %d, want 1", msg.Sequence())
	}

	// Verify CreatePDR IEs
	if len(msg.CreatePDR) == 0 {
		t.Fatal("message missing CreatePDR")
	}
	pdr := msg.CreatePDR[0]
	pdrs, err := pdr.CreatePDR()
	if err != nil {
		t.Fatalf("parse CreatePDR: %v", err)
	}
	var hasPDRID, hasPDI, hasPrecedence bool
	for _, child := range pdrs {
		switch {
		case child.Type == 56: // PDRID type
			hasPDRID = true
			id, err := child.PDRID()
			if err != nil {
				t.Fatalf("parse PDRID: %v", err)
			}
			if id != 1 {
				t.Errorf("PDRID = %d, want 1", id)
			}
		case child.Type == 29: // Precedence type
			hasPrecedence = true
			prec, err := child.Precedence()
			if err != nil {
				t.Fatalf("parse Precedence: %v", err)
			}
			if prec != 100 {
				t.Errorf("Precedence = %d, want 100", prec)
			}
		case child.Type == 2: // PDI type
			hasPDI = true
		}
	}
	if !hasPDRID {
		t.Error("CreatePDR missing PDRID")
	}
	if !hasPDI {
		t.Error("CreatePDR missing PDI")
	}
	if !hasPrecedence {
		t.Error("CreatePDR missing Precedence")
	}

	// Verify CreateFAR IEs
	if len(msg.CreateFAR) == 0 {
		t.Fatal("message missing CreateFAR")
	}
	far := msg.CreateFAR[0]
	fars, err := far.CreateFAR()
	if err != nil {
		t.Fatalf("parse CreateFAR: %v", err)
	}
	var hasFARID bool
	for _, child := range fars {
		if child.Type == 108 { // FARID type
			hasFARID = true
			id, err := child.FARID()
			if err != nil {
				t.Fatalf("parse FARID: %v", err)
			}
			if id != 1 {
				t.Errorf("FARID = %d, want 1", id)
			}
		}
	}
	if !hasFARID {
		t.Error("CreateFAR missing FARID")
	}

	// Verify message can be marshalled
	b := make([]byte, msg.MarshalLen())
	if err := msg.MarshalTo(b); err != nil {
		t.Fatalf("MarshalTo: %v", err)
	}
	if len(b) == 0 {
		t.Error("marshalled bytes should not be empty")
	}
}

func TestBuildModifySession(t *testing.T) {
	params := ModifyParams{AMBRUL: 1000000, AMBRDL: 2000000, QFI: 9}
	msg := BuildModifySession(0x5678, params, 2)

	if msg.SEID() != 0x5678 {
		t.Errorf("SEID = %d, want 0x5678", msg.SEID())
	}
	if msg.Sequence() != 2 {
		t.Errorf("Sequence = %d, want 2", msg.Sequence())
	}

	if len(msg.UpdatePDR) == 0 {
		t.Error("message missing UpdatePDR")
	}
	if len(msg.UpdateFAR) == 0 {
		t.Error("message missing UpdateFAR")
	}

	b := make([]byte, msg.MarshalLen())
	if err := msg.MarshalTo(b); err != nil {
		t.Fatalf("MarshalTo: %v", err)
	}
}

func TestBuildDeleteSession(t *testing.T) {
	msg := BuildDeleteSession(0xABCD, 3)

	if msg.SEID() != 0xABCD {
		t.Errorf("SEID = %d, want 0xABCD", msg.SEID())
	}
	if msg.Sequence() != 3 {
		t.Errorf("Sequence = %d, want 3", msg.Sequence())
	}

	b := make([]byte, msg.MarshalLen())
	if err := msg.MarshalTo(b); err != nil {
		t.Fatalf("MarshalTo: %v", err)
	}
}

func TestClient_EstablishSession(t *testing.T) {
	mock := &mockSender{}
	client := NewClient(mock)

	if err := client.EstablishSession(0x1234, "10.0.0.1", 100); err != nil {
		t.Fatalf("EstablishSession: %v", err)
	}

	if len(mock.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mock.messages))
	}

	parsed, err := message.Parse(mock.messages[0])
	if err != nil {
		t.Fatalf("parse sent message: %v", err)
	}
	if parsed.MessageType() != message.MsgTypeSessionEstablishmentRequest {
		t.Errorf("type = %d, want SessionEstablishmentRequest", parsed.MessageType())
	}
}

func TestClient_ModifySession(t *testing.T) {
	mock := &mockSender{}
	client := NewClient(mock)

	params := ModifyParams{AMBRUL: 500000, AMBRDL: 1000000, QFI: 5}
	if err := client.ModifySession(0x5678, params); err != nil {
		t.Fatalf("ModifySession: %v", err)
	}

	if len(mock.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mock.messages))
	}

	parsed, err := message.Parse(mock.messages[0])
	if err != nil {
		t.Fatalf("parse sent message: %v", err)
	}
	if parsed.MessageType() != message.MsgTypeSessionModificationRequest {
		t.Errorf("type = %d, want SessionModificationRequest", parsed.MessageType())
	}
}

func TestClient_DeleteSession(t *testing.T) {
	mock := &mockSender{}
	client := NewClient(mock)

	if err := client.DeleteSession(0xABCD); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if len(mock.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mock.messages))
	}

	parsed, err := message.Parse(mock.messages[0])
	if err != nil {
		t.Fatalf("parse sent message: %v", err)
	}
	if parsed.MessageType() != message.MsgTypeSessionDeletionRequest {
		t.Errorf("type = %d, want SessionDeletionRequest", parsed.MessageType())
	}
}

func TestClient_SequenceIncrements(t *testing.T) {
	mock := &mockSender{}
	client := NewClient(mock)

	client.EstablishSession(1, "10.0.0.1", 1)
	client.EstablishSession(2, "10.0.0.2", 2)
	client.EstablishSession(3, "10.0.0.3", 3)

	if len(mock.messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(mock.messages))
	}

	for i, raw := range mock.messages {
		parsed, err := message.Parse(raw)
		if err != nil {
			t.Fatalf("parse message %d: %v", i, err)
		}
		expectedSeq := uint32(i + 1)
		if parsed.Sequence() != expectedSeq {
			t.Errorf("message %d: seq = %d, want %d", i, parsed.Sequence(), expectedSeq)
		}
	}
}

func TestClient_Close(t *testing.T) {
	mock := &mockSender{}
	client := NewClient(mock)

	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !mock.closed {
		t.Error("sender was not closed")
	}
}
