package goal

import "testing"

func BenchmarkGoalBlockReached(b *testing.B) {
	g := NewGoalBlock(128, 70, -64)
	b.ReportAllocs()
	b.ResetTimer()
	var hit bool
	for i := 0; i < b.N; i++ {
		hit = g.Satisfied(i&255, 70, -64)
	}
	_ = hit
}

func BenchmarkGoalCompositeReached(b *testing.B) {
	g := NewGoalComposite(CompositeAll, NewGoalXZ(40, 40), NewGoalY(64))
	b.ReportAllocs()
	b.ResetTimer()
	var hit bool
	for i := 0; i < b.N; i++ {
		hit = g.Satisfied(40, 64, 40)
	}
	_ = hit
}

func BenchmarkGoalHeuristic(b *testing.B) {
	g := NewGoalBlock(200, 80, 200)
	b.ReportAllocs()
	b.ResetTimer()
	var h float64
	for i := 0; i < b.N; i++ {
		// Sweep coordinates so the benchmark exercises the full octile path
		// (both axes, plus the vertical term) rather than a constant.
		h += g.Heuristic(i&127, 64+(i&15), i&63)
	}
	_ = h
}
