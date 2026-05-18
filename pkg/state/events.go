package state

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
