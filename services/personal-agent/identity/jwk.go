package identity

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
)

// ecJWK is a P-256 public JWK. Its fields are the RFC 7638 required members
// in lexicographic order, so its JSON encoding is the thumbprint input.
type ecJWK struct {
	Crv string `json:"crv"`
	Kty string `json:"kty"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// rsaJWK is an RSA public JWK, its fields likewise the RFC 7638 required
// members in lexicographic order.
type rsaJWK struct {
	E   string `json:"e"`
	Kty string `json:"kty"`
	N   string `json:"n"`
}

// publishedJWK is a jwks.json entry: the public members plus kid, use, alg.
type publishedJWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`
	N   string `json:"n,omitempty"`
	E   string `json:"e,omitempty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
}

var b64 = base64.RawURLEncoding

// ecPublicJWK encodes a P-256 key with x and y as fixed-length 32-byte
// big-endian values (RFC 7518 §6.2.1.2).
func ecPublicJWK(pub *ecdsa.PublicKey) (ecJWK, error) {
	if pub.Curve != elliptic.P256() {
		return ecJWK{}, errors.New("identity: only P-256 EC keys are supported")
	}
	raw, err := pub.Bytes() // 0x04 || X (32 bytes) || Y (32 bytes)
	if err != nil {
		return ecJWK{}, fmt.Errorf("identity: %w", err)
	}
	return ecJWK{Crv: "P-256", Kty: "EC", X: b64.EncodeToString(raw[1:33]), Y: b64.EncodeToString(raw[33:65])}, nil
}

func rsaPublicJWK(pub *rsa.PublicKey) rsaJWK {
	return rsaJWK{
		E:   b64.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		Kty: "RSA",
		N:   b64.EncodeToString(pub.N.Bytes()),
	}
}

// Thumbprint is the RFC 7638 JWK thumbprint of a P-256 or RSA public key:
// SHA-256 over the required members in lexicographic order with no
// whitespace, base64url-encoded without padding.
func Thumbprint(pub crypto.PublicKey) (string, error) {
	var members any
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		jwk, err := ecPublicJWK(k)
		if err != nil {
			return "", err
		}
		members = jwk
	case *rsa.PublicKey:
		members = rsaPublicJWK(k)
	default:
		return "", fmt.Errorf("identity: unsupported key type %T", pub)
	}
	// encoding/json emits struct fields in declaration order with no
	// whitespace, and base64url values and "P-256" need no escaping.
	input, err := json.Marshal(members)
	if err != nil {
		return "", fmt.Errorf("identity: %w", err)
	}
	sum := sha256.Sum256(input)
	return b64.EncodeToString(sum[:]), nil
}

func buildJWKS(es *ecdsa.PublicKey, ps *rsa.PublicKey) ([]byte, error) {
	ec, err := ecPublicJWK(es)
	if err != nil {
		return nil, err
	}
	ecKid, err := Thumbprint(es)
	if err != nil {
		return nil, err
	}
	rs := rsaPublicJWK(ps)
	rsKid, err := Thumbprint(ps)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string][]publishedJWK{"keys": {
		{Kty: ec.Kty, Crv: ec.Crv, X: ec.X, Y: ec.Y, Kid: ecKid, Use: "sig", Alg: "ES256"},
		{Kty: rs.Kty, N: rs.N, E: rs.E, Kid: rsKid, Use: "sig", Alg: "PS384"},
	}})
}

// JWKS is the jwks.json document: the identity public keys, each with kid
// set to its RFC 7638 thumbprint, use "sig" and alg ES256 or PS384.
func (i *Identity) JWKS() []byte {
	return append([]byte(nil), i.jwks...)
}

// Metadata is the client metadata document served at ClientID (PAP §4.1).
func (i *Identity) Metadata() map[string]any {
	return map[string]any{
		"client_id":                  i.ClientID,
		"client_name":                "Ellen's assistant",
		"jwks_uri":                   i.JWKSURI,
		"redirect_uris":              []string{i.RedirectURI},
		"token_endpoint_auth_method": "private_key_jwt",
	}
}
