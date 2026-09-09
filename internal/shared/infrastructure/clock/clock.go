// Package clock provides implementations of the application.Clock port so
// domain and application code stay deterministic under test.
package clock

import "time"

// System is the real clock. It always returns UTC. Implements application.Clock.
type System struct{}

func (System) Now() time.Time { return time.Now().UTC() }

// Fixed is a test clock that always returns At. Implements application.Clock.
type Fixed struct{ At time.Time }

func (f Fixed) Now() time.Time { return f.At }
