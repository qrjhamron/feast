package registry

import (
	"testing"
)

func TestRegistryItems(t *testing.T) {
	// 1. known item IDs
	name, ok := ItemNameFromID(1)
	if !ok || name != "stone" {
		t.Errorf("expected stone for ID 1, got name=%s ok=%t", name, ok)
	}

	name, ok = ItemNameFromID(831)
	if !ok || name != "iron_pickaxe" {
		t.Errorf("expected iron_pickaxe for ID 831, got name=%s ok=%t", name, ok)
	}

	// 2. unknown item IDs
	name, ok = ItemNameFromID(99999)
	if ok || name != "item_99999" {
		t.Errorf("expected item_99999/false for unknown ID, got name=%s ok=%t", name, ok)
	}

	// 3. placeable item check
	stoneItem := ItemStack{Present: true, ItemID: 1}
	if !IsPlaceableBlockItem(stoneItem) {
		t.Errorf("expected stone to be placeable")
	}

	pickItem := ItemStack{Present: true, ItemID: 831}
	if IsPlaceableBlockItem(pickItem) {
		t.Errorf("expected pickaxe to NOT be placeable")
	}

	emptyItem := ItemStack{Present: false}
	if IsPlaceableBlockItem(emptyItem) {
		t.Errorf("expected empty item to NOT be placeable")
	}
}

func TestRegistryBlocks(t *testing.T) {
	// 4. unknown block fallback
	name, ok := BlockNameFromStateID(99999)
	if ok || name != "block_99999" {
		t.Errorf("expected block_99999/false for unknown state, got name=%s ok=%t", name, ok)
	}

	name, ok = BlockNameFromStateID(1)
	if !ok || name != "stone" {
		t.Errorf("expected stone for state 1, got name=%s ok=%t", name, ok)
	}
}

func TestRegistryTools(t *testing.T) {
	// 5. preferred tool for stone/dirt/wood
	if PreferredToolForBlock("stone") != ToolPickaxe {
		t.Errorf("expected pickaxe for stone")
	}
	if PreferredToolForBlock("dirt") != ToolShovel {
		t.Errorf("expected shovel for dirt")
	}
	if PreferredToolForBlock("oak_log") != ToolAxe {
		t.Errorf("expected axe for oak_log")
	}

	if ToolKindFromItem("diamond_pickaxe") != ToolPickaxe {
		t.Errorf("expected pickaxe for diamond_pickaxe")
	}
	if ToolKindFromItem("iron_shovel") != ToolShovel {
		t.Errorf("expected shovel for iron_shovel")
	}
	if ToolKindFromItem("stone_axe") != ToolAxe {
		t.Errorf("expected axe for stone_axe")
	}
	if ToolKindFromItem("dirt") != ToolNone {
		t.Errorf("expected none for dirt item")
	}
}
