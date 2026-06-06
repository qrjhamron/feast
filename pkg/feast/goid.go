package feast

import (
	"runtime"
	"strconv"
)

// goID returns the numeric ID of the calling goroutine.
//
// Go intentionally does not expose goroutine IDs, so this parses the leading
// token of runtime.Stack ("goroutine <id> [running]:"). It is used solely to
// detect re-entrant [Client.Disconnect] calls made from within a
// client-managed goroutine (e.g. an event handler running on the read or
// heartbeat loop). A zero return means the ID could not be determined, which
// is treated as "not a managed goroutine".
func goID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	const prefix = "goroutine "
	s := buf[:n]
	if len(s) < len(prefix) {
		return 0
	}
	s = s[len(prefix):]
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	id, err := strconv.ParseUint(string(s[:i]), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// markManagedGoroutine records the current goroutine as client-managed and
// returns a function that clears the marking. Call it at the top of every
// goroutine the client owns and tracks via c.wg:
//
//	defer c.markManagedGoroutine()()
//
// This lets Disconnect detect when it is being invoked from within one of those
// goroutines (which would otherwise self-deadlock on c.wg.Wait).
func (c *Client) markManagedGoroutine() func() {
	id := goID()
	if id == 0 {
		return func() {}
	}
	c.managedGoids.Store(id, struct{}{})
	return func() { c.managedGoids.Delete(id) }
}

// inManagedGoroutine reports whether the caller is running on a client-managed
// goroutine (read loop, heartbeat loop, chunk-ingest worker, or navigation
// worker).
func (c *Client) inManagedGoroutine() bool {
	id := goID()
	if id == 0 {
		return false
	}
	_, ok := c.managedGoids.Load(id)
	return ok
}
