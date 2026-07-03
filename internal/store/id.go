package store

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID returns a short random hex identifier, good enough for a
// single-user local data file.
func NewID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
