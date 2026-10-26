// Package identity derives the pseudonyms the demo seeder writes for its
// synthetic users, the same way an Omnia workspace does.
//
// Omnia keys every user-scoped memory by a pseudonym, never a raw identifier
// (memory-api's `virtual_user_id`). Since Omnia #3563 each workspace
// pseudonymises under its own key, derived from the install key and written by
// the operator into the `omnia-workspace-pseudonym-key` Secret (data key
// `key`) in the workspace namespace. A pod there reads it as
// OMNIA_PSEUDONYM_HMAC_KEY:
//
//	pseudonym   = hex(HMAC-SHA256(workspaceKey, raw))[:16]
//	fingerprint = hex(HMAC-SHA256(workspaceKey, "omnia-pseudonym-key-fingerprint"))[:16]
//
// An install with no key ("unkeyed") uses the plain SHA-256 form for both, and
// identities starting with "system:" are always hashed unkeyed. This is the
// algorithm Omnia documents for computing a pseudonym outside the cluster
// (privacy-api reference, "Compute a workspace key"); the tests pin it to
// Omnia's published vectors.
package identity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	// KeyEnv is the variable an Omnia workspace pod reads its workspace
	// pseudonym key from. The seed Job maps the Secret onto it.
	KeyEnv = "OMNIA_PSEUDONYM_HMAC_KEY"
	// KeyRequiredEnv, when "true", makes the seeder refuse to run without a
	// key, as Omnia's own pods do.
	KeyRequiredEnv = "OMNIA_PSEUDONYM_KEY_REQUIRED"
	// FingerprintHeader states the caller's key fingerprint on a request;
	// memory-api refuses a mismatch with 409 pseudonym_key_mismatch.
	FingerprintHeader = "X-Omnia-Pseudonym-Key-Fingerprint"
	// BoundFingerprintHeader is on every memory-api answer, /healthz included:
	// the fingerprint its database is bound to.
	BoundFingerprintHeader = "X-Omnia-Pseudonym-Key-Bound"
	// KeyMismatchError is the `error` of memory-api's 409 for a foreign key.
	KeyMismatchError = "pseudonym_key_mismatch"
	// UnkeyedFingerprint is the fingerprint of an install with no key.
	UnkeyedFingerprint = "unkeyed"

	pseudonymLength       = 16
	fingerprintBytes      = 8
	fingerprintLabel      = "omnia-pseudonym-key-fingerprint"
	systemPrincipalPrefix = "system:"
)

// Pseudonymizer derives pseudonyms under one workspace key. The zero value is
// the unkeyed form. It never exposes the key.
type Pseudonymizer struct {
	key string
}

// New returns a Pseudonymizer for workspaceKey ("" for an unkeyed install).
func New(workspaceKey string) Pseudonymizer {
	return Pseudonymizer{key: workspaceKey}
}

// Keyed reports whether a workspace key is set.
func (p Pseudonymizer) Keyed() bool { return p.key != "" }

// ID returns the 16-hex-character pseudonym of raw. Empty input stays empty so
// an absent identity is never turned into a user.
func (p Pseudonymizer) ID(raw string) string {
	if raw == "" {
		return ""
	}
	if p.key == "" || strings.HasPrefix(raw, systemPrincipalPrefix) {
		sum := sha256.Sum256([]byte(raw))
		return hex.EncodeToString(sum[:])[:pseudonymLength]
	}
	return hmacHex(p.key, raw)[:pseudonymLength]
}

// Fingerprint identifies the key without revealing it, exactly as memory-api
// records it in memory_meta; "unkeyed" for no key.
func (p Pseudonymizer) Fingerprint() string {
	if p.key == "" {
		return UnkeyedFingerprint
	}
	return hmacHex(p.key, fingerprintLabel)[:2*fingerprintBytes]
}

func hmacHex(key, msg string) string {
	h := hmac.New(sha256.New, []byte(key))
	_, _ = h.Write([]byte(msg))
	return hex.EncodeToString(h.Sum(nil))
}
