// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

// Package demoidp is the identity provider the demo's front doors sign their
// users in with: the Larkmere member portal, the Wrenmoor customer portal and
// Larkmere's scripted telephony callers.
//
// A STAND-IN (docs/stand-ins.md). A real deployment's edge (an API gateway, an
// Istio RequestAuthentication) verifies the user's token from the customer's
// identity provider and forwards both the claim headers edgeTrust reads and the
// token itself. Omnia's mint exchanges that token for the subject proof the
// runtime needs before it gives the conversation a user: user-scoped memory,
// and anything else keyed on a verified user, depends on it. An edge that
// forwards headers and no token gets a conversation with no verified user.
//
// So each front door signs a short-lived RS256 token for the user it asserts,
// and serves the public key at JWKSPath. The install lists that issuer in the
// operator's delegation.trustedIssuers ("<issuer>=<jwksURL>"), the same as it
// would list Entra or Okta. Nobody signs in to it: it vouches for the demo's
// fixed, fictional users, which is all a demo needs. The key is generated at
// start and never written down; a restart rotates it, and the mint refetches
// the JWKS when it meets an unknown key id.
package demoidp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWKSPath is where an issuer serves its public key.
const JWKSPath = "/.well-known/jwks.json"

// Issuer signs tokens for the users one front door asserts.
type Issuer struct {
	url string
	key *rsa.PrivateKey
	kid string
}

// New returns an issuer named url, with a fresh key. url is the token's `iss`,
// and the install trusts url + JWKSPath as its JWKS.
func New(url string) (*Issuer, error) {
	if url == "" {
		return nil, errors.New("demoidp: issuer url is required")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(key.N.Bytes())
	return &Issuer{url: url, key: key, kid: hex.EncodeToString(sum[:8])}, nil
}

// URL is the issuer's name, the `iss` of every token it signs.
func (i *Issuer) URL() string { return i.url }

// Sign returns a token naming sub, valid for ttl. Omnia's mint reads `sub` as
// the user, derives the workspace pseudonym from it, and lets nothing it mints
// from the token outlive `exp`, so ttl must cover the conversation.
func (i *Issuer) Sign(sub, email string, ttl time.Duration) (string, error) {
	return i.SignFor(sub, email, "", ttl)
}

// SignFor is Sign with an audience: the token's `aud`, omitted when empty.
//
// Omnia's end-user plane (management-api's /api/v1/me routes, a Workspace's
// spec.endUserAuth.oidc) requires an `aud` and matches it exactly. Its docs
// ask for an audience of its own, never an agent's, so a token minted for a
// conversation cannot also open the person's privacy controls.
func (i *Issuer) SignFor(sub, email, audience string, ttl time.Duration) (string, error) {
	if sub == "" {
		return "", errors.New("demoidp: a token must name a subject")
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": i.url,
		"sub": sub,
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"exp": now.Add(ttl).Unix(),
	}
	if audience != "" {
		claims["aud"] = audience
	}
	if email != "" {
		claims["email"] = email
	}
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	t.Header["kid"] = i.kid
	return t.SignedString(i.key)
}

// DiscoveryPath is where an issuer serves its OpenID Connect discovery
// document. Omnia's end-user plane finds the JWKS through it, not from a
// configured URL, and refuses a document naming another issuer.
const DiscoveryPath = "/.well-known/openid-configuration"

// ServeDiscovery serves the issuer's discovery document: the fields a token
// verifier reads. There is no authorization endpoint, because nobody signs in
// to this issuer.
func (i *Issuer) ServeDiscovery(w http.ResponseWriter, _ *http.Request) {
	doc := map[string]any{
		"issuer":                                i.url,
		"jwks_uri":                              i.url + JWKSPath,
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"subject_types_supported":               []string{"public"},
		"response_types_supported":              []string{"id_token"},
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "max-age=300")
	_ = json.NewEncoder(w).Encode(doc)
}

// ServeJWKS serves the issuer's public key as a JWK set.
func (i *Issuer) ServeJWKS(w http.ResponseWriter, _ *http.Request) {
	pub := i.key.PublicKey
	set := map[string]any{"keys": []map[string]string{{
		"kty": "RSA",
		"use": "sig",
		"alg": "RS256",
		"kid": i.kid,
		"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "max-age=300")
	_ = json.NewEncoder(w).Encode(set)
}
