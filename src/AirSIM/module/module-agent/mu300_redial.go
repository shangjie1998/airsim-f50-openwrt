package main

import (
	"crypto/sha256"
	"encoding/hex"
)

// Operation journals store digests instead of raw dial parameters or owners.
// There is no per-number redial cooldown. Old dial.json is left untouched for
// backup/rollback; execution deduplication uses operations.json instead.
func mu300DialHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
