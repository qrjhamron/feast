package feast

import (
	"context"
	"fmt"
	"time"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/state"
)

// Container represents an open inventory window, such as a chest.
type Container struct {
	// ID is the server-assigned window ID.
	ID int32
	// Type is the container type (e.g. "minecraft:generic_9x3").
	Type string
	// Title is the parsed chat-component title of the container.
	Title  string
	client *Client
}

var (
// Errors moved to errors.go
)

func (ct *Container) Items() map[int]ItemStack {
	if ct == nil || ct.client == nil {
		return nil
	}
	return ct.client.ContainerItems(ct.ID)
}

func (ct *Container) Deposit(ctx context.Context, itemName string, count int) error {
	if ct == nil || ct.client == nil {
		return ErrContainerNotOpen
	}
	return ct.client.DepositToContainer(ctx, ct.ID, itemName, count)
}

func (ct *Container) Withdraw(ctx context.Context, itemName string, count int) error {
	if ct == nil || ct.client == nil {
		return ErrContainerNotOpen
	}
	return ct.client.WithdrawFromContainer(ctx, ct.ID, itemName, count)
}

func (ct *Container) Close(ctx context.Context) error {
	if ct == nil || ct.client == nil {
		return ErrContainerNotOpen
	}
	return ct.client.CloseContainer(ctx, ct.ID)
}

func (c *Client) OpenChest(ctx context.Context, pos BlockPos) (*Container, error) {
	ch := make(chan *Container, 1)
	handlerID, _ := c.bus.On("open_screen", func(e state.Event) {
		if ev, ok := e.(state.OpenScreenEvent); ok {
			typeName := "chest"
			// 0 = generic_9x1, 1 = generic_9x2, 2 = generic_9x3, etc.
			if ev.WindowType >= 0 && ev.WindowType <= 5 {
				typeName = "chest"
			} else {
				typeName = fmt.Sprintf("window_type_%d", ev.WindowType)
			}
			select {
			case ch <- &Container{
				ID:     ev.WindowID,
				Type:   typeName,
				Title:  ev.Title,
				client: c,
			}:
			default:
			}
		}
	})
	defer c.bus.Off(handlerID)

	// Send interact packet
	err := c.WritePacket(&protocol.PlayServerboundUseItemOnPacket{
		Hand:        0, // Main hand
		Position:    protocol.BlockPos{X: pos.X, Y: pos.Y, Z: pos.Z},
		Face:        protocol.BlockFaceTop,
		CursorX:     0.5,
		CursorY:     0.5,
		CursorZ:     0.5,
		InsideBlock: false,
		Sequence:    1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send chest use packet: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case container := <-ch:
		return container, nil
	case <-time.After(3 * time.Second):
		return nil, ErrContainerOpenTimeout
	}
}

func (c *Client) CloseContainer(ctx context.Context, id int32) error {
	err := c.WritePacket(&protocol.PlayServerboundCloseContainerPacket{
		WindowID: byte(id),
	})
	if err != nil {
		return fmt.Errorf("failed to send close container packet: %w", err)
	}

	c.containerMu.Lock()
	if c.activeContainer != nil && c.activeContainer.ID == id {
		c.activeContainer = nil
	}
	delete(c.containerSlots, id)
	delete(c.containerStateIDs, id)
	c.containerMu.Unlock()

	return nil
}

func (c *Client) ContainerItems(id int32) map[int]ItemStack {
	c.containerMu.RLock()
	defer c.containerMu.RUnlock()
	m, exists := c.containerSlots[id]
	if !exists {
		return nil
	}
	copyM := make(map[int]ItemStack, len(m))
	for k, v := range m {
		copyM[k] = v
	}
	return copyM
}

func (c *Client) trackContainerSlot(windowID int32, slot int16, item protocol.ItemStack) {
	c.containerMu.Lock()
	defer c.containerMu.Unlock()
	if c.containerSlots == nil {
		c.containerSlots = make(map[int32]map[int]ItemStack)
	}
	m, exists := c.containerSlots[windowID]
	if !exists {
		m = make(map[int]ItemStack)
		c.containerSlots[windowID] = m
	}
	if !item.Present {
		m[int(slot)] = ItemStack{Present: false}
	} else {
		name, _ := ItemNameFromID(item.ItemID)
		m[int(slot)] = ItemStack{
			Present: true,
			ItemID:  item.ItemID,
			Name:    name,
			Count:   int(item.Count),
			NBT:     item.NBT,
		}
	}
}

func (c *Client) trackContainerStateID(windowID, stateID int32) {
	c.containerMu.Lock()
	defer c.containerMu.Unlock()
	if c.containerStateIDs == nil {
		c.containerStateIDs = make(map[int32]int32)
	}
	c.containerStateIDs[windowID] = stateID
}

func (c *Client) trackContainerContentNonZero(windowID int32, slots []protocol.ItemStack) {
	c.containerMu.Lock()
	defer c.containerMu.Unlock()
	if c.containerSlots == nil {
		c.containerSlots = make(map[int32]map[int]ItemStack)
	}
	m := make(map[int]ItemStack)
	for slot, item := range slots {
		if !item.Present {
			m[slot] = ItemStack{Present: false}
		} else {
			name, _ := ItemNameFromID(item.ItemID)
			m[slot] = ItemStack{
				Present: true,
				ItemID:  item.ItemID,
				Name:    name,
				Count:   int(item.Count),
				NBT:     item.NBT,
			}
		}
	}
	c.containerSlots[windowID] = m
}

func (c *Client) ActiveContainer() *Container {
	c.containerMu.RLock()
	defer c.containerMu.RUnlock()
	return c.activeContainer
}

func (c *Client) DepositToContainer(ctx context.Context, containerID int32, itemName string, count int) error {
	if count <= 0 {
		return fmt.Errorf("invalid deposit count %d", count)
	}
	if err := c.requireActiveContainer(containerID); err != nil {
		return err
	}
	sourceSlot, sourceBefore, ok := c.findInventoryItemSlot(itemName, count)
	if !ok {
		return ErrInventoryItemNotFound
	}
	destSlot, _, ok := c.findContainerDestinationSlot(containerID, sourceBefore, count)
	if !ok {
		return ErrContainerFull
	}

	if err := c.sendContainerClick(containerID, int16(sourceSlot)); err != nil {
		return err
	}
	if err := c.waitForContainerTransfer(ctx, containerID, sourceSlot, destSlot, sourceBefore.Count, true); err != nil {
		return err
	}
	return nil
}

func (c *Client) WithdrawFromContainer(ctx context.Context, containerID int32, itemName string, count int) error {
	if count <= 0 {
		return fmt.Errorf("invalid withdraw count %d", count)
	}
	if err := c.requireActiveContainer(containerID); err != nil {
		return err
	}
	sourceSlot, sourceBefore, ok := c.findContainerItemSlot(containerID, itemName, count)
	if !ok {
		return ErrContainerSlotNotFound
	}
	destSlot, _, ok := c.findInventoryDestinationSlot(sourceBefore, count)
	if !ok {
		return ErrInventoryFull
	}

	if err := c.sendContainerClick(containerID, int16(sourceSlot)); err != nil {
		return err
	}
	if err := c.waitForContainerTransfer(ctx, containerID, sourceSlot, destSlot, sourceBefore.Count, false); err != nil {
		return err
	}
	return nil
}

func (c *Client) requireActiveContainer(containerID int32) error {
	c.containerMu.RLock()
	defer c.containerMu.RUnlock()
	if c.activeContainer == nil || c.activeContainer.ID != containerID {
		return ErrContainerNotOpen
	}
	if _, ok := c.containerSlots[containerID]; !ok {
		return ErrContainerNotOpen
	}
	return nil
}

func (c *Client) findInventoryItemSlot(itemName string, count int) (int, ItemStack, bool) {
	c.inventoryMu.RLock()
	defer c.inventoryMu.RUnlock()
	for slot := 9; slot <= 44; slot++ {
		st, ok := c.inventory.Slots[slot]
		if ok && st.Present && st.Name == itemName && st.Count >= count {
			return slot, st, true
		}
	}
	return -1, ItemStack{}, false
}

func (c *Client) findInventoryDestinationSlot(source ItemStack, count int) (int, ItemStack, bool) {
	c.inventoryMu.RLock()
	defer c.inventoryMu.RUnlock()
	for slot := 9; slot <= 44; slot++ {
		st, ok := c.inventory.Slots[slot]
		if !ok || !st.Present {
			return slot, ItemStack{Present: false}, true
		}
		if compatibleStack(st, source) && st.Count+count <= 64 {
			return slot, st, true
		}
	}
	return -1, ItemStack{}, false
}

func (c *Client) findContainerItemSlot(containerID int32, itemName string, count int) (int, ItemStack, bool) {
	c.containerMu.RLock()
	defer c.containerMu.RUnlock()
	for slot, st := range c.containerSlots[containerID] {
		if st.Present && st.Name == itemName && st.Count >= count {
			return slot, st, true
		}
	}
	return -1, ItemStack{}, false
}

func (c *Client) findContainerDestinationSlot(containerID int32, source ItemStack, count int) (int, ItemStack, bool) {
	c.containerMu.RLock()
	defer c.containerMu.RUnlock()
	for slot, st := range c.containerSlots[containerID] {
		if !st.Present {
			return slot, ItemStack{Present: false}, true
		}
		if compatibleStack(st, source) && st.Count+count <= 64 {
			return slot, st, true
		}
	}
	return -1, ItemStack{}, false
}

func compatibleStack(a, b ItemStack) bool {
	return a.Present && b.Present && a.ItemID == b.ItemID && a.Name == b.Name
}

func (c *Client) sendContainerClick(containerID int32, slot int16) error {
	c.containerMu.RLock()
	stateID := c.containerStateIDs[containerID]
	c.containerMu.RUnlock()
	return c.writePacket(&protocol.PlayServerboundClickContainerPacket{
		WindowID:    byte(containerID),
		StateID:     stateID,
		Slot:        slot,
		Button:      0,
		Mode:        0,
		CarriedItem: protocol.ItemStack{Present: false},
	})
}

func (c *Client) waitForContainerTransfer(ctx context.Context, containerID int32, sourceSlot, destSlot, sourceBefore int, deposit bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		if c.containerTransferObserved(containerID, sourceSlot, destSlot, sourceBefore, deposit) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return ErrContainerActionTimeout
		case <-tick.C:
		}
	}
}

func (c *Client) containerTransferObserved(containerID int32, sourceSlot, destSlot, sourceBefore int, deposit bool) bool {
	if deposit {
		c.inventoryMu.RLock()
		src := c.inventory.Slots[sourceSlot]
		c.inventoryMu.RUnlock()
		c.containerMu.RLock()
		dst := c.containerSlots[containerID][destSlot]
		c.containerMu.RUnlock()
		return dst.Present && (!src.Present || src.Count < sourceBefore)
	}

	c.containerMu.RLock()
	src := c.containerSlots[containerID][sourceSlot]
	c.containerMu.RUnlock()
	c.inventoryMu.RLock()
	dst := c.inventory.Slots[destSlot]
	c.inventoryMu.RUnlock()
	return dst.Present && (!src.Present || src.Count < sourceBefore)
}
