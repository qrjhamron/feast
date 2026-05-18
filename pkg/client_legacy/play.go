//go:build legacy
// +build legacy

package client

import (
	"bytes"
	"fmt"
	"time"

	"github.com/user/feastgo/pkg/protocol"
)

func (c *Client) HandlePlay() error {
	// 1. Send Client Information (Serverbound 0x09 in 1.20.4?)
	// Wait, search result said 0x09 or 0x07. Wiki.vg says 0x09 for 1.20.4 Play.
	err := c.SendClientInfo()
	if err != nil {
		return err
	}

	// Start position update ticker
	go c.startPositionTicker()

	for {
		p, err := c.ReadPacket()
		if err != nil {
			return err
		}

		switch p.ID {
		case 0x24: // Keep Alive (Clientbound)
			// Respond with Keep Alive (Serverbound 0x15)
			err = c.SendPacket(&protocol.Packet{ID: 0x15, Data: p.Data})
			if err != nil {
				return err
			}
		case 0x3E: // Synchronize Player Position (Clientbound)
			err = c.handleSynchronizePosition(p)
			if err != nil {
				return err
			}
		case 0x1B: // Disconnect (Play state)
			reason, _ := protocol.ReadString(bytes.NewReader(p.Data))
			return fmt.Errorf("play disconnected: %s", reason)
		case 0x37: // Player Chat Message (1.20.4)
			fmt.Printf("[Chat] Player Message received (ID 0x37)\n")
		case 0x64: // System Chat Message (1.20.4)
			r := bytes.NewReader(p.Data)
			content, _ := protocol.ReadString(r)
			overlay, _ := protocol.ReadBoolean(r)
			if !overlay {
				fmt.Printf("[Chat] %s\n", content)
			}
		default:
			// Ignore other packets
		}
	}
}

func (c *Client) SendClientInfo() error {
	var buf []byte
	w := &packetWriter{data: &buf}
	protocol.WriteString(w, "en_us")
	w.Write([]byte{10})             // View distance
	protocol.WriteVarInt(w, 0)      // Chat mode
	protocol.WriteBoolean(w, true)  // Chat colors
	w.Write([]byte{0x7F})           // Skin parts
	protocol.WriteVarInt(w, 1)      // Main hand (Right)
	protocol.WriteBoolean(w, false) // Text filtering
	protocol.WriteBoolean(w, true)  // Allow server listings

	return c.SendPacket(&protocol.Packet{ID: 0x09, Data: buf})
}

func (c *Client) SendChat(message string) error {
	var buf []byte
	w := &packetWriter{data: &buf}
	protocol.WriteString(w, message)
	protocol.WriteInt64(w, time.Now().UnixMilli())
	protocol.WriteInt64(w, 0)       // Salt
	protocol.WriteBoolean(w, false) // Has Signature
	protocol.WriteVarInt(w, 0)      // Message Count
	w.Write(make([]byte, 20))       // Acknowledged Messages (BitSet 20 bytes)

	return c.SendPacket(&protocol.Packet{ID: 0x05, Data: buf})
}

func (c *Client) handleSynchronizePosition(p *protocol.Packet) error {
	r := bytes.NewReader(p.Data)
	x, _ := protocol.ReadDouble(r)
	y, _ := protocol.ReadDouble(r)
	z, _ := protocol.ReadDouble(r)
	yaw, _ := protocol.ReadFloat(r)
	pitch, _ := protocol.ReadFloat(r)
	flags, _ := r.ReadByte()
	teleportID, _, _ := protocol.ReadVarInt(r)

	fmt.Printf("<<< Synchronize Position: (%.2f, %.2f, %.2f) TeleportID: %d\n", x, y, z, teleportID)

	// Update local position
	if flags&0x01 != 0 {
		c.X += x
	} else {
		c.X = x
	}
	if flags&0x02 != 0 {
		c.Y += y
	} else {
		c.Y = y
	}
	if flags&0x04 != 0 {
		c.Z += z
	} else {
		c.Z = z
	}
	if flags&0x08 != 0 {
		c.Yaw += yaw
	} else {
		c.Yaw = yaw
	}
	if flags&0x10 != 0 {
		c.Pitch += pitch
	} else {
		c.Pitch = pitch
	}

	// 1. Confirm Teleport (Serverbound 0x00)
	var confirmBuf []byte
	cw := &packetWriter{data: &confirmBuf}
	protocol.WriteVarInt(cw, teleportID)
	err := c.SendPacket(&protocol.Packet{ID: 0x00, Data: confirmBuf})
	if err != nil {
		return err
	}

	// 2. Send initial Position and Rotation (Serverbound 0x18)
	return c.sendPositionAndRotation()
}

func (c *Client) sendPositionAndRotation() error {
	var buf []byte
	w := &packetWriter{data: &buf}
	protocol.WriteDouble(w, c.X)
	protocol.WriteDouble(w, c.Y)
	protocol.WriteDouble(w, c.Z)
	protocol.WriteFloat(w, c.Yaw)
	protocol.WriteFloat(w, c.Pitch)
	protocol.WriteBoolean(w, c.OnGround)

	// 0x18 is Player Position and Rotation in 1.20.4
	return c.SendPacket(&protocol.Packet{ID: 0x18, Data: buf})
}

func (c *Client) startPositionTicker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if c.State != StatePlay {
			return
		}
		err := c.sendPositionAndRotation()
		if err != nil {
			fmt.Printf("Position ticker error: %v\n", err)
			return
		}
	}
}
