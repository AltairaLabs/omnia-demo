// Package identity is the Personal Agent's identity under the Personal Agent
// Protocol (PAP 1.0, https://personalagentprotocol.org/docs/1.0/spec): its
// client metadata document (§4.1), its public key set, the delegations it
// signs with its identity keys, and the Session JWTs (§5.2) signed with the
// delegated keys. It is written from the spec, using only the standard
// library and github.com/golang-jwt/jwt/v5.
package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Identity is the Personal Agent's long-lived identity: the ES256 and PS384
// keys published at JWKSURI, and the URLs of its metadata document.
type Identity struct {
	ClientID    string
	JWKSURI     string
	RedirectURI string

	es256    *ecdsa.PrivateKey
	es256Kid string
	ps384    *rsa.PrivateKey
	jwks     []byte
}

// Load reads es256.pem (P-256) and ps384.pem (RSA, at least 2048 bits), both
// PKCS#8, from dir. base is the HTTPS origin the agent is served from, such
// as https://assistant.example.com.
func Load(dir, base string) (*Identity, error) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("identity: base %q must be an https origin", base)
	}
	base = strings.TrimSuffix(base, "/")

	ecKey, err := readPKCS8(filepath.Join(dir, "es256.pem"))
	if err != nil {
		return nil, err
	}
	es, ok := ecKey.(*ecdsa.PrivateKey)
	if !ok || es.Curve != elliptic.P256() {
		return nil, errors.New("identity: es256.pem is not a P-256 key")
	}
	rsaKey, err := readPKCS8(filepath.Join(dir, "ps384.pem"))
	if err != nil {
		return nil, err
	}
	ps, ok := rsaKey.(*rsa.PrivateKey)
	if !ok || ps.N.BitLen() < 2048 {
		return nil, errors.New("identity: ps384.pem is not an RSA key of at least 2048 bits")
	}

	id := &Identity{
		ClientID:    base + "/agent.json",
		JWKSURI:     base + "/jwks.json",
		RedirectURI: base + "/signin/callback",
		es256:       es,
		ps384:       ps,
	}
	if id.es256Kid, err = Thumbprint(&es.PublicKey); err != nil {
		return nil, err
	}
	if id.jwks, err = buildJWKS(&es.PublicKey, &ps.PublicKey); err != nil {
		return nil, err
	}
	return id, nil
}

func readPKCS8(path string) (any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("identity: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("identity: %s is not a PKCS#8 PEM", path)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("identity: %s: %w", path, err)
	}
	return key, nil
}

// randomID returns 128 bits from crypto/rand, base64url without padding.
func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error.
	return base64.RawURLEncoding.EncodeToString(b)
}
