package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// delegationLifetime is the longest a delegation may live (PAP §5.2).
const delegationLifetime = 24 * time.Hour

// Delegated is a short-lived ES256 key and the delegation, signed by the
// identity ES256 key, that binds it to this Personal Agent.
type Delegated struct {
	// KID is the RFC 7638 thumbprint of the delegated public key.
	KID string
	// Delegation is the compact poppy-delegation+jwt.
	Delegation string
	// Expiry is the delegation's exp.
	Expiry time.Time

	key    *ecdsa.PrivateKey
	issuer string
}

// Delegate generates a fresh P-256 key and signs a delegation for it with
// the identity ES256 key, valid from now for 24 hours. The delegation names
// only this Personal Agent and the key: no User ID, Agent ID or Company.
func (i *Identity) Delegate(now time.Time) (*Delegated, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("identity: %w", err)
	}
	jwk, err := ecPublicJWK(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	kid, err := Thumbprint(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	exp := now.Add(delegationLifetime)
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": i.ClientID,
		"cnf": map[string]any{"jwk": jwk},
		"iat": now.Unix(),
		"exp": exp.Unix(),
		"jti": randomID(),
	})
	tok.Header["typ"] = "poppy-delegation+jwt"
	tok.Header["kid"] = i.es256Kid
	compact, err := tok.SignedString(i.es256)
	if err != nil {
		return nil, fmt.Errorf("identity: sign delegation: %w", err)
	}
	return &Delegated{
		KID:        kid,
		Delegation: compact,
		Expiry:     time.Unix(exp.Unix(), 0),
		key:        key,
		issuer:     i.ClientID,
	}, nil
}

// SessionClaims are the per-request inputs to a Session JWT. AccessToken is
// optional; the rest are required.
type SessionClaims struct {
	Aud, Sub, Sid, Method, URL, AccessToken string
}

// SignSession signs a poppy-session+jwt (PAP §5.2) for one request with the
// delegated key. htu is URL without query or fragment, ath is set only when
// an access token is sent, and every call gets a fresh jti with iat = now.
func (d *Delegated) SignSession(c SessionClaims, now time.Time) (string, error) {
	switch {
	case c.Aud == "":
		return "", errors.New("identity: session JWT needs an aud")
	case c.Sub == "":
		return "", errors.New("identity: session JWT needs a sub")
	case c.Sid == "":
		return "", errors.New("identity: session JWT needs a sid")
	case c.Method == "":
		return "", errors.New("identity: session JWT needs a method")
	case c.URL == "":
		return "", errors.New("identity: session JWT needs a URL")
	}
	if !now.Before(d.Expiry) {
		return "", errors.New("identity: delegation has expired")
	}
	u, err := url.Parse(c.URL)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return "", fmt.Errorf("identity: session URL %q is not absolute", c.URL)
	}
	u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = "", false, "", ""

	claims := jwt.MapClaims{
		"iss": d.issuer,
		"aud": c.Aud,
		"sid": c.Sid,
		"sub": c.Sub,
		"htm": c.Method,
		"htu": u.String(),
		"iat": now.Unix(),
		"jti": randomID(),
	}
	if c.AccessToken != "" {
		sum := sha256.Sum256([]byte(c.AccessToken))
		claims["ath"] = b64.EncodeToString(sum[:])
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["typ"] = "poppy-session+jwt"
	tok.Header["kid"] = d.KID
	tok.Header["poppy_dlg"] = d.Delegation
	compact, err := tok.SignedString(d.key)
	if err != nil {
		return "", fmt.Errorf("identity: sign session JWT: %w", err)
	}
	return compact, nil
}
