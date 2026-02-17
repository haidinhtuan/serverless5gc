package ngap

import (
	"testing"

	"github.com/free5gc/ngap/ngapType"
)

func TestBuildAndParseNGSetupResponse(t *testing.T) {
	plmn := PLMNBytes("001", "01")
	data, err := BuildNGSetupResponse(plmn, 0x01, []byte{0x01, 0x02, 0x03})
	if err != nil {
		t.Fatalf("BuildNGSetupResponse: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty NGSetupResponse")
	}

	ctx, err := ParseNGAPMessage(data)
	if err != nil {
		t.Fatalf("ParseNGAPMessage: %v", err)
	}
	if ctx.ProcedureCode != ngapType.ProcedureCodeNGSetup {
		t.Errorf("ProcedureCode = %d, want %d", ctx.ProcedureCode, ngapType.ProcedureCodeNGSetup)
	}
	if ctx.MessageType != 1 {
		t.Errorf("MessageType = %d, want 1 (successful)", ctx.MessageType)
	}
}

func TestBuildAndParseDownlinkNASTransport(t *testing.T) {
	nasPDU := []byte{0x7E, 0x00, 0x42, 0x01}
	data, err := BuildDownlinkNASTransport(100, 42, nasPDU)
	if err != nil {
		t.Fatalf("BuildDownlinkNASTransport: %v", err)
	}

	ctx, err := ParseNGAPMessage(data)
	if err != nil {
		t.Fatalf("ParseNGAPMessage: %v", err)
	}
	if ctx.ProcedureCode != ngapType.ProcedureCodeDownlinkNASTransport {
		t.Errorf("ProcedureCode = %d, want %d", ctx.ProcedureCode, ngapType.ProcedureCodeDownlinkNASTransport)
	}
	if ctx.AMFUeNgapID != 100 {
		t.Errorf("AMFUeNgapID = %d, want 100", ctx.AMFUeNgapID)
	}
	if ctx.RANUeNgapID != 42 {
		t.Errorf("RANUeNgapID = %d, want 42", ctx.RANUeNgapID)
	}
	if len(ctx.NASPDU) != len(nasPDU) {
		t.Errorf("NASPDU len = %d, want %d", len(ctx.NASPDU), len(nasPDU))
	}
}

func TestBuildInitialContextSetupRequest(t *testing.T) {
	plmn := PLMNBytes("001", "01")
	nasPDU := []byte{0x7E, 0x00, 0x42, 0x01}
	data, err := BuildInitialContextSetupRequest(100, 42, nasPDU, plmn, 0x01, []byte{0x01, 0x02, 0x03})
	if err != nil {
		t.Fatalf("BuildInitialContextSetupRequest: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty InitialContextSetupRequest")
	}

	ctx, err := ParseNGAPMessage(data)
	if err != nil {
		t.Fatalf("ParseNGAPMessage: %v", err)
	}
	if ctx.ProcedureCode != ngapType.ProcedureCodeInitialContextSetup {
		t.Errorf("ProcedureCode = %d, want %d", ctx.ProcedureCode, ngapType.ProcedureCodeInitialContextSetup)
	}
}

func TestPLMNBytes(t *testing.T) {
	tests := []struct {
		mcc, mnc string
		want     []byte
	}{
		{"001", "01", []byte{0x00, 0xF1, 0x10}},
		{"310", "41", []byte{0x13, 0xF0, 0x14}},
		{"310", "410", []byte{0x13, 0x40, 0x01}},
	}
	for _, tt := range tests {
		got := PLMNBytes(tt.mcc, tt.mnc)
		if len(got) != 3 || got[0] != tt.want[0] || got[1] != tt.want[1] || got[2] != tt.want[2] {
			t.Errorf("PLMNBytes(%q, %q) = %x, want %x", tt.mcc, tt.mnc, got, tt.want)
		}
	}
}
