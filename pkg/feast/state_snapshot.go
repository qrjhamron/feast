package feast

import (
	"strings"

	"github.com/qrjhamron/feast/pkg/state"
)

// StateSnapshot is an immutable point-in-time snapshot of the bot's common
// runtime state.
//
// Ready means the client is in Play state and has received a server position
// sync. Connected means the client is past the initial handshake/login stages
// and has not been fully torn down yet.
type StateSnapshot struct {
	Username           string
	Connected          bool
	Ready              bool
	PositionSynced     bool
	CurrentState       state.State
	EntityID           int32
	Position           Vec3
	Yaw                float32
	Pitch              float32
	Health             float32
	Food               int32
	SelectedHotbarSlot int
	HeldItem           ItemStack
}

// State returns a thread-safe snapshot of the client's common runtime state.
func (c *Client) State() StateSnapshot {
	if c == nil {
		return StateSnapshot{}
	}
	player := c.PlayerState()
	inv := c.InventorySnapshot()
	positionSynced := c.PositionSynced()
	held := ItemStack{}
	if slot := 36 + inv.SelectedHotbarSlot; slot >= 0 {
		if stack, ok := inv.Slots[slot]; ok && stack.Present {
			held = cloneItemStack(stack)
		}
	}
	current := c.currentState()
	return StateSnapshot{
		Username:           c.opts.Username,
		Connected:          current != state.StateHandshaking && current != state.StateDisconnected,
		Ready:              current == state.StatePlay && positionSynced,
		PositionSynced:     positionSynced,
		CurrentState:       current,
		EntityID:           player.EntityID,
		Position:           Vec3{X: player.X, Y: player.Y, Z: player.Z},
		Yaw:                player.Yaw,
		Pitch:              player.Pitch,
		Health:             player.Health,
		Food:               player.Food,
		SelectedHotbarSlot: inv.SelectedHotbarSlot,
		HeldItem:           held,
	}
}

func cloneItemStack(in ItemStack) ItemStack {
	out := in
	if len(in.NBT) > 0 {
		out.NBT = append([]byte(nil), in.NBT...)
	}
	return out
}

func normalizeBlockName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.TrimPrefix(name, "minecraft:")
	return name
}
