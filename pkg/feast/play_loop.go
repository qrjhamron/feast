package feast

import (
	"bytes"
	"fmt"
	"time"

	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/protocol/consts"
	"github.com/user/feastgo/pkg/state"
)

func (c *Client) startPlayLoops() {
	c.wg.Add(2)
	go c.readLoop()
	go c.heartbeatLoop()
}

func unmarshalRaw(pkt protocol.Packet, raw *protocol.RawPacket) error {
	return pkt.Unmarshal(protocol.NewReader(bytes.NewReader(raw.Data)))
}

func (c *Client) readLoop() {
	defer c.wg.Done()
	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		raw, err := c.readPacket()
		if err != nil {
			if c.closing() {
				return
			}
			classification := classifyReadError(err)
			wrapped := fmt.Errorf("%s: %w", classification, err)
			c.bus.Emit(state.ErrorEvent{Op: "read_loop", Error: wrapped})
			c.emitDisconnectWithClean(fmt.Sprintf("read loop stopped: %v", wrapped), false)
			return
		}
		if raw.ID == consts.PlayClientboundClientboundKeepAlive {
			pkt := &protocol.PlayClientboundKeepAlivePacket{}
			if err := unmarshalRaw(pkt, raw); err == nil {
				c.statsMu.Lock()
				c.stats.LastKeepAliveAt = time.Now()
				c.statsMu.Unlock()
				if err := c.writePacket(&protocol.PlayServerboundKeepAlivePacket{KeepAliveID: pkt.KeepAliveID}); err != nil {
					c.bus.Emit(state.ErrorEvent{Op: "keepalive_response", Error: err})
					c.emitDisconnectWithClean(fmt.Sprintf("keepalive response failed: %v", err), false)
					return
				}
				c.log("keepalive replied", int32(pkt.KeepAliveID), nil)
			}
		}
		_ = c.dispatcher.Dispatch(c.currentState(), raw)
	}
}

func (c *Client) heartbeatLoop() {
	defer c.wg.Done()
	t := ticker50ms()
	defer t.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-t.C:
			if err := c.writePacket(c.heartbeatPacket()); err != nil {
				if c.closing() {
					return
				}
				c.bus.Emit(state.ErrorEvent{Op: "heartbeat", Error: err})
				c.emitDisconnectWithClean(fmt.Sprintf("heartbeat stopped: %v", err), false)
				return
			}
		}
	}
}
