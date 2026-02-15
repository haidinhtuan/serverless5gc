package pfcp

import (
	"fmt"
	"net"
	"sync/atomic"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// ModifyParams holds parameters for modifying a PFCP session.
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

// Client communicates with the UPF via PFCP.
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

// BuildEstablishSession constructs a PFCP Session Establishment Request.
func BuildEstablishSession(seid uint64, ueIP string, teid uint32, seq uint32) *message.SessionEstablishmentRequest {
	return message.NewSessionEstablishmentRequest(
		0, 0,
		seid,
		seq,
		0,
		ie.NewCreatePDR(
			ie.NewPDRID(1),
			ie.NewPrecedence(100),
			ie.NewPDI(
				ie.NewSourceInterface(ie.SrcInterfaceAccess),
				ie.NewFTEID(0x01, teid, net.ParseIP(ueIP), nil, 0),
			),
		),
		ie.NewCreateFAR(
			ie.NewFARID(1),
			ie.NewApplyAction(0x02), // FORW (forward)
			ie.NewForwardingParameters(
				ie.NewDestinationInterface(ie.DstInterfaceCore),
			),
		),
	)
}

// EstablishSession sends a PFCP Session Establishment Request to the UPF.
func (c *Client) EstablishSession(seid uint64, ueIP string, teid uint32) error {
	msg := BuildEstablishSession(seid, ueIP, teid, c.nextSeq())
	b := make([]byte, msg.MarshalLen())
	if err := msg.MarshalTo(b); err != nil {
		return fmt.Errorf("marshal session establishment: %w", err)
	}
	return c.sender.Send(b)
}

// BuildModifySession constructs a PFCP Session Modification Request.
func BuildModifySession(seid uint64, params ModifyParams, seq uint32) *message.SessionModificationRequest {
	return message.NewSessionModificationRequest(
		0, 0,
		seid,
		seq,
		0,
		ie.NewUpdatePDR(
			ie.NewPDRID(1),
			ie.NewPrecedence(100),
		),
		ie.NewUpdateFAR(
			ie.NewFARID(1),
			ie.NewApplyAction(0x02),
		),
	)
}

// ModifySession sends a PFCP Session Modification Request to the UPF.
func (c *Client) ModifySession(seid uint64, params ModifyParams) error {
	msg := BuildModifySession(seid, params, c.nextSeq())
	b := make([]byte, msg.MarshalLen())
	if err := msg.MarshalTo(b); err != nil {
		return fmt.Errorf("marshal session modification: %w", err)
	}
	return c.sender.Send(b)
}

// BuildDeleteSession constructs a PFCP Session Deletion Request.
func BuildDeleteSession(seid uint64, seq uint32) *message.SessionDeletionRequest {
	return message.NewSessionDeletionRequest(
		0, 0,
		seid,
		seq,
		0,
	)
}

// DeleteSession sends a PFCP Session Deletion Request to the UPF.
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
