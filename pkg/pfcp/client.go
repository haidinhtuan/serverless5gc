// Package pfcp implements PFCP protocol communication between SMF and UPF
// per 3GPP TS 29.244.
package pfcp

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// SessionParams holds all parameters needed for PFCP session establishment
// per TS 29.244 Section 7.5.2.
type SessionParams struct {
	SEID       uint64
	UEIP       string
	TEID       uint32
	NodeID     string // SMF PFCP node IP (F-SEID)
	AMBRUL     uint64 // Session-AMBR UL in kbps
	AMBRDL     uint64 // Session-AMBR DL in kbps
	QFI        uint8
	NetworkDNN string // Data Network Name (network instance)
}

// ModifyParams holds parameters for PFCP session modification
// per TS 29.244 Section 7.5.4.
type ModifyParams struct {
	AMBRUL uint64
	AMBRDL uint64
	QFI    uint8
}

// Sender abstracts the transport for PFCP messages (UDP in production, mock in tests).
type Sender interface {
	Send(b []byte) error
	Close() error
}

// UDPSender sends PFCP messages over UDP.
type UDPSender struct {
	conn *net.UDPConn
}

// NewUDPSender dials the given UPF address and returns a UDPSender.
func NewUDPSender(upfAddr string) (*UDPSender, error) {
	addr, err := net.ResolveUDPAddr("udp", upfAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve UPF addr: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("dial UPF: %w", err)
	}
	return &UDPSender{conn: conn}, nil
}

func (u *UDPSender) Send(b []byte) error {
	_, err := u.conn.Write(b)
	return err
}

func (u *UDPSender) Close() error {
	return u.conn.Close()
}

// Client communicates with the UPF via PFCP (TS 29.244).
type Client struct {
	sender Sender
	seq    uint32
}

// NewClient creates a PFCP client that sends messages via the provided sender.
func NewClient(sender Sender) *Client {
	return &Client{sender: sender}
}

// NewUDPClient creates a PFCP client that communicates with the UPF over UDP.
func NewUDPClient(upfAddr string) (*Client, error) {
	sender, err := NewUDPSender(upfAddr)
	if err != nil {
		return nil, err
	}
	return &Client{sender: sender}, nil
}

func (c *Client) nextSeq() uint32 {
	return atomic.AddUint32(&c.seq, 1)
}

// BuildAssociationSetupRequest constructs a PFCP Association Setup Request
// per TS 29.244 Section 7.4.4.1. This is the first message exchanged between
// SMF (CP function) and UPF (UP function) to establish the PFCP association.
// IEs: Node ID, Recovery Time Stamp, CP Function Features (optional).
func BuildAssociationSetupRequest(nodeID string, seq uint32) *message.AssociationSetupRequest {
	if nodeID == "" {
		nodeID = "127.0.0.1"
	}
	return message.NewAssociationSetupRequest(
		seq,
		ie.NewNodeID(nodeID, "", ""),
		ie.NewRecoveryTimeStamp(time.Now()),
	)
}

// AssociationSetup sends a PFCP Association Setup Request to the UPF
// per TS 29.244 Section 7.4.4.
func (c *Client) AssociationSetup(nodeID string) error {
	msg := BuildAssociationSetupRequest(nodeID, c.nextSeq())
	b := make([]byte, msg.MarshalLen())
	if err := msg.MarshalTo(b); err != nil {
		return fmt.Errorf("marshal association setup: %w", err)
	}
	return c.sender.Send(b)
}

// BuildEstablishSession constructs a PFCP Session Establishment Request
// per TS 29.244 Section 7.5.2 with the following IEs:
//   - Node ID (SMF PFCP address)
//   - F-SEID (SMF-assigned SEID)
//   - Create PDR: PDR ID, precedence, PDI (source interface, F-TEID, network instance)
//   - Create FAR: FAR ID, apply action (FORW), forwarding parameters (destination interface)
//   - Create QER: QER ID, gate status (open), MBR (UL/DL), GBR (UL/DL)
//   - Create URR: URR ID, measurement method (volume+duration), reporting triggers
func BuildEstablishSession(p SessionParams, seq uint32) *message.SessionEstablishmentRequest {
	nodeIP := p.NodeID
	if nodeIP == "" {
		nodeIP = "127.0.0.1"
	}
	dnn := p.NetworkDNN
	if dnn == "" {
		dnn = "internet"
	}

	return message.NewSessionEstablishmentRequest(
		0, 0,
		p.SEID,
		seq,
		0,
		// TS 29.244 Section 7.5.2.1: Node ID
		ie.NewNodeID(nodeIP, "", ""),
		// TS 29.244 Section 7.5.2.2: F-SEID (CP function SEID)
		ie.NewFSEID(p.SEID, net.ParseIP(nodeIP), nil),
		// TS 29.244 Section 7.5.2.3: Create PDR
		ie.NewCreatePDR(
			ie.NewPDRID(1),
			ie.NewPrecedence(100),
			ie.NewPDI(
				ie.NewSourceInterface(ie.SrcInterfaceAccess),
				ie.NewFTEID(0x01, p.TEID, net.ParseIP(p.UEIP), nil, 0),
				ie.NewNetworkInstance(dnn),
			),
		),
		// TS 29.244 Section 7.5.2.4: Create FAR
		ie.NewCreateFAR(
			ie.NewFARID(1),
			ie.NewApplyAction(0x02), // FORW (forward)
			ie.NewForwardingParameters(
				ie.NewDestinationInterface(ie.DstInterfaceCore),
			),
		),
		// TS 29.244 Section 7.5.2.6: Create QER (QoS Enforcement Rule)
		ie.NewCreateQER(
			ie.NewQERID(1),
			ie.NewGateStatus(ie.GateStatusOpen, ie.GateStatusOpen),
			ie.NewMBR(p.AMBRUL, p.AMBRDL),
			ie.NewGBR(0, 0), // no guaranteed bit rate for default bearer
		),
		// TS 29.244 Section 7.5.2.5: Create URR (Usage Reporting Rule)
		// Used for cost metrics collection in evaluation
		ie.NewCreateURR(
			ie.NewURRID(1),
			ie.NewMeasurementMethod(0, 1, 1), // volume + duration measurement
			ie.NewReportingTriggers(0x01),     // periodic reporting
		),
	)
}

// EstablishSession sends a PFCP Session Establishment Request to the UPF
// per TS 29.244 Section 7.5.2.
func (c *Client) EstablishSession(seid uint64, ueIP string, teid uint32) error {
	p := SessionParams{
		SEID:   seid,
		UEIP:   ueIP,
		TEID:   teid,
		AMBRUL: 1000000, // default 1 Mbps
		AMBRDL: 5000000, // default 5 Mbps
	}
	return c.EstablishSessionWithParams(p)
}

// EstablishSessionWithParams sends a full PFCP Session Establishment Request.
func (c *Client) EstablishSessionWithParams(p SessionParams) error {
	msg := BuildEstablishSession(p, c.nextSeq())
	b := make([]byte, msg.MarshalLen())
	if err := msg.MarshalTo(b); err != nil {
		return fmt.Errorf("marshal session establishment: %w", err)
	}
	return c.sender.Send(b)
}

// BuildModifySession constructs a PFCP Session Modification Request
// per TS 29.244 Section 7.5.4.
func BuildModifySession(seid uint64, params ModifyParams, seq uint32) *message.SessionModificationRequest {
	ies := []*ie.IE{
		ie.NewUpdatePDR(
			ie.NewPDRID(1),
			ie.NewPrecedence(100),
		),
		ie.NewUpdateFAR(
			ie.NewFARID(1),
			ie.NewApplyAction(0x02), // FORW
		),
	}

	// Update QER with new AMBR if provided
	if params.AMBRUL > 0 || params.AMBRDL > 0 {
		ies = append(ies, ie.NewUpdateQER(
			ie.NewQERID(1),
			ie.NewGateStatus(ie.GateStatusOpen, ie.GateStatusOpen),
			ie.NewMBR(params.AMBRUL, params.AMBRDL),
		))
	}

	return message.NewSessionModificationRequest(
		0, 0,
		seid,
		seq,
		0,
		ies...,
	)
}

// ModifySession sends a PFCP Session Modification Request to the UPF
// per TS 29.244 Section 7.5.4.
func (c *Client) ModifySession(seid uint64, params ModifyParams) error {
	msg := BuildModifySession(seid, params, c.nextSeq())
	b := make([]byte, msg.MarshalLen())
	if err := msg.MarshalTo(b); err != nil {
		return fmt.Errorf("marshal session modification: %w", err)
	}
	return c.sender.Send(b)
}

// BuildDeleteSession constructs a PFCP Session Deletion Request
// per TS 29.244 Section 7.5.6.
func BuildDeleteSession(seid uint64, seq uint32) *message.SessionDeletionRequest {
	return message.NewSessionDeletionRequest(
		0, 0,
		seid,
		seq,
		0,
	)
}

// DeleteSession sends a PFCP Session Deletion Request to the UPF
// per TS 29.244 Section 7.5.6.
func (c *Client) DeleteSession(seid uint64) error {
	msg := BuildDeleteSession(seid, c.nextSeq())
	b := make([]byte, msg.MarshalLen())
	if err := msg.MarshalTo(b); err != nil {
		return fmt.Errorf("marshal session deletion: %w", err)
	}
	return c.sender.Send(b)
}

// Close releases the underlying transport resources.
func (c *Client) Close() error {
	if c.sender != nil {
		return c.sender.Close()
	}
	return nil
}
