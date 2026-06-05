package state

import (
	"testing"

	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/protocol/consts"
)

func TestDispatcherSetHeldItemEmitsInventoryEvent(t *testing.T) {
	bus := NewEventBus()
	dispatcher := NewDispatcher(bus)
	got := make(chan HeldItemEvent, 1)
	if _, err := bus.On("held_item", func(e Event) {
		if ev, ok := e.(HeldItemEvent); ok {
			got <- ev
		}
	}); err != nil {
		t.Fatalf("on held_item: %v", err)
	}

	raw := rawFromPacket(t, &protocol.PlayClientboundSetHeldItemPacket{Slot: 4})
	if raw.ID != consts.PlayClientboundSetHeldItem {
		t.Fatalf("packet id=0x%02x want 0x%02x", raw.ID, consts.PlayClientboundSetHeldItem)
	}
	if err := dispatcher.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	select {
	case ev := <-got:
		if ev.Slot != 4 {
			t.Fatalf("slot=%d want 4", ev.Slot)
		}
	default:
		t.Fatal("expected held_item event")
	}
}

func TestDispatcherSetContainerSlotEmitsInventorySlotEvent(t *testing.T) {
	bus := NewEventBus()
	dispatcher := NewDispatcher(bus)
	got := make(chan InventorySlotEvent, 1)
	if _, err := bus.On("inventory_slot", func(e Event) {
		if ev, ok := e.(InventorySlotEvent); ok {
			got <- ev
		}
	}); err != nil {
		t.Fatalf("on inventory_slot: %v", err)
	}

	raw := rawFromPacket(t, &protocol.PlayClientboundSetContainerSlotPacket{
		WindowID: 0,
		StateID:  7,
		Slot:     36,
		Item:     protocol.ItemStack{Present: true, ItemID: 1, Count: 64, NBT: []byte{0x00}},
	})
	if err := dispatcher.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	select {
	case ev := <-got:
		if ev.WindowID != 0 || ev.StateID != 7 || ev.Slot != 36 {
			t.Fatalf("unexpected event: %+v", ev)
		}
		if !ev.Item.Present || ev.Item.ItemID != 1 || ev.Item.Count != 64 {
			t.Fatalf("unexpected item: %+v", ev.Item)
		}
	default:
		t.Fatal("expected inventory_slot event")
	}
}
