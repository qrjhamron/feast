package feast

import "testing"

// TestLastMovementStatsNotStaleAfterFailure verifies that an overall navigation
// failure clears a stale per-segment Reached=true left in the stats.
func TestLastMovementStatsNotStaleAfterFailure(t *testing.T) {
	c := NewClient(Options{})
	// Simulate a successful executor segment leaving Reached=true.
	c.statsTrackMu.Lock()
	c.lastMovementStats.Reached = true
	c.lastMovementStats.PacketsSent = 42
	c.statsTrackMu.Unlock()

	c.logNavFailed("stuck_after_replans")

	got := c.LastMovementStats()
	if got.Reached {
		t.Fatal("LastMovementStats().Reached must be false after a navigation failure")
	}
	if got.PacketsSent != 42 {
		t.Fatalf("packet count must be preserved, got %d", got.PacketsSent)
	}
}

// TestNavigateOverallFailureOverridesStepSuccess verifies the route result wins
// over a per-segment success when reporting the final navigation outcome.
func TestNavigateOverallFailureOverridesStepSuccess(t *testing.T) {
	c := NewClient(Options{})
	c.setNavReached(true)
	if !c.LastMovementStats().Reached {
		t.Fatal("precondition: expected Reached=true")
	}
	c.logNavFailed("no_path")
	if c.LastMovementStats().Reached {
		t.Fatal("overall failure must override step success")
	}

	// Arrival flips it back to true.
	c.logNavArrived(1, 2, 3)
	if !c.LastMovementStats().Reached {
		t.Fatal("arrival must report Reached=true")
	}
}
