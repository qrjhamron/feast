package feast

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/protocol/consts"
	"github.com/user/feastgo/pkg/state"
	"github.com/user/feastgo/pkg/world"
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
		if raw.ID == consts.PlayClientboundStartConfiguration {
			if err := c.handleStartConfiguration(); err != nil {
				c.bus.Emit(state.ErrorEvent{Op: "start_configuration", Error: err})
				c.emitDisconnectWithClean(fmt.Sprintf("start configuration failed: %v", err), false)
				return
			}
			continue
		}
		if raw.ID == consts.PlayClientboundClientboundKeepAlive {
			pkt := &protocol.PlayClientboundKeepAlivePacket{}
			if err := unmarshalRaw(pkt, raw); err == nil {
				c.statsMu.Lock()
				c.stats.LastKeepAliveAt = time.Now()
				c.statsMu.Unlock()
				c.respondKeepAlive(pkt.KeepAliveID)
			}
		}
		if raw.ID == consts.PlayClientboundChunkDataAndUpdateLight {
			c.wg.Add(1)
			go func(pkt *protocol.RawPacket, st state.State) {
				defer c.wg.Done()
				c.chunkSem <- struct{}{}
				defer func() { <-c.chunkSem }()
				if err := c.ingestChunk(pkt); err != nil {
					c.log(fmt.Sprintf("[hpa] chunk ingest failed: %v", err), -1, nil)
				}
				if err := c.dispatcher.Dispatch(st, pkt); err != nil {
					c.log(fmt.Sprintf("[dispatch] error: %v", err), -1, err)
				}
			}(raw, c.currentState())
			continue
		}
		if err := c.dispatcher.Dispatch(c.currentState(), raw); err != nil {
			c.log(fmt.Sprintf("[dispatch] error: %v", err), raw.ID, err)
		}
	}
}

func (c *Client) ingestChunk(raw *protocol.RawPacket) error {
	pkt := &protocol.PlayClientboundChunkDataAndUpdateLightPacket{}
	if err := unmarshalRaw(pkt, raw); err != nil {
		return err
	}
	chunk, err := world.ParseChunkFromTailData(pkt.ChunkX, pkt.ChunkZ, pkt.TailData)
	if err != nil {
		return err
	}
	c.world.AddChunk(chunk)
	return nil
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
			c.statsMu.RLock()
			lastKA := c.stats.LastKeepAliveAt
			c.statsMu.RUnlock()
			if !lastKA.IsZero() && time.Since(lastKA) > 20*time.Second {
				c.log("[keepalive] timeout risk if last keepalive > 20s ago", -1, nil)
			}
			if c.IsMoving() {
				// Navigation owns movement packets while active.
				continue
			}
			hb := c.heartbeatPacket()
			if err := c.writePacket(hb); err != nil {
				if c.closing() || errors.Is(err, ErrClientClosed) {
					return
				}
				c.bus.Emit(state.ErrorEvent{Op: "heartbeat", Error: err})
				c.emitDisconnectWithClean(fmt.Sprintf("heartbeat stopped: %v", err), false)
				return
			}
		}
	}
}

func (c *Client) respondKeepAlive(id int64) {
	if err := c.writePacket(&protocol.PlayServerboundKeepAlivePacket{KeepAliveID: id}); err != nil {
		if c.closing() || errors.Is(err, ErrClientClosed) {
			return
		}
		c.bus.Emit(state.ErrorEvent{Op: "keepalive_response", Error: err})
		c.emitDisconnectWithClean(fmt.Sprintf("keepalive response failed: %v", err), false)
		return
	}
	c.log("keepalive replied", int32(id), nil)
}
