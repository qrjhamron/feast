//go:build legacy
// +build legacy

package client

import (
	"bytes"
	"fmt"

	"github.com/user/feastgo/pkg/protocol"
)

func (c *Client) handleConfiguration() error {
	for {
		p, err := c.ReadPacket()
		if err != nil {
			return err
		}

		switch p.ID {
		case 0x01: // Disconnect
			reason, _ := protocol.ReadString(bytes.NewReader(p.Data))
			return fmt.Errorf("config disconnected: %s", reason)
		case 0x02: // Finish Configuration
			// Respond with Acknowledge Finish Configuration (Serverbound 0x02)
			fmt.Println(">>> Sending Acknowledge Finish Configuration (0x02)")
			err = c.SendPacket(&protocol.Packet{ID: 0x02, Data: []byte{}})
			if err != nil {
				return err
			}
			c.State = StatePlay
			fmt.Println("Successfully transitioned to Play state!")
			return nil
		case 0x03: // Keep Alive (Clientbound)
			// Respond with Keep Alive Response (Serverbound 0x03)
			fmt.Println(">>> Responding to Configuration Keep Alive (0x03)")
			err = c.SendPacket(&protocol.Packet{ID: 0x03, Data: p.Data})
			if err != nil {
				return err
			}
		case 0x04: // Ping (Clientbound)
			// Respond with Pong (Serverbound 0x04)
			fmt.Println(">>> Responding to Configuration Ping (0x04)")
			err = c.SendPacket(&protocol.Packet{ID: 0x04, Data: p.Data})
			if err != nil {
				return err
			}
		default:
			// Ignore other configuration packets
			// fmt.Printf("Ignoring config packet 0x%02X\n", p.ID)
		}
	}
}
