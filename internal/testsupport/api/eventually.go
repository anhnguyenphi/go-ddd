package api

import (
	"testing"
	"time"
)

// Eventually polls cond every 100ms until it returns ok or timeout elapses,
// then fails the test with the last reason cond gave. Use it wherever the
// read side is eventually consistent (projections fed by the outbox/bus).
func Eventually(t testing.TB, timeout time.Duration, cond func() (ok bool, reason string)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var reason string
	for {
		var ok bool
		ok, reason = cond()
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %s: %s", timeout, reason)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// AwaitStatus polls GET path until it returns want, and returns that response.
func (c *Client) AwaitStatus(t testing.TB, path string, want int, timeout time.Duration) Response {
	t.Helper()
	var last Response
	Eventually(t, timeout, func() (bool, string) {
		res, err := c.Try("GET", path, nil)
		if err != nil {
			return false, err.Error()
		}
		last = res
		return res.Status == want, "GET " + path + " -> " + string(res.Body)
	})
	return last
}
