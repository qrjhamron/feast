package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/qrjhamron/feast/pkg/feast"
	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/world"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	host := getenv("MC_HOST", "127.0.0.1")
	port := getenv("MC_PORT", "25565")
	username := getenv("MC_USERNAME", "FeastGoBot")
	debug := os.Getenv("FEAST_DEBUG") == "true"

	bot := feast.NewClient(feast.Options{
		Host:     host,
		Port:     port,
		Username: username,
		Debug:    debug,
	})

	mainCtx, mainCancel := context.WithCancel(context.Background())
	defer mainCancel()

	stopChan := make(chan struct{})
	var commandRunning atomic.Bool

	// Register event hooks before connecting.
	bot.OnChat(func(e feast.ChatEvent) {
		fmt.Printf("[chat-debug] sender=%q msg=%q\n", e.Sender, e.Message)
		msg := cleanChatMessage(e.Message)
		if msg == "" {
			return
		}

		fields := strings.Fields(msg)
		cmd := fields[0]

		switch cmd {
		case "!help":
			sendChat(bot, "commands: !pos, !inv, !find <block> [count] [radius], !goto <block> [radius], !break <block> [radius], !mine <block> [radius], !stop")
		case "!pos":
			pos := bot.Position()
			sendChat(bot, fmt.Sprintf("pos: x=%.2f y=%.2f z=%.2f", pos.X, pos.Y, pos.Z))
		case "!inv":
			inv := bot.Inventory()
			selected := inv.SelectedHotbarSlot
			heldItemStr := "empty"
			heldItem, hasHeld := bot.HeldItem()
			if hasHeld && heldItem.Present && heldItem.Name != "" && heldItem.Name != "air" {
				heldItemStr = fmt.Sprintf("%s x%d", heldItem.Name, heldItem.Count)
			}

			var parts []string
			for i := 0; i < 9; i++ {
				slotIndex := 36 + i
				stack, ok := inv.Slots[slotIndex]
				if ok && stack.Present && stack.Name != "" && stack.Name != "air" {
					parts = append(parts, fmt.Sprintf("%d=%s x%d", i, stack.Name, stack.Count))
				} else {
					parts = append(parts, fmt.Sprintf("%d=empty", i))
				}
			}
			sendChat(bot, fmt.Sprintf("hotbar: selected=%d held=%s", selected, heldItemStr))
			sendChat(bot, strings.Join(parts, ", "))
		case "!stop":
			sendChat(bot, "stopping "+username)
			mainCancel()
			close(stopChan)
		case "!find":
			go func() {
				if !commandRunning.CompareAndSwap(false, true) {
					handleBadCommand(bot, "busy: command already running")
					return
				}
				defer commandRunning.Store(false)

				block, count, radius, ok := parseFindArgs(msg)
				if !ok {
					handleBadCommand(bot, "usage: !find <block> [count] [radius]")
					return
				}

				fmt.Printf("[chatcmd] command=find block=%s count=%d radius=%d\n", block, count, radius)

				// We search loaded blocks only.
				matches := localFindBlocks(bot.World(), bot.Position(), block, count, radius)

				fmt.Printf("[chatcmd] found=%d\n", len(matches))

				resolved := resolveBlockName(block)
				if len(matches) == 0 {
					sendChat(bot, fmt.Sprintf("not found: %s within radius %d loaded blocks", resolved, radius))
					return
				}

				sendChat(bot, fmt.Sprintf("found %d %s:", len(matches), resolved))
				var chunk []string
				for i, m := range matches {
					chunk = append(chunk, fmt.Sprintf("%d) %d %d %d d=%.2f", i+1, m.X, m.Y, m.Z, m.Distance))
					if len(chunk) == 5 || i == len(matches)-1 {
						sendChat(bot, strings.Join(chunk, " | "))
						chunk = nil
					}
				}
			}()
		case "!goto":
			go func() {
				if !commandRunning.CompareAndSwap(false, true) {
					handleBadCommand(bot, "busy: command already running")
					return
				}
				defer commandRunning.Store(false)

				block, radius, ok := parseGotoArgs(msg)
				if !ok {
					handleBadCommand(bot, "usage: !goto <block> [radius]")
					return
				}

				fmt.Printf("[chatcmd] command=goto block=%s radius=%d\n", block, radius)

				matches := localFindBlocks(bot.World(), bot.Position(), block, 1, radius)
				if len(matches) == 0 {
					sendChat(bot, "goto failed: block not found")
					return
				}
				m := matches[0]

				sendChat(bot, fmt.Sprintf("navigating to %s at %d %d %d...", resolveBlockName(block), m.X, m.Y, m.Z))

				ctx, cancel := context.WithTimeout(mainCtx, 60*time.Second)
				defer cancel()

				var g goal.Goal
				sx, sy, sz, ok := findSafeStandingPositionNear(bot.World(), m.X, m.Y, m.Z)
				if ok {
					g = goal.NewGoalBlock(sx, sy, sz)
				} else {
					g = goal.NewGoalProximity(m.X, m.Y, m.Z, 2.0)
				}

				err := bot.NavigateTo(ctx, g)
				if err != nil {
					sendChat(bot, fmt.Sprintf("goto failed: %v", err))
				} else {
					sendChat(bot, fmt.Sprintf("arrived near %s", resolveBlockName(block)))
				}
			}()
		case "!break":
			go func() {
				if !commandRunning.CompareAndSwap(false, true) {
					handleBadCommand(bot, "busy: command already running")
					return
				}
				defer commandRunning.Store(false)

				block, radius, ok := parseBreakArgs(msg)
				if !ok {
					handleBadCommand(bot, "usage: !break <block> [radius]")
					return
				}

				fmt.Printf("[chatcmd] command=break block=%s radius=%d\n", block, radius)

				botPos := bot.Position()
				botFootX := int32(math.Floor(botPos.X))
				botFootY := int32(math.Floor(botPos.Y))
				botFootZ := int32(math.Floor(botPos.Z))

				matches := localFindBlocks(bot.World(), botPos, block, 100, radius)
				if len(matches) == 0 {
					sendChat(bot, "break failed: block not found")
					return
				}

				var selectedBlock *world.BlockHit
				for i := range matches {
					m := &matches[i]
					pos := feast.BlockPos{X: int32(m.X), Y: int32(m.Y), Z: int32(m.Z)}

					underFeet := pos.X == botFootX && pos.Z == botFootZ && pos.Y == botFootY-1

					minX := botPos.X - 0.3
					maxX := botPos.X + 0.3
					minZ := botPos.Z - 0.3
					maxZ := botPos.Z + 0.3
					blockMinX := float64(pos.X)
					blockMaxX := float64(pos.X) + 1.0
					blockMinZ := float64(pos.Z)
					blockMaxZ := float64(pos.Z) + 1.0

					inFootprint := pos.Y == botFootY-1 &&
						blockMinX <= maxX && blockMaxX >= minX &&
						blockMinZ <= maxZ && blockMaxZ >= minZ

					if underFeet {
						fmt.Printf("[break-select] rejected_under_feet=%d,%d,%d\n", pos.X, pos.Y, pos.Z)
						continue
					}
					if inFootprint {
						fmt.Printf("[break-select] rejected_support=%d,%d,%d\n", pos.X, pos.Y, pos.Z)
						continue
					}

					// Reachability check
					eyeX, eyeY, eyeZ := botPos.X, botPos.Y+1.62, botPos.Z
					tcX, tcY, tcZ := float64(pos.X)+0.5, float64(pos.Y)+0.5, float64(pos.Z)+0.5
					dx := tcX - eyeX
					dy := tcY - eyeY
					dz := tcZ - eyeZ
					dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
					if dist > 4.5 {
						continue
					}

					selectedBlock = m
					break
				}

				if selectedBlock == nil {
					sendChat(bot, "break failed: no safe reachable block found")
					return
				}

				pos := feast.BlockPos{X: int32(selectedBlock.X), Y: int32(selectedBlock.Y), Z: int32(selectedBlock.Z)}
				fmt.Printf("[break-select] selected=%d,%d,%d reason=safe_reachable\n", pos.X, pos.Y, pos.Z)

				sendChat(bot, fmt.Sprintf("breaking %s at %d %d %d...", resolveBlockName(block), selectedBlock.X, selectedBlock.Y, selectedBlock.Z))

				ctx, cancel := context.WithTimeout(mainCtx, 30*time.Second)
				defer cancel()

				err := bot.BreakBlock(ctx, pos, feast.BreakOptions{AutoTool: true})
				if err != nil {
					sendChat(bot, fmt.Sprintf("break failed: %v", err))
				} else {
					sendChat(bot, "break done")
				}
			}()
		case "!mine":
			go func() {
				if !commandRunning.CompareAndSwap(false, true) {
					handleBadCommand(bot, "busy: command already running")
					return
				}
				defer commandRunning.Store(false)

				block, radius, ok := parseGotoArgs(msg)
				if !ok {
					handleBadCommand(bot, "usage: !mine <block> [radius]")
					return
				}

				fmt.Printf("[chatcmd] command=mine block=%s radius=%d\n", block, radius)

				// 1. Find the block
				matches := localFindBlocks(bot.World(), bot.Position(), block, 1, radius)
				if len(matches) == 0 {
					sendChat(bot, "mine failed: block not found")
					return
				}
				m := matches[0]

				// 2. Navigate near it
				sendChat(bot, fmt.Sprintf("mining %s: navigating to %d %d %d...", resolveBlockName(block), m.X, m.Y, m.Z))

				ctx, cancel := context.WithTimeout(mainCtx, 90*time.Second)
				defer cancel()

				var g goal.Goal
				sx, sy, sz, ok := findSafeStandingPositionNear(bot.World(), m.X, m.Y, m.Z)
				if ok {
					g = goal.NewGoalBlock(sx, sy, sz)
				} else {
					g = goal.NewGoalProximity(m.X, m.Y, m.Z, 2.0)
				}

				err := bot.NavigateTo(ctx, g)
				if err != nil {
					sendChat(bot, fmt.Sprintf("mine failed during navigation: %v", err))
					return
				}

				sendChat(bot, "arrived near block, selecting safe reachable block to break...")

				// 3. Select and break the block (search within 6 blocks of our new position)
				botPos := bot.Position()
				botFootX := int32(math.Floor(botPos.X))
				botFootY := int32(math.Floor(botPos.Y))
				botFootZ := int32(math.Floor(botPos.Z))

				breakMatches := localFindBlocks(bot.World(), botPos, block, 100, 6)
				if len(breakMatches) == 0 {
					sendChat(bot, "mine failed: block no longer found nearby")
					return
				}

				var selectedBlock *world.BlockHit
				for i := range breakMatches {
					bm := &breakMatches[i]
					pos := feast.BlockPos{X: int32(bm.X), Y: int32(bm.Y), Z: int32(bm.Z)}

					underFeet := pos.X == botFootX && pos.Z == botFootZ && pos.Y == botFootY-1

					minX := botPos.X - 0.3
					maxX := botPos.X + 0.3
					minZ := botPos.Z - 0.3
					maxZ := botPos.Z + 0.3
					blockMinX := float64(pos.X)
					blockMaxX := float64(pos.X) + 1.0
					blockMinZ := float64(pos.Z)
					blockMaxZ := float64(pos.Z) + 1.0

					inFootprint := pos.Y == botFootY-1 &&
						blockMinX <= maxX && blockMaxX >= minX &&
						blockMinZ <= maxZ && blockMaxZ >= minZ

					if underFeet {
						fmt.Printf("[break-select] rejected_under_feet=%d,%d,%d\n", pos.X, pos.Y, pos.Z)
						continue
					}
					if inFootprint {
						fmt.Printf("[break-select] rejected_support=%d,%d,%d\n", pos.X, pos.Y, pos.Z)
						continue
					}

					// Reachability check
					eyeX, eyeY, eyeZ := botPos.X, botPos.Y+1.62, botPos.Z
					tcX, tcY, tcZ := float64(pos.X)+0.5, float64(pos.Y)+0.5, float64(pos.Z)+0.5
					dx := tcX - eyeX
					dy := tcY - eyeY
					dz := tcZ - eyeZ
					dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
					if dist > 4.5 {
						continue
					}

					selectedBlock = bm
					break
				}

				if selectedBlock == nil {
					sendChat(bot, "mine failed: no safe reachable block found to break")
					return
				}

				pos := feast.BlockPos{X: int32(selectedBlock.X), Y: int32(selectedBlock.Y), Z: int32(selectedBlock.Z)}
				sendChat(bot, fmt.Sprintf("breaking %s at %d %d %d...", resolveBlockName(block), selectedBlock.X, selectedBlock.Y, selectedBlock.Z))

				err = bot.BreakBlock(ctx, pos, feast.BreakOptions{AutoTool: true})
				if err != nil {
					sendChat(bot, fmt.Sprintf("mine failed during break: %v", err))
				} else {
					sendChat(bot, "mine done")
				}
			}()
		default:
			if strings.HasPrefix(cmd, "!") {
				handleBadCommand(bot, fmt.Sprintf("unknown command: %s. Type !help for options.", cmd))
			}
		}
	})

	if err := bot.Connect(); err != nil {
		log.Fatalf("connect failed: %v", err)
	}
	defer bot.Disconnect()

	fmt.Println("[chatcmd] connected=true")

	// Wait until bot position syncs.
	waitCtx, waitCancel := context.WithTimeout(mainCtx, 15*time.Second)
	if err := bot.WaitUntilReady(waitCtx); err != nil {
		waitCancel()
		log.Fatalf("wait ready failed: %v", err)
	}
	waitCancel()

	fmt.Println("[chatcmd] ready=true")
	fmt.Printf("[chatcmd] username=%s\n", username)
	fmt.Println("[chatcmd] commands=!help,!pos,!inv,!find,!goto,!break,!mine,!stop")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
	case <-stopChan:
	}

	fmt.Println("[chatcmd] stopped=true")
}

func cleanChatMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}

	// If it contains ": ", let's split. Minecraft chat usually has "<Username>: <message>"
	if idx := strings.Index(msg, ": "); idx != -1 {
		// Extract part after ": "
		sub := strings.TrimSpace(msg[idx+2:])
		if strings.HasPrefix(sub, "!") {
			return sub
		}
	}

	// Check if it starts with "!"
	if strings.HasPrefix(msg, "!") {
		return msg
	}

	// If it's a system message from say command, e.g. "[Server] !help" or similar, check for "!"
	if idx := strings.Index(msg, "!"); idx != -1 {
		sub := strings.TrimSpace(msg[idx:])
		return sub
	}

	return ""
}

func resolveBlockName(input string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	input = strings.TrimPrefix(input, "minecraft:")
	switch input {
	case "oak", "wood", "log":
		return "oak_log"
	case "grass":
		return "grass_block"
	case "dirt":
		return "dirt"
	case "stone":
		return "stone"
	}
	return input
}

func parseFindArgs(msg string) (block string, count int, radius int, ok bool) {
	fields := strings.Fields(msg)
	if len(fields) < 2 {
		return "", 0, 0, false
	}
	block = fields[1]
	count = 1
	radius = 64

	if len(fields) >= 3 {
		var err error
		count, err = strconv.Atoi(fields[2])
		if err != nil {
			return "", 0, 0, false
		}
	}
	if len(fields) >= 4 {
		var err error
		radius, err = strconv.Atoi(fields[3])
		if err != nil {
			return "", 0, 0, false
		}
	}

	if count < 1 {
		count = 1
	} else if count > 20 {
		count = 20
	}

	if radius < 1 {
		radius = 1
	} else if radius > 128 {
		radius = 128
	}

	return block, count, radius, true
}

func parseGotoArgs(msg string) (block string, radius int, ok bool) {
	fields := strings.Fields(msg)
	if len(fields) < 2 {
		return "", 0, false
	}
	block = fields[1]
	radius = 64

	if len(fields) >= 3 {
		var err error
		radius, err = strconv.Atoi(fields[2])
		if err != nil {
			return "", 0, false
		}
	}

	if radius < 1 {
		radius = 1
	} else if radius > 128 {
		radius = 128
	}

	return block, radius, true
}

func parseBreakArgs(msg string) (block string, radius int, ok bool) {
	fields := strings.Fields(msg)
	if len(fields) < 2 {
		return "", 0, false
	}
	block = fields[1]
	radius = 64

	if len(fields) >= 3 {
		var err error
		radius, err = strconv.Atoi(fields[2])
		if err != nil {
			return "", 0, false
		}
	}

	if radius < 1 {
		radius = 1
	} else if radius > 128 {
		radius = 128
	}

	return block, radius, true
}

func localFindBlocks(w *world.World, botPos world.Vec3, blockName string, count int, radius int) []world.BlockHit {
	target := resolveBlockName(blockName)
	if target == "" {
		return nil
	}

	botX := botPos.X
	botY := botPos.Y
	botZ := botPos.Z

	minX := int(math.Floor(botX)) - radius
	maxX := int(math.Floor(botX)) + radius
	minZ := int(math.Floor(botZ)) - radius
	maxZ := int(math.Floor(botZ)) + radius
	radiusSq := float64(radius * radius)

	chunkCoords := w.ChunkCoords()
	var matches []world.BlockHit

	minY := world.MinY
	maxY := world.MaxY

	botYInt := int(math.Floor(botY))

	scanYRange := func(yMin, yMax int) {
		for _, cc := range chunkCoords {
			cx, cz := cc[0], cc[1]
			cMinX := cx * 16
			cMaxX := cx*16 + 15
			cMinZ := cz * 16
			cMaxZ := cz*16 + 15
			if cMaxX < minX || cMinX > maxX || cMaxZ < minZ || cMinZ > maxZ {
				continue
			}

			for lx := 0; lx < 16; lx++ {
				wx := cMinX + lx
				if wx < minX || wx > maxX {
					continue
				}
				dx := float64(wx) - botX
				for lz := 0; lz < 16; lz++ {
					wz := cMinZ + lz
					if wz < minZ || wz > maxZ {
						continue
					}
					dz := float64(wz) - botZ
					if dx*dx+dz*dz > radiusSq {
						continue
					}

					for wy := yMin; wy <= yMax; wy++ {
						dy := float64(wy) - botY
						distSq := dx*dx + dy*dy + dz*dz
						if distSq > radiusSq {
							continue
						}

						block, err := w.GetBlock(wx, wy, wz)
						if err != nil {
							continue
						}

						blockNorm := strings.TrimPrefix(strings.TrimSpace(strings.ToLower(block.Name)), "minecraft:")
						if blockNorm == target {
							matches = append(matches, world.BlockHit{
								X:        wx,
								Y:        wy,
								Z:        wz,
								Block:    block,
								Distance: math.Sqrt(distSq),
							})
						}
					}
				}
			}
		}
	}

	// Range 1: Near bot (within 16 blocks vertically)
	r1Min := botYInt - 16
	if r1Min < botYInt-radius {
		r1Min = botYInt - radius
	}
	if r1Min < minY {
		r1Min = minY
	}
	r1Max := botYInt + 16
	if r1Max > botYInt+radius {
		r1Max = botYInt + radius
	}
	if r1Max > maxY {
		r1Max = maxY
	}

	scanYRange(r1Min, r1Max)

	// Range 2 & 3: Fallback wider range if needed
	if len(matches) < count {
		r2Min := botYInt - radius
		if r2Min < minY {
			r2Min = minY
		}
		r2Max := r1Min - 1
		if r2Min <= r2Max {
			scanYRange(r2Min, r2Max)
		}

		r3Min := r1Max + 1
		r3Max := botYInt + radius
		if r3Max > maxY {
			r3Max = maxY
		}
		if r3Min <= r3Max {
			scanYRange(r3Min, r3Max)
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Distance < matches[j].Distance
	})

	if len(matches) > count {
		matches = matches[:count]
	}
	return matches
}

func isSafeStandingPosition(w *world.World, x, y, z int) bool {
	pos := world.BlockPos{X: int32(x), Y: int32(y), Z: int32(z)}
	if !w.IsBlockLoaded(pos) {
		return false
	}
	if !w.IsPassable(x, y, z) || !w.IsPassable(x, y+1, z) {
		return false
	}
	below := world.BlockPos{X: int32(x), Y: int32(y - 1), Z: int32(z)}
	if !w.IsBlockLoaded(below) || !w.IsSolid(below) {
		return false
	}
	return true
}

func findSafeStandingPositionNear(w *world.World, tx, ty, tz int) (int, int, int, bool) {
	var bestX, bestY, bestZ int
	bestDistSq := math.Inf(1)
	found := false

	for dx := -2; dx <= 2; dx++ {
		for dy := -2; dy <= 2; dy++ {
			for dz := -2; dz <= 2; dz++ {
				if dx == 0 && dy == 0 && dz == 0 {
					continue
				}
				cx := tx + dx
				cy := ty + dy
				cz := tz + dz
				if isSafeStandingPosition(w, cx, cy, cz) {
					distSq := float64(dx*dx + dy*dy + dz*dz)
					if distSq < bestDistSq {
						bestDistSq = distSq
						bestX, bestY, bestZ = cx, cy, cz
						found = true
					}
				}
			}
		}
	}
	return bestX, bestY, bestZ, found
}

var (
	lastBadCommandMu   sync.Mutex
	lastBadCommandTime time.Time
)

func sendChat(bot *feast.Client, msg string) {
	lines := strings.Split(msg, "\n")
	for _, line := range lines {
		line = strings.ReplaceAll(line, "\r", "")
		line = strings.TrimSpace(line)
		if line != "" {
			bot.Chat(line)
		}
	}
}

func handleBadCommand(bot *feast.Client, msg string) {
	lastBadCommandMu.Lock()
	defer lastBadCommandMu.Unlock()
	if time.Since(lastBadCommandTime) < 2*time.Second {
		fmt.Printf("[chatcmd] rate-limited message: %q\n", msg)
		return
	}
	lastBadCommandTime = time.Now()
	sendChat(bot, msg)
}
