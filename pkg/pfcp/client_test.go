package pfcp

import (
	"sync"
	"testing"

	"github.com/wmnsk/go-pfcp/message"
)

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

func TestBuildEstablishSession_IEs(t *testing.T) {
	p := SessionParams{
		SEID:       0x1234,
		UEIP:       "10.0.0.1",
		TEID:       100,
		NodeID:     "192.168.1.1",
		AMBRUL:     1000000,
		AMBRDL:     5000000,
		QFI:        9,
		NetworkDNN: "internet",
	}
	msg := BuildEstablishSession(p, 1)

	if msg.SEID() != 0x1234 {
		t.Errorf("SEID = %d, want 0x1234", msg.SEID())
	}
	if msg.Sequence() != 1 {
		t.Errorf("Sequence = %d, want 1", msg.Sequence())
	}

	// TS 29.244: Verify Node ID
	if msg.NodeID == nil {
		t.Error("message missing NodeID IE")
	}

	// TS 29.244: Verify F-SEID (CP function SEID)
	if msg.CPFSEID == nil {
		t.Error("message missing CPFSEID (F-SEID) IE")
	} else {
		fseid, err := msg.CPFSEID.FSEID()
		if err != nil {
			t.Fatalf("parse F-SEID: %v", err)
		}
		if fseid.SEID != 0x1234 {
			t.Errorf("F-SEID = %d, want 0x1234", fseid.SEID)
		}
	}

	// TS 29.244 Section 7.5.2.3: Verify CreatePDR
	if len(msg.CreatePDR) == 0 {
		t.Fatal("message missing CreatePDR")
	}
	pdrs, err := msg.CreatePDR[0].CreatePDR()
	if err != nil {
		t.Fatalf("parse CreatePDR: %v", err)
	}
	var hasPDRID, hasPDI, hasPrecedence bool
	for _, child := range pdrs {
		switch {
		case child.Type == 56: // PDRID
			hasPDRID = true
			id, err := child.PDRID()
			if err != nil {
				t.Fatalf("parse PDRID: %v", err)
			}
			if id != 1 {
				t.Errorf("PDRID = %d, want 1", id)
			}
		case child.Type == 29: // Precedence
			hasPrecedence = true
			prec, err := child.Precedence()
			if err != nil {
				t.Fatalf("parse Precedence: %v", err)
			}
			if prec != 100 {
				t.Errorf("Precedence = %d, want 100", prec)
			}
		case child.Type == 2: // PDI
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

	// TS 29.244 Section 7.5.2.4: Verify CreateFAR
	if len(msg.CreateFAR) == 0 {
		t.Fatal("message missing CreateFAR")
	}
	fars, err := msg.CreateFAR[0].CreateFAR()
	if err != nil {
		t.Fatalf("parse CreateFAR: %v", err)
	}
	var hasFARID bool
	for _, child := range fars {
		if child.Type == 108 { // FARID
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

	// TS 29.244 Section 7.5.2.6: Verify CreateQER
	if len(msg.CreateQER) == 0 {
		t.Fatal("message missing CreateQER")
	}

	// TS 29.244 Section 7.5.2.5: Verify CreateURR
	if len(msg.CreateURR) == 0 {
		t.Fatal("message missing CreateURR")
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

func TestBuildEstablishSession_Defaults(t *testing.T) {
	// Verify defaults when NodeID and DNN are empty
	p := SessionParams{SEID: 1, UEIP: "10.0.0.1", TEID: 1}
	msg := BuildEstablishSession(p, 1)

	if msg.NodeID == nil {
		t.Error("should have default NodeID")
	}
	if msg.CPFSEID == nil {
		t.Error("should have default F-SEID")
	}

	b := make([]byte, msg.MarshalLen())
	if err := msg.MarshalTo(b); err != nil {
		t.Fatalf("MarshalTo with defaults: %v", err)
	}
}

func TestBuildModifySession(t *testing.T) {
	params := ModifyParams{AMBRUL: 2000000, AMBRDL: 10000000, QFI: 5}
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
	// When AMBR is provided, UpdateQER should be present
	if len(msg.UpdateQER) == 0 {
		t.Error("message missing UpdateQER (AMBR was provided)")
	}

	b := make([]byte, msg.MarshalLen())
	if err := msg.MarshalTo(b); err != nil {
		t.Fatalf("MarshalTo: %v", err)
	}
}

func TestBuildModifySession_NoQERUpdate(t *testing.T) {
	// When AMBR is 0, no UpdateQER should be included
	params := ModifyParams{QFI: 5}
	msg := BuildModifySession(0x1, params, 1)

	if len(msg.UpdateQER) != 0 {
		t.Error("UpdateQER should not be present when AMBR is 0")
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

func TestClient_EstablishSessionWithParams(t *testing.T) {
	mock := &mockSender{}
	client := NewClient(mock)

	p := SessionParams{
		SEID:       42,
		UEIP:       "10.45.0.5",
		TEID:       200,
		NodeID:     "192.168.1.100",
		AMBRUL:     2000000,
		AMBRDL:     8000000,
		NetworkDNN: "enterprise",
	}
	if err := client.EstablishSessionWithParams(p); err != nil {
		t.Fatalf("EstablishSessionWithParams: %v", err)
	}

	if len(mock.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mock.messages))
	}
	parsed, err := message.Parse(mock.messages[0])
	if err != nil {
		t.Fatalf("parse: %v", err)
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
		t.Fatalf("parse: %v", err)
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
		t.Fatalf("parse: %v", err)
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
