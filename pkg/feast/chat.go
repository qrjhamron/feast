package feast

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/state"
)

// SendChat sends one chat message in play state.
func (c *Client) SendChat(message string) error {
	if err := c.requireConn(); err != nil {
		return err
	}

	if c.CurrentState() != state.StatePlay {
		return fmt.Errorf("cannot send chat: not in play state (current: %v)", c.CurrentState())
	}

	// In offline mode without signed chat, keeping count at 0 is often safer
	// than incrementing it without a proper session/signature chain.
	count := int32(0)

	pkt := &protocol.PlayServerboundChatMessagePacket{
		Message:      message,
		Timestamp:    time.Now().UnixMilli(),
		Salt:         rand.Int63(),
		MessageCount: count,
	}

	return c.writePacket(pkt)
}
