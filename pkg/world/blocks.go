package world

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed blocks_1.20.4.json
var blocksJSON []byte

type blockRegistryEntry struct {
	Name        string `json:"name"`
	BoundingBox string `json:"boundingBox"`
	MinStateID  int32  `json:"minStateId"`
	MaxStateID  int32  `json:"maxStateId"`
}

var (
	registryOnce       sync.Once
	registryErr        error
	passableBlockNames map[string]struct{}
	blockNameByStateID map[int32]string
	// stateIDByName maps a block name to its lowest (canonical) global state ID.
	// It lets FindAnyStateIDByName resolve in O(1) and deterministically instead
	// of scanning the full ~26k-entry state table in non-deterministic map order.
	stateIDByName map[string]int32
)

func initBlockRegistry() error {
	registryOnce.Do(func() {
		var entries []blockRegistryEntry
		if err := json.Unmarshal(blocksJSON, &entries); err != nil {
			registryErr = fmt.Errorf("unmarshal embedded blocks.json: %w", err)
			return
		}
		passableBlockNames = make(map[string]struct{}, len(entries))
		blockNameByStateID = make(map[int32]string, len(entries)*4)
		stateIDByName = make(map[string]int32, len(entries))
		for _, e := range entries {
			if e.Name == "" {
				continue
			}
			if e.BoundingBox == "empty" {
				passableBlockNames[e.Name] = struct{}{}
			}
			if existing, ok := stateIDByName[e.Name]; !ok || e.MinStateID < existing {
				stateIDByName[e.Name] = e.MinStateID
			}
			for stateID := e.MinStateID; stateID <= e.MaxStateID; stateID++ {
				blockNameByStateID[stateID] = e.Name
			}
		}
	})
	return registryErr
}

func ResolveBlockName(id int32) string {
	if err := initBlockRegistry(); err != nil {
		return ""
	}
	return blockNameByStateID[id]
}

func isPassableBlockName(name string) bool {
	if name == "" {
		return false
	}
	if err := initBlockRegistry(); err != nil {
		return false
	}
	_, ok := passableBlockNames[name]
	return ok
}

func FindAnyStateIDByName(name string) (int32, bool) {
	if err := initBlockRegistry(); err != nil {
		return 0, false
	}
	id, ok := stateIDByName[name]
	return id, ok
}
