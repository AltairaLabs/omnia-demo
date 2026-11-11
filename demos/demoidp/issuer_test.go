// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

package demoidp

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// A token verifies against the served JWKS the way Omnia's mint verifies a
// trusted issuer's: RS256, the key found by kid, the issuer exact, a subject.
func TestTokenVerifiesAgainstJWKS(t *testing.T) {
	iss, err := New("http://portal.example.svc:8080")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := iss.Sign("4471-889", "ellen@example.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	iss.ServeJWKS(rec, httptest.NewRequest("GET", JWKSPath, nil))
	var set struct {
		Keys []struct{ Kid, Kty, Alg, N, E string } `json:"keys"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &set); err != nil || len(set.Keys) != 1 {
		t.Fatalf("jwks %s: %v", rec.Body.String(), err)
	}
	jwk := set.Keys[0]
	n, _ := base64.RawURLEncoding.DecodeString(jwk.N)
	e, _ := base64.RawURLEncoding.DecodeString(jwk.E)
	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}

	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(raw, claims, func(tok *jwt.Token) (any, error) {
		if tok.Method.Alg() != "RS256" || tok.Header["kid"] != jwk.Kid {
			t.Fatalf("alg %s kid %v", tok.Method.Alg(), tok.Header["kid"])
		}
		return pub, nil
	}, jwt.WithIssuer("http://portal.example.svc:8080"))
	if err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != "4471-889" || claims["email"] != "ellen@example.com" {
		t.Fatalf("claims %v", claims)
	}
}

func TestSignRefusesNoSubject(t *testing.T) {
	iss, _ := New("http://x")
	if _, err := iss.Sign("", "", time.Minute); err == nil {
		t.Fatal("signed a token for no one")
	}
	if _, err := New(""); err == nil {
		t.Fatal("issuer with no name")
	}
}
