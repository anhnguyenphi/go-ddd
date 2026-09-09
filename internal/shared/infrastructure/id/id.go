// Package id generates opaque unique identifiers (RFC 4122 v4 UUIDs) with no
// external dependencies. UUID and Fixed implement application.IDGenerator.
package id

import (
	"crypto/rand"
	"encoding/hex"
)

// UUID is the default crypto/rand-backed generator.
type UUID struct{}

func (UUID) NewID() string { return New() }

// New returns a random UUIDv4 string, e.g. "9f1b9d3e-...-...".
func New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("id: entropy source failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10

	var buf [36]byte
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])
	return string(buf[:])
}

// Fixed is a deterministic generator for tests.
type Fixed struct{ Value string }

func (f Fixed) NewID() string { return f.Value }
