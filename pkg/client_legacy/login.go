//go:build legacy
// +build legacy

package client

import (
	"bytes"
	"fmt"

	"github.com/user/feastgo/pkg/protocol"
)

func (c *Client) Login(username string) error {
	// 1. Handshake (Next State = 2 for Login)
	err := c.Handshake(765, "localhost", 25565, 2)
	if err != nil {
		return err
	}
	c.State = StateLogin

	// 2. Login Start
	var loginStartBuf []byte
	w := &packetWriter{data: &loginStartBuf}
	protocol.WriteString(w, username)
	// 1.20.4 also sends UUID (optional/fake for offline)
	// But let's check: Login Start 0x00 has Name (String) and Player UUID (UUID)
	// Actually 1.19.3+ Login Start has Player UUID as well.
	// For offline, we can send a random UUID or all zeros.
	fakeUUID := make([]byte, 16)
	w.Write(fakeUUID)

	err = c.SendPacket(&protocol.Packet{ID: 0x00, Data: loginStartBuf})
	if err != nil {
		return err
	}

	for {
		p, err := c.ReadPacket()
		if err != nil {
			return err
		}

		switch p.ID {
		case 0x00: // Disconnect
			reason, _ := protocol.ReadString(bytes.NewReader(p.Data))
			return fmt.Errorf("login disconnected: %s", reason)
		case 0x01: // Encryption Request
			return fmt.Errorf("server is in online mode (not supported yet)")
		case 0x02: // Login Success
			// Transitions to Configuration state
			err = c.handleLoginSuccess(p)
			if err != nil {
				return err
			}
			return c.handleConfiguration()
		case 0x03: // Set Compression
			threshold, _, err := protocol.ReadVarInt(bytes.NewReader(p.Data))
			if err != nil {
				return err
			}
			c.CompressionThreshold = int(threshold)
		case 0x04: // Login Plugin Request
			// Just ignore or send empty response for now
			queryID, _, _ := protocol.ReadVarInt(bytes.NewReader(p.Data))
			var respBuf []byte
			rw := &packetWriter{data: &respBuf}
			protocol.WriteVarInt(rw, queryID)
			rw.Write([]byte{0x00}) // false for successful

			c.SendPacket(&protocol.Packet{ID: 0x02, Data: respBuf})
		default:
			fmt.Printf("Unknown login packet ID: %x\n", p.ID)
		}
	}
}

func (c *Client) handleLoginSuccess(p *protocol.Packet) error {
	r := bytes.NewReader(p.Data)

	// UUID (16 bytes)
	c.UUID = make([]byte, 16)
	_, err := r.Read(c.UUID)
	if err != nil {
		return err
	}
	fmt.Printf("Logged in with UUID: %x\n", c.UUID)

	// Username (String)
	_, err = protocol.ReadString(r)
	if err != nil {
		return err
	}

	// Property Count (VarInt)
	propCount, _, err := protocol.ReadVarInt(r)
	if err != nil {
		return err
	}

	for i := 0; i < int(propCount); i++ {
		// Name (String)
		protocol.ReadString(r)
		// Value (String)
		protocol.ReadString(r)
		// Is Signed (Boolean)
		isSigned, _ := protocol.ReadBoolean(r)
		if isSigned {
			// Signature (String)
			protocol.ReadString(r)
		}
	}

	// Strict Error Handling (Boolean) - 1.20.4
	protocol.ReadBoolean(r)

	// Send Login Acknowledged (0x03)
	fmt.Println(">>> Sending Login Acknowledged (0x03)")
	err = c.SendPacket(&protocol.Packet{ID: 0x03, Data: []byte{}})
	if err != nil {
		return err
	}

	c.State = StateConfiguration
	return nil
}
