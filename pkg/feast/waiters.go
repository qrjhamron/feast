package feast

import (
	"context"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

// WaitForPositionSync blocks until the client receives a server position sync
// or the context is cancelled.
//
// It is a discoverability-friendly alias for [Client.WaitUntilReady].
func (c *Client) WaitForPositionSync(ctx context.Context) error {
	return c.WaitUntilReady(ctx)
}

// WaitForBlockUpdate blocks until the server updates the loaded block at the
// given coordinates and returns the post-update cached world state.
//
// It waits for a block_update or section_blocks_update event that touches the
// exact block. The returned state comes from the world cache after the update
// handlers have run. If the target chunk is not loaded yet, the waiter keeps
// waiting rather than treating it as air.
func (c *Client) WaitForBlockUpdate(ctx context.Context, x, y, z int) (world.BlockState, error) {
	if c == nil {
		return world.BlockState{}, ErrNotConnected
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return world.BlockState{}, err
	}

	target := protocol.BlockPos{X: int32(x), Y: int32(y), Z: int32(z)}
	matched := make(chan struct{}, 1)
	blockID, _ := c.bus.On("block_update", func(e state.Event) {
		ev, ok := e.(state.BlockUpdateEvent)
		if !ok {
			return
		}
		if ev.X == target.X && ev.Y == target.Y && ev.Z == target.Z {
			select {
			case matched <- struct{}{}:
			default:
			}
		}
	})
	sectionID, _ := c.bus.On("section_blocks_update", func(e state.Event) {
		ev, ok := e.(state.SectionBlocksUpdateEvent)
		if !ok {
			return
		}
		for _, update := range ev.Updates {
			if update.X == target.X && update.Y == target.Y && update.Z == target.Z {
				select {
				case matched <- struct{}{}:
				default:
				}
				return
			}
		}
	})
	defer c.bus.Off(blockID)
	defer c.bus.Off(sectionID)

	select {
	case <-ctx.Done():
		return world.BlockState{}, ctx.Err()
	case <-matched:
		block, err := c.world.GetBlock(x, y, z)
		if err != nil {
			return world.BlockState{}, err
		}
		return block, nil
	}
}

// WaitForBlockState blocks until the loaded block at x/y/z matches name.
//
// The target must be loaded and actually match the requested state name; an
// unloaded chunk does not count as a match.
func (c *Client) WaitForBlockState(ctx context.Context, x, y, z int, name string) error {
	if c == nil {
		return ErrNotConnected
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	want := normalizeBlockName(name)
	if block, err := c.world.GetBlock(x, y, z); err == nil && normalizeBlockName(block.Name) == want {
		return nil
	}
	for {
		block, err := c.WaitForBlockUpdate(ctx, x, y, z)
		if err != nil {
			return err
		}
		if normalizeBlockName(block.Name) == want {
			return nil
		}
	}
}
