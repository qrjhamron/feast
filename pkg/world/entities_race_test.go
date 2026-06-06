package world

import (
	"sync"
	"testing"
)

func TestEntityStoreConcurrentReadWrite(t *testing.T) {
	t.Parallel()

	s := NewEntityStore()
	const n = 64

	for i := 0; i < n; i++ {
		s.Upsert(&Entity{ID: int32(i), X: float64(i), Y: 64, Z: float64(-i)})
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		id := int32(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				s.UpdateDelta(id, 1, 0, -1)
				s.UpdateRotation(id, float32(j%360), float32((j*2)%360))
				if j%17 == 0 {
					s.Upsert(&Entity{ID: id, X: float64(j), Y: 64, Z: float64(-j)})
				}
			}
		}()
	}

	for i := 0; i < n; i++ {
		id := int32(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_, _ = s.Get(id)
				_ = s.All()
				_ = s.Nearby(0, 64, 0, 128)
			}
		}()
	}

	wg.Wait()
}
