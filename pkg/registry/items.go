package registry

import (
	"fmt"
	"strings"
)

type ItemStack struct {
	Present bool
	ItemID  int32
	Name    string
	Count   int
	NBT     []byte
	Source  string // for compatibility
}

var itemIDToName = map[int32]string{
	0:   "air",
	1:   "stone",
	27:  "grass_block",
	14:  "grass_block", // legacy
	28:  "dirt",
	15:  "dirt", // legacy
	35:  "cobblestone",
	22:  "cobblestone", // legacy
	36:  "oak_planks",
	23:  "oak_planks", // legacy
	131: "oak_log",
	57:  "sand",
	61:  "gravel",
	62:  "coal_ore",
	64:  "iron_ore",
	8:   "deepslate",
	32:  "water",
	33:  "lava",

	// Tools
	816: "wooden_pickaxe",
	821: "stone_pickaxe",
	831: "iron_pickaxe",
	836: "diamond_pickaxe",

	815: "wooden_shovel",
	820: "stone_shovel",
	830: "iron_shovel",
	835: "diamond_shovel",

	817: "wooden_axe",
	822: "stone_axe",
	832: "iron_axe",
	837: "diamond_axe",
}

var itemNameToID = map[string]int32{}

func init() {
	for id, name := range itemIDToName {
		// Prefer standard/current IDs for name-to-ID mapping
		if _, exists := itemNameToID[name]; !exists || id >= 27 {
			itemNameToID[name] = id
		}
	}
}

func ItemNameFromID(id int32) (string, bool) {
	name, ok := itemIDToName[id]
	if ok {
		return name, true
	}
	return fmt.Sprintf("item_%d", id), false
}

func BlockNameFromItem(item ItemStack) (string, bool) {
	if !item.Present {
		return "air", true
	}
	// Check standard ItemNameFromID
	return ItemNameFromID(item.ItemID)
}

func IsPlaceableBlockItem(item ItemStack) bool {
	if !item.Present {
		return false
	}
	name, ok := BlockNameFromItem(item)
	if !ok || name == "air" {
		return false
	}
	// Items that represent placeable blocks
	placeableBlocks := map[string]bool{
		"stone":       true,
		"dirt":        true,
		"grass_block": true,
		"cobblestone": true,
		"oak_log":     true,
		"oak_planks":  true,
		"sand":        true,
		"gravel":      true,
		"coal_ore":    true,
		"iron_ore":    true,
		"deepslate":   true,
	}
	return placeableBlocks[normalizeName(name)]
}

func normalizeName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.TrimPrefix(name, "minecraft:")
	return name
}
