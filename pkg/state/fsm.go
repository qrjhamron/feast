package state

import "fmt"

// State is a Minecraft connection state.
type State int

const (
	// StateHandshaking is the initial connection state.
	StateHandshaking State = iota
	// StateLogin is the login protocol state.
	StateLogin
	// StateConfiguration is the configuration protocol state.
	StateConfiguration
	// StatePlay is the play protocol state.
	StatePlay
)

// FSM tracks connection state transitions.
type FSM struct {
	current State
}

// NewFSM creates a new FSM at handshaking state.
func NewFSM() *FSM {
	return &FSM{current: StateHandshaking}
}

// Current returns the current state.
func (f *FSM) Current() State {
	return f.current
}

// Transition moves to a target state when the transition is allowed.
func (f *FSM) Transition(to State) error {
	if f.current == to {
		return nil
	}
	if !isAllowedTransition(f.current, to) {
		return fmt.Errorf("illegal transition: %v -> %v", f.current, to)
	}
	f.current = to
	return nil
}

func isAllowedTransition(from, to State) bool {
	switch from {
	case StateHandshaking:
		return to == StateLogin
	case StateLogin:
		return to == StateConfiguration
	case StateConfiguration:
		return to == StatePlay
	case StatePlay:
		return false
	default:
		return false
	}
}
