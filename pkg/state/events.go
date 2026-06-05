package state

import "github.com/user/feastgo/pkg/protocol"

// Event is the common interface for all state events.
type Event interface {
	EventType() string
}

// SpawnEvent is emitted when player spawn/sync position is received.
type SpawnEvent struct {
	EntityID int32
	X        float64
	Y        float64
	Z        float64
}

// EventType returns event key.
func (e SpawnEvent) EventType() string { return "spawn" }

// PlayLoginEvent is emitted when the Play Login packet is received.
type PlayLoginEvent struct {
	EntityID int32
}

// EventType returns event key.
func (e PlayLoginEvent) EventType() string { return "play_login" }

// PositionEvent is emitted when the server synchronizes player position.
type PositionEvent struct {
	X          float64
	Y          float64
	Z          float64
	Yaw        float32
	Pitch      float32
	Flags      byte
	TeleportID int32
}

// EventType returns event key.
func (e PositionEvent) EventType() string { return "position" }

// ChatEvent is emitted when chat is received.
type ChatEvent struct {
	Sender  string
	Message string
}

// EventType returns event key.
func (e ChatEvent) EventType() string { return "chat" }

// KeepAliveEvent is emitted when a keepalive packet is received.
type KeepAliveEvent struct {
	ID int64
}

// EventType returns event key.
func (e KeepAliveEvent) EventType() string { return "keep_alive" }

// HealthEvent is emitted when player health/food changes.
type HealthEvent struct {
	Health     float32
	Food       int32
	Saturation float32
}

// EventType returns event key.
func (e HealthEvent) EventType() string { return "health" }

// KickEvent is emitted when server kicks/disconnects connection.
type KickEvent struct {
	Reason string
}

// EventType returns event key.
func (e KickEvent) EventType() string { return "kick" }

// LoginEvent is emitted on login success.
type LoginEvent struct {
	UUID     string
	RawUUID  [16]byte
	Username string
}

// EventType returns event key.
func (e LoginEvent) EventType() string { return "login" }

// DisconnectEvent is emitted on stream disconnect packet.
type DisconnectEvent struct {
	Reason string
	Clean  bool
}

// EventType returns event key.
func (e DisconnectEvent) EventType() string { return "disconnect" }

// ErrorEvent is emitted when the runtime sees a non-fatal or fatal loop error.
type ErrorEvent struct {
	Op    string
	Error error
}

// EventType returns event key.
func (e ErrorEvent) EventType() string { return "error" }

// PacketEvent is a catchall raw packet event.
type PacketEvent struct {
	ID   int32
	Data []byte
}

// EventType returns event key.
func (e PacketEvent) EventType() string { return "packet" }

// NavStartEvent is emitted when navigation starts.
type NavStartEvent struct {
	FromX int
	FromY int
	FromZ int
	ToX   int
	ToY   int
	ToZ   int
}

// EventType returns event key.
func (e NavStartEvent) EventType() string { return "nav_start" }

// NavArrivedEvent is emitted when navigation reaches target.
type NavArrivedEvent struct {
	X int
	Y int
	Z int
}

// EventType returns event key.
func (e NavArrivedEvent) EventType() string { return "nav_arrived" }

// NavFailedEvent is emitted when navigation fails.
type NavFailedEvent struct {
	Reason string
}

// EventType returns event key.
func (e NavFailedEvent) EventType() string { return "nav_failed" }

// NavStepEvent is emitted for each navigation movement packet.
type NavStepEvent struct {
	X float64
	Y float64
	Z float64
}

// EventType returns event key.
func (e NavStepEvent) EventType() string { return "nav_step" }

// NavStuckEvent is emitted when navigation appears stuck.
type NavStuckEvent struct {
	X      int
	Y      int
	Z      int
	Reason string
}

// EventType returns event key.
func (e NavStuckEvent) EventType() string { return "nav_stuck" }

// BlockUpdateEvent is emitted on a single block state change.
type BlockUpdateEvent struct {
	X       int32
	Y       int32
	Z       int32
	StateID int32
}

// EventType returns event key.
func (e BlockUpdateEvent) EventType() string { return "block_update" }

// SectionBlockUpdate is one decoded world update from section batch packet.
type SectionBlockUpdate struct {
	X       int32
	Y       int32
	Z       int32
	StateID int32
}

// SectionBlocksUpdateEvent is emitted for batched section block updates.
type SectionBlocksUpdateEvent struct {
	Updates []SectionBlockUpdate
}

// EventType returns event key.
func (e SectionBlocksUpdateEvent) EventType() string { return "section_blocks_update" }

// EntityMoveDeltaEvent is emitted for entity relative movement updates.
type EntityMoveDeltaEvent struct {
	EntityID int32
	DX       int16
	DY       int16
	DZ       int16
}

// EventType returns event key.
func (e EntityMoveDeltaEvent) EventType() string { return "entity_move_delta" }

// EntityRotateEvent is emitted for entity rotation updates.
type EntityRotateEvent struct {
	EntityID int32
	Yaw      byte
	Pitch    byte
	OnGround bool
}

// EventType returns event key.
func (e EntityRotateEvent) EventType() string { return "entity_rotate" }

// TimeUpdateEvent is emitted when world time is updated.
type TimeUpdateEvent struct {
	WorldAge  int64
	TimeOfDay int64
}

// EventType returns event key.
func (e TimeUpdateEvent) EventType() string { return "time_update" }

// PlayerInfoUpdateEvent is emitted for player info updates.
type PlayerInfoUpdateEvent struct {
	Actions byte
	Players []protocol.PlayClientboundPlayerInfoUpdatePlayer
}

// EventType returns event key.
func (e PlayerInfoUpdateEvent) EventType() string { return "player_info_update" }

// HeldItemEvent is emitted when the selected hotbar slot changes.
type HeldItemEvent struct {
	Slot int
}

// EventType returns event key.
func (e HeldItemEvent) EventType() string { return "held_item" }

// InventorySlotEvent is emitted for minimal player inventory slot updates.
type InventorySlotEvent struct {
	WindowID int32
	StateID  int32
	Slot     int16
	Item     protocol.ItemStack
}

// EventType returns event key.
func (e InventorySlotEvent) EventType() string { return "inventory_slot" }

// ContainerContentEvent is emitted for full player inventory slot updates.
type ContainerContentEvent struct {
	WindowID int32
	StateID  int32
	Slots    []protocol.ItemStack
}

// EventType returns event key.
func (e ContainerContentEvent) EventType() string { return "container_content" }

// ChunkLoadEvent is emitted when a chunk payload is received.
type ChunkLoadEvent struct {
	ChunkX int32
	ChunkZ int32
}

// EventType returns event key.
func (e ChunkLoadEvent) EventType() string { return "chunk_load" }

// EntitySpawnEvent is emitted when an entity spawns.
type EntitySpawnEvent struct {
	EntityID int32
	UUID     [16]byte
	Type     int32
	X        float64
	Y        float64
	Z        float64
}

// EventType returns event key.
func (e EntitySpawnEvent) EventType() string { return "entity_spawn" }

// EntityRemoveEvent is emitted when an entity is removed.
type EntityRemoveEvent struct {
	EntityID int32
}

// EventType returns event key.
func (e EntityRemoveEvent) EventType() string { return "entity_remove" }

// EntityVelocityEvent is emitted when an entity's velocity is set.
type EntityVelocityEvent struct {
	EntityID  int32
	VelocityX int16
	VelocityY int16
	VelocityZ int16
}

// EventType returns event key.
func (e EntityVelocityEvent) EventType() string { return "entity_velocity" }

// EntityTeleportEvent is emitted when an entity is teleported.
type EntityTeleportEvent struct {
	EntityID int32
	X        float64
	Y        float64
	Z        float64
	Yaw      byte
	Pitch    byte
	OnGround bool
}

// EventType returns event key.
func (e EntityTeleportEvent) EventType() string { return "entity_teleport" }

// EntityMetadataUpdateEvent is emitted when an entity's metadata changes.
type EntityMetadataUpdateEvent struct {
	EntityID int32
	Metadata []protocol.EntityMetadataEntry
}

// EventType returns event key.
func (e EntityMetadataUpdateEvent) EventType() string { return "entity_metadata" }
