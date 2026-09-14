// Package identity provides the deterministic pseudonym used by the demo
// seeder for synthetic user identifiers.
package identity

import (
	"crypto/sha256"
	"encoding/hex"
)

// PseudonymizeID returns a non-reversible 16-character pseudonym. Empty input
// remains empty so an absent identity is never turned into a real user.
func PseudonymizeID(raw string) string {
	if raw == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:16]
}
