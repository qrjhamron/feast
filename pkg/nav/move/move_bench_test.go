package move

import (
	"testing"

	"github.com/qrjhamron/feast/pkg/world"
)

// benchWorld builds a 3x3 chunk flat stone field at y=63 so movement primitives
// exercise real (loaded) world lookups across chunk boundaries.
func benchWorld() *world.World {
	w := world.NewWorld()
	for cx := 0; cx < 3; cx++ {
		for cz := 0; cz < 3; cz++ {
			ch := world.NewChunk(cx, cz)
			for x := 0; x < 16; x++ {
				for z := 0; z < 16; z++ {
					ch.SetBlock(x, 63, z, world.BlockState{ID: 1, Name: "stone", Solid: true})
				}
			}
			w.AddChunk(ch)
		}
	}
	return w
}

func BenchmarkWalkNeighbors(b *testing.B) {
	w := benchWorld()
	from := [3]int{20, 64, 20}
	moves := []MoveWalk{{Dx: 1}, {Dx: -1}, {Dz: 1}, {Dz: -1}}
	b.ReportAllocs()
	b.ResetTimer()
	var acc float64
	for i := 0; i < b.N; i++ {
		for _, m := range moves {
			acc += m.Cost(w, from)
		}
	}
	_ = acc
}

func BenchmarkDiagonalCheck(b *testing.B) {
	w := benchWorld()
	from := [3]int{20, 64, 20}
	m := MoveWalkDiagonal{Dx: 1, Dz: 1}
	b.ReportAllocs()
	b.ResetTimer()
	var acc float64
	for i := 0; i < b.N; i++ {
		acc += m.Cost(w, from)
	}
	_ = acc
}

func BenchmarkJumpCheck(b *testing.B) {
	w := benchWorld()
	from := [3]int{20, 64, 20}
	m := MoveJump{Dx: 1}
	b.ReportAllocs()
	b.ResetTimer()
	var acc float64
	for i := 0; i < b.N; i++ {
		acc += m.Cost(w, from)
	}
	_ = acc
}

func BenchmarkFallCheck(b *testing.B) {
	w := benchWorld()
	from := [3]int{20, 64, 20}
	m := MoveFall{Dx: 1, Dy: -2}
	b.ReportAllocs()
	b.ResetTimer()
	var acc float64
	for i := 0; i < b.N; i++ {
		acc += m.Cost(w, from)
	}
	_ = acc
}

// BenchmarkMovePrimitiveGeneration evaluates the full per-node neighbor set the
// planner expands (cardinal walk/swim/parkour/jump/falls + diagonals), modeling
// the real A* expansion cost for one node.
func BenchmarkMovePrimitiveGeneration(b *testing.B) {
	w := benchWorld()
	from := [3]int{20, 64, 20}

	var all []Movement
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for _, d := range dirs {
		all = append(all, MoveWalk{Dx: d[0], Dz: d[1]})
		all = append(all, MoveParkour{Dx: d[0], Dz: d[1]})
		all = append(all, MoveJump{Dx: d[0], Dz: d[1]})
		for dy := -1; dy >= -8; dy-- {
			all = append(all, MoveFall{Dx: d[0], Dy: dy, Dz: d[1]})
		}
	}
	for _, d := range [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		all = append(all, MoveWalkDiagonal{Dx: d[0], Dz: d[1]})
	}

	b.ReportAllocs()
	b.ResetTimer()
	var acc float64
	for i := 0; i < b.N; i++ {
		for _, m := range all {
			acc += m.Cost(w, from)
		}
	}
	_ = acc
}
