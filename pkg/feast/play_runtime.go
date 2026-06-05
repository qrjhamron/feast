package feast

import (
	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/state"
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
	st.Y = float64(int(st.Y))
	st.VelocityY = 0
	st.OnGround = true
	return st
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
