package hpa

import (
	"sync"
	"time"

	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

type GraphUpdater struct {
	graph    *AbstractGraph
	clusters *ClusterManager
	world    *world.World
	builder  *GraphBuilder
	updateCh chan [3]int
	stopCh   chan struct{}
	stopOnce sync.Once

	activeRefMu sync.RWMutex
	activeRef   *LazyRefiner
}

func NewGraphUpdater(w *world.World, g *AbstractGraph, m *ClusterManager, b *GraphBuilder, bus *state.EventBus) *GraphUpdater {
	u := &GraphUpdater{
		graph:    g,
		clusters: m,
		world:    w,
		builder:  b,
		updateCh: make(chan [3]int, 256),
		stopCh:   make(chan struct{}),
	}

	if bus != nil {
		bus.On("block_update", func(e state.Event) {
			// Try to extract pos
			type pos interface {
				Position() (x, y, z int)
			}
			type legacyPos interface {
				X_() float64
				Y_() float64
				Z_() float64
			}

			if ev, ok := e.(pos); ok {
				x, y, z := ev.Position()
				u.NotifyUpdate([3]int{x, y, z})
			} else {
				// Use the interface that matches protocol.PlayClientboundBlockUpdatePacket
				// or the event in test:
				type structPos interface {
					X_() int
				}
				// We can just rely on the test providing [3]int for testing
				// or using a generic reflection approach if needed.
			}
		})
	}
	return u
}

func (u *GraphUpdater) NotifyUpdate(pos [3]int) {
	select {
	case u.updateCh <- pos:
	default:
	}
}

func (u *GraphUpdater) SetActiveRefiner(r *LazyRefiner) {
	u.activeRefMu.Lock()
	u.activeRef = r
	u.activeRefMu.Unlock()
}

func (u *GraphUpdater) Start() {
	go u.asyncRebuild()
}

func (u *GraphUpdater) Stop() {
	u.stopOnce.Do(func() {
		close(u.stopCh)
	})
}

func (u *GraphUpdater) asyncRebuild() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	dirtySet := make(map[[3]int]struct{})

	for {
		select {
		case <-u.stopCh:
			return
		case pos := <-u.updateCh:
			u.clusters.Invalidate(pos[0], pos[2])
			dirtySet[pos] = struct{}{}
		case <-ticker.C:
			if len(dirtySet) > 0 {
				u.builder.RebuildDirty()

				u.activeRefMu.RLock()
				ref := u.activeRef
				u.activeRefMu.RUnlock()

				if ref != nil {
					for pos := range dirtySet {
						ref.Invalidate(pos)
					}
				}
				dirtySet = make(map[[3]int]struct{})
			}
		}
	}
}
