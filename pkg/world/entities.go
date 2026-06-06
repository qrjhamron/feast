package world

import (
	"fmt"
	"math"
	"sync"

	"github.com/qrjhamron/feast/pkg/protocol"
)

type AABB struct {
	MinX, MinY, MinZ float64
	MaxX, MaxY, MaxZ float64
}

func (a AABB) Intersects(b AABB) bool {
	return a.MinX < b.MaxX && a.MaxX > b.MinX &&
		a.MinY < b.MaxY && a.MaxY > b.MinY &&
		a.MinZ < b.MaxZ && a.MaxZ > b.MinZ
}

// Entity represents one non-player or player entity tracked from play packets.
type Entity struct {
	ID       int32
	UUID     [16]byte
	Type     string
	X        float64
	Y        float64
	Z        float64
	Yaw      float32
	Pitch    float32
	VX       float64
	VY       float64
	VZ       float64
	Pose     protocol.EntityPose
	Width    float64
	Height   float64
	OnGround bool
	Metadata map[byte]protocol.EntityMetadataEntry
}

func EntityTypeFromID(id int32) string {
	switch id {
	case 2:
		return "item"
	case 64, 68, 73:
		return "pig"
	case 112, 114, 116:
		return "zombie"
	case 117, 120:
		return "player"
	default:
		return fmt.Sprintf("entity_%d", id)
	}
}

func HitboxForEntityType(entityType string, pose protocol.EntityPose) AABB {
	width := 0.6
	height := 1.8

	switch entityType {
	case "player":
		switch pose {
		case protocol.PoseSneaking:
			height = 1.5
		case protocol.PoseSwimming, protocol.PoseCrawling:
			height = 0.6
		default:
			height = 1.8
		}
	case "pig":
		width = 0.9
		height = 0.9
	case "zombie":
		width = 0.6
		height = 1.95
	default:
		width = 0.6
		height = 1.8
	}

	return AABB{
		MinX: -width / 2.0,
		MinY: 0.0,
		MinZ: -width / 2.0,
		MaxX: width / 2.0,
		MaxY: height,
		MaxZ: width / 2.0,
	}
}

func (e Entity) Hitbox() AABB {
	width := e.Width
	height := e.Height
	if width <= 0 || height <= 0 {
		base := HitboxForEntityType(e.Type, e.Pose)
		return AABB{
			MinX: e.X + base.MinX,
			MinY: e.Y + base.MinY,
			MinZ: e.Z + base.MinZ,
			MaxX: e.X + base.MaxX,
			MaxY: e.Y + base.MaxY,
			MaxZ: e.Z + base.MaxZ,
		}
	}
	return AABB{
		MinX: e.X - width/2.0,
		MinY: e.Y,
		MinZ: e.Z - width/2.0,
		MaxX: e.X + width/2.0,
		MaxY: e.Y + height,
		MaxZ: e.Z + width/2.0,
	}
}

// EntityStore is a thread-safe store keyed by entity ID.
type EntityStore struct {
	mu sync.RWMutex
	m  map[int32]*Entity
}

// NewEntityStore creates an empty entity store.
func NewEntityStore() *EntityStore {
	return &EntityStore{m: make(map[int32]*Entity)}
}

// Upsert inserts or replaces the entity by ID.
func (s *EntityStore) Upsert(e *Entity) {
	if s == nil || e == nil {
		return
	}
	s.mu.Lock()
	s.m[e.ID] = cloneEntity(e)
	s.mu.Unlock()
}

// Remove deletes an entity by ID.
func (s *EntityStore) Remove(id int32) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.m, id)
	s.mu.Unlock()
}

// Get returns a snapshot of one entity.
func (s *EntityStore) Get(id int32) (*Entity, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	e := s.m[id]
	if e == nil {
		s.mu.RUnlock()
		return nil, false
	}
	cp := cloneEntity(e)
	s.mu.RUnlock()
	return cp, true
}

// Minecraft relative entity movement deltas are encoded as signed shorts
// using 1/4096 block units.
func (s *EntityStore) UpdateDelta(id int32, dx, dy, dz int16) {
	if s == nil {
		return
	}
	s.mu.Lock()
	e := s.m[id]
	if e != nil {
		e.X += float64(dx) / 4096.0
		e.Y += float64(dy) / 4096.0
		e.Z += float64(dz) / 4096.0
	}
	s.mu.Unlock()
}

// UpdateRotation updates yaw/pitch in degrees for an entity, if present.
func (s *EntityStore) UpdateRotation(id int32, yaw, pitch float32) {
	if s == nil {
		return
	}
	s.mu.Lock()
	e := s.m[id]
	if e != nil {
		e.Yaw = yaw
		e.Pitch = pitch
	}
	s.mu.Unlock()
}

// UpdatePosition updates absolute position.
func (s *EntityStore) UpdatePosition(id int32, x, y, z float64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	e := s.m[id]
	if e != nil {
		e.X = x
		e.Y = y
		e.Z = z
	}
	s.mu.Unlock()
}

// Minecraft entity velocity components are encoded as signed shorts
// using 1/8000 block units per tick.
func (s *EntityStore) UpdateVelocity(id int32, vx, vy, vz int16) {
	if s == nil {
		return
	}
	s.mu.Lock()
	e := s.m[id]
	if e != nil {
		e.VX = float64(vx) / 8000.0
		e.VY = float64(vy) / 8000.0
		e.VZ = float64(vz) / 8000.0
	}
	s.mu.Unlock()
}

// All returns a snapshot of all entities.
func (s *EntityStore) All() []*Entity {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	out := make([]*Entity, 0, len(s.m))
	for _, e := range s.m {
		out = append(out, cloneEntity(e))
	}
	s.mu.RUnlock()
	return out
}

// Nearby returns entities within radius of the provided point.
func (s *EntityStore) Nearby(x, y, z float64, radius float64) []*Entity {
	if s == nil {
		return nil
	}
	if radius <= 0 {
		return nil
	}
	r2 := radius * radius
	s.mu.RLock()
	out := make([]*Entity, 0, 16)
	for _, e := range s.m {
		dx := e.X - x
		dy := e.Y - y
		dz := e.Z - z
		if dx*dx+dy*dy+dz*dz <= r2 || math.IsNaN(r2) {
			out = append(out, cloneEntity(e))
		}
	}
	s.mu.RUnlock()
	return out
}

// Reset clears all tracked entities.
func (s *EntityStore) Reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.m = make(map[int32]*Entity)
	s.mu.Unlock()
}

// anyIntersecting reports whether any tracked entity (other than excludeID)
// intersects box. It returns on the first hit and allocates nothing, making it
// the fast path for IsEntityBlocking on the navigation hot path.
func (s *EntityStore) anyIntersecting(box AABB, excludeID int32) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.m {
		if e.ID == excludeID {
			continue
		}
		if e.Hitbox().Intersects(box) {
			return true
		}
	}
	return false
}

func cloneEntity(e *Entity) *Entity {
	if e == nil {
		return nil
	}
	cp := *e
	if e.Metadata != nil {
		cp.Metadata = make(map[byte]protocol.EntityMetadataEntry)
		for k, v := range e.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp
}
