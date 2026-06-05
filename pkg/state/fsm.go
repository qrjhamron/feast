package state

import (
	"fmt"
	"sync"
)

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
	// StateDisconnected is the disconnected state.
	StateDisconnected
)

// FSM tracks connection state transitions.
type FSM struct {
	mu      sync.RWMutex
	current State
}

// NewFSM creates a new FSM at handshaking state.
func NewFSM() *FSM {
	return &FSM{current: StateHandshaking}
}

// Current returns the current state.
func (f *FSM) Current() State {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.current
}

// Transition moves to a target state when the transition is allowed.
func (f *FSM) Transition(to State) error {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	if to == StateDisconnected {
		return true
	}
	switch from {
	case StateHandshaking:
		return to == StateLogin
	case StateLogin:
		return to == StateConfiguration
	case StateConfiguration:
		return to == StatePlay
	default:
		return false
	}
}
