package registry

import (
	"strings"
)

type ToolKind string

const (
	ToolNone    ToolKind = "none"
	ToolPickaxe ToolKind = "pickaxe"
	ToolShovel  ToolKind = "shovel"
	ToolAxe     ToolKind = "axe"
)

func PreferredToolForBlock(blockName string) ToolKind {
	name := normalizeBlockName(blockName)
	switch name {
	case "stone", "cobblestone", "coal_ore", "iron_ore", "deepslate":
		return ToolPickaxe
	case "dirt", "grass_block", "sand", "gravel":
		return ToolShovel
	case "oak_log", "oak_planks":
		return ToolAxe
	default:
		// Check substrings for general robustness
		if strings.Contains(name, "ore") || strings.Contains(name, "deepslate") || strings.Contains(name, "stone") {
			return ToolPickaxe
		}
		if strings.Contains(name, "dirt") || strings.Contains(name, "grass") || strings.Contains(name, "sand") || strings.Contains(name, "gravel") {
			return ToolShovel
		}
		if strings.Contains(name, "log") || strings.Contains(name, "planks") || strings.Contains(name, "wood") {
			return ToolAxe
		}
		return ToolNone
	}
}

func ToolKindFromItem(itemName string) ToolKind {
	name := normalizeBlockName(itemName)
	if strings.Contains(name, "pickaxe") {
		return ToolPickaxe
	}
	if strings.Contains(name, "shovel") {
		return ToolShovel
	}
	if strings.Contains(name, "axe") && !strings.Contains(name, "pickaxe") {
		return ToolAxe
	}
	return ToolNone
}

func normalizeBlockName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.TrimPrefix(name, "minecraft:")
	return name
}
