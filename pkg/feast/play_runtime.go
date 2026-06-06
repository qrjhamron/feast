package feast

import (
	"context"
	"math"
	"time"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

func applyGravityTick(st PlayerState, belowPassable bool) PlayerState {
	if belowPassable {
		vy := st.VelocityY - 0.08
		if vy < -0.98 {
			vy = -0.98
		}
		st.VelocityY = vy
		st.Y += vy
		st.OnGround = false
		return st
	}
	st.Y = math.Floor(st.Y)
	st.VelocityY = 0
	st.OnGround = true
	return st
}

func (c *Client) isSupportLostForState(st PlayerState) bool {
	feetX := int(math.Floor(st.X))
	feetZ := int(math.Floor(st.Z))
	belowY := int(math.Floor(st.Y)) - 1

	isAirOrReplaceable := func(x, y, z int) bool {
		_, err := c.world.GetBlock(x, y, z)
		if err != nil {
			// Unloaded blocks are treated as solid support to prevent falling into the void.
			return false
		}
		return c.world.IsReplaceable(world.BlockPos{X: int32(x), Y: int32(y), Z: int32(z)})
	}

	// 1. Check if the block directly under the bot's feet is air or replaceable.
	if !isAirOrReplaceable(feetX, belowY, feetZ) {
		return false
	}

	// 2. Check if the entire footprint support of the bot (XZ radius 0.3 around the bot) is removed
	minX := int(math.Floor(st.X - 0.3))
	maxX := int(math.Floor(st.X + 0.3))
	minZ := int(math.Floor(st.Z - 0.3))
	maxZ := int(math.Floor(st.Z + 0.3))

	for x := minX; x <= maxX; x++ {
		for z := minZ; z <= maxZ; z++ {
			if !isAirOrReplaceable(x, belowY, z) {
				return false
			}
		}
	}
	return true
}

// IsSupportLost checks if all blocks under the bot's footprint (XZ radius 0.3 around the bot) at Y = floor(Y)-1 are air or replaceable.
func (c *Client) IsSupportLost() bool {
	return c.isSupportLostForState(c.PlayerState())
}

// WaitForGround applies gravity ticks and sends position packets until the bot lands or a timeout is reached.
func (c *Client) WaitForGround(ctx context.Context) error {
	if !c.AcquireMovement() {
		return nil
	}
	defer c.ReleaseMovement()

	if !c.IsSupportLost() {
		return nil
	}

	startY := c.PlayerState().Y
	c.debugActionf("gravity", "support_lost=true")
	c.debugActionf("gravity", "start_y=%v", startY)

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	packetsSent := 0
	timeoutChan := time.After(10 * time.Second)

	for {
		select {
		case <-ctx.Done():
			st := c.PlayerState()
			c.debugActionf("gravity", "final_y=%v", st.Y)
			c.debugActionf("gravity", "grounded=%v", st.OnGround)
			c.debugActionf("gravity", "packets_sent=%d", packetsSent)
			c.debugActionf("gravity", "result=FAIL")
			return ctx.Err()
		case <-timeoutChan:
			st := c.PlayerState()
			c.debugActionf("gravity", "final_y=%v", st.Y)
			c.debugActionf("gravity", "grounded=%v", st.OnGround)
			c.debugActionf("gravity", "packets_sent=%d", packetsSent)
			c.debugActionf("gravity", "result=FAIL")
			return ErrGravityTimeout
		case <-ticker.C:
			st := c.PlayerState()
			belowPassable := c.isSupportLostForState(st)
			if !belowPassable {
				// Grounded! Apply gravity tick with belowPassable=false to snap.
				next := applyGravityTick(st, false)
				c.stateMu.Lock()
				c.player = next
				c.stateMu.Unlock()

				// Send final position packet
				pkt := &protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
					X:        next.X,
					Y:        next.Y,
					Z:        next.Z,
					Yaw:      next.Yaw,
					Pitch:    next.Pitch,
					OnGround: next.OnGround,
				}
				if err := c.writePacket(pkt); err != nil {
					return err
				}
				packetsSent++

				c.debugActionf("gravity", "final_y=%v", next.Y)
				c.debugActionf("gravity", "grounded=true")
				c.debugActionf("gravity", "packets_sent=%d", packetsSent)
				c.debugActionf("gravity", "result=PASS")
				return nil
			}

			// Airborne, apply gravity tick
			next := applyGravityTick(st, true)
			c.stateMu.Lock()
			c.player = next
			c.stateMu.Unlock()

			pkt := &protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
				X:        next.X,
				Y:        next.Y,
				Z:        next.Z,
				Yaw:      next.Yaw,
				Pitch:    next.Pitch,
				OnGround: next.OnGround,
			}
			if err := c.writePacket(pkt); err != nil {
				return err
			}
			packetsSent++
		}
	}
}

func (c *Client) gravityTick() {
	st := c.PlayerState()
	feetX := int(st.X)
	feetY := int(st.Y)
	feetZ := int(st.Z)
	belowPassable := c.world.IsPassable(feetX, feetY-1, feetZ)
	next := applyGravityTick(st, belowPassable)

	c.stateMu.Lock()
	c.player = next
	c.stateMu.Unlock()
}

func (c *Client) handleStartConfiguration() error {
	c.StopNavigation()
	if err := c.fsm.Transition(state.StateConfiguration); err != nil {
		return err
	}
	if err := c.runConfigFlow(); err != nil {
		return err
	}
	if err := c.fsm.Transition(state.StatePlay); err != nil {
		return err
	}
	return c.writePacket(&protocol.PlayServerboundClientInformationPacket{})
}
