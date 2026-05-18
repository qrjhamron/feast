//go:build legacy
// +build legacy

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/user/feastgo/pkg/protocol"
)

type StatusResponse struct {
	Version struct {
		Name     string `json:"name"`
		Protocol int    `json:"protocol"`
	} `json:"version"`
	Players struct {
		Max    int `json:"max"`
		Online int `json:"online"`
		Sample []struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"sample"`
	} `json:"players"`
	Description interface{} `json:"description"`
	Favicon     string      `json:"favicon"`
}

func (c *Client) Ping(host string, port uint16) (*StatusResponse, time.Duration, error) {
	// 1. Handshake (State 1 = Status)
	err := c.Handshake(765, host, port, 1)
	if err != nil {
		return nil, 0, fmt.Errorf("handshake failed: %v", err)
	}

	// 2. Status Request
	err = c.SendPacket(&protocol.Packet{ID: 0x00, Data: []byte{}})
	if err != nil {
		return nil, 0, fmt.Errorf("status request failed: %v", err)
	}

	// 3. Status Response
	respPacket, err := c.ReadPacket()
	if err != nil {
		return nil, 0, fmt.Errorf("read status response failed: %v", err)
	}
	if respPacket.ID != 0x00 {
		return nil, 0, fmt.Errorf("unexpected packet ID: %d", respPacket.ID)
	}

	jsonStr, err := protocol.ReadString(bytes.NewReader(respPacket.Data))
	if err != nil {
		return nil, 0, fmt.Errorf("decode status json failed: %v", err)
	}

	var status StatusResponse
	err = json.Unmarshal([]byte(jsonStr), &status)
	if err != nil {
		return nil, 0, fmt.Errorf("unmarshal status json failed: %v", err)
	}

	// 4. Ping Request
	startTime := time.Now()
	payload := startTime.UnixNano() / int64(time.Millisecond)
	var pingBuf []byte
	pw := &packetWriter{data: &pingBuf}
	protocol.WriteInt64(pw, payload)

	err = c.SendPacket(&protocol.Packet{ID: 0x01, Data: pingBuf})
	if err != nil {
		return &status, 0, fmt.Errorf("ping request failed: %v", err)
	}

	// 5. Ping Response
	pongPacket, err := c.ReadPacket()
	if err != nil {
		return &status, 0, fmt.Errorf("read ping response failed: %v", err)
	}
	if pongPacket.ID != 0x01 {
		return &status, 0, fmt.Errorf("unexpected pong packet ID: %d", pongPacket.ID)
	}

	latency := time.Since(startTime)

	return &status, latency, nil
}
