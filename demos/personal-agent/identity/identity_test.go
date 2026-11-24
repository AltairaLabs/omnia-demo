package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testBase = "https://assistant.azure.altairalabs.ai"

// writeTestKeys generates the two identity keys in-process and writes them
// as PKCS#8 PEMs to a temporary directory, as Load expects to find them.
func writeTestKeys(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for name, key := range map[string]any{"es256.pem": ec, "ps384.pem": rs} {
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
		if err := os.WriteFile(filepath.Join(dir, name), pemBytes, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func loadTestIdentity(t *testing.T) *Identity {
	t.Helper()
	id, err := Load(writeTestKeys(t), testBase)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return id
}

// parseUnverified splits a compact JWT into its header and claims without
// checking the signature, for assertions on the wire format.
func parseUnverified(t *testing.T, compact string) (map[string]any, map[string]any) {
	t.Helper()
	parts := strings.Split(compact, ".")
	if len(parts) != 3 {
		t.Fatalf("compact JWT has %d parts, want 3", len(parts))
	}
	decode := func(s string) map[string]any {
		raw, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			t.Fatalf("base64url: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("json: %v", err)
		}
		return m
	}
	return decode(parts[0]), decode(parts[1])
}

// verify checks the compact JWT's signature with key, allowing only alg.
func verify(t *testing.T, compact, alg string, key any) {
	t.Helper()
	_, err := jwt.Parse(compact, func(*jwt.Token) (any, error) { return key, nil },
		jwt.WithValidMethods([]string{alg}), jwt.WithoutClaimsValidation())
	if err != nil {
		t.Fatalf("signature does not verify: %v", err)
	}
}

// jwkToECDSA rebuilds a P-256 public key from a JWK's x and y members.
func jwkToECDSA(t *testing.T, jwk map[string]any) *ecdsa.PublicKey {
	t.Helper()
	x, err := base64.RawURLEncoding.DecodeString(jwk["x"].(string))
	if err != nil {
		t.Fatal(err)
	}
	y, err := base64.RawURLEncoding.DecodeString(jwk["y"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if len(x) != 32 || len(y) != 32 {
		t.Fatalf("x/y lengths %d/%d, want 32/32", len(x), len(y))
	}
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
	if err != nil {
		t.Fatal(err)
	}
	return pub
}

func TestThumbprintRFC7638Example(t *testing.T) {
	// RFC 7638 §3.1.
	n, err := base64.RawURLEncoding.DecodeString("0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM78LhWx4cbbfAAtVT86zwu1RK7aPFFxuhDR1L6tSoc_BJECPebWKRXjBZCiFV4n3oknjhMstn64tZ_2W-5JsGY4Hc5n9yBXArwl93lqt7_RN5w6Cf0h4QyQ5v-65YGjQR0_FDW2QvzqY368QQMicAtaSqzs8KJZgnYb9c7d0zgdAZHzu6qMQvRL5hajrn1n91CbOpbISD08qNLyrdkt-bFTWhAI4vMQFh6WeZu0fM4lFd2NcRwr3XPksINHaQ-G_xBniIqbw0Ls1jF44-csFCur-kEgU8awapJzKnqDKgw")
	if err != nil {
		t.Fatal(err)
	}
	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: 65537}
	got, err := Thumbprint(pub)
	if err != nil {
		t.Fatal(err)
	}
	if want := "NzbLsXh8uDCcd-6MNwXF4W_7noWXFZAfHkxZsRGC9Xs"; got != want {
		t.Fatalf("Thumbprint = %q, want %q", got, want)
	}
}

func TestMetadataShape(t *testing.T) {
	id := loadTestIdentity(t)
	md := id.Metadata()

	if got, want := md["client_id"], testBase+"/agent.json"; got != want {
		t.Errorf("client_id = %v, want %v", got, want)
	}
	if got, want := md["jwks_uri"], testBase+"/jwks.json"; got != want {
		t.Errorf("jwks_uri = %v, want %v", got, want)
	}
	if got := md["token_endpoint_auth_method"]; got != "private_key_jwt" {
		t.Errorf("token_endpoint_auth_method = %v", got)
	}
	if got := md["client_name"]; got != "Ellen's assistant" {
		t.Errorf("client_name = %v", got)
	}
	redirects, ok := md["redirect_uris"].([]string)
	if !ok || len(redirects) != 1 || redirects[0] != testBase+"/signin/callback" {
		t.Errorf("redirect_uris = %#v", md["redirect_uris"])
	}

	clientURL, err := url.Parse(md["client_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	jwksURL, err := url.Parse(md["jwks_uri"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if jwksURL.Host != clientURL.Host {
		t.Errorf("jwks_uri host %q differs from client_id host %q", jwksURL.Host, clientURL.Host)
	}

	// The document must survive JSON encoding with the same members.
	if _, err := json.Marshal(md); err != nil {
		t.Fatal(err)
	}
}

func TestJWKSPublicOnly(t *testing.T) {
	id := loadTestIdentity(t)
	var set struct {
		Keys []map[string]any `json:"keys"`
	}
	if err := json.Unmarshal(id.JWKS(), &set); err != nil {
		t.Fatal(err)
	}
	if len(set.Keys) != 2 {
		t.Fatalf("jwks has %d keys, want 2", len(set.Keys))
	}
	wantKid := map[string]string{}
	for alg, pub := range map[string]any{"ES256": id.es256.Public(), "PS384": id.ps384.Public()} {
		kid, err := Thumbprint(pub)
		if err != nil {
			t.Fatal(err)
		}
		wantKid[alg] = kid
	}
	for _, k := range set.Keys {
		alg, _ := k["alg"].(string)
		if k["kid"] != wantKid[alg] {
			t.Errorf("%s kid = %v, want %v", alg, k["kid"], wantKid[alg])
		}
		if k["use"] != "sig" {
			t.Errorf("%s use = %v", alg, k["use"])
		}
		for _, private := range []string{"d", "p", "q", "dp", "dq", "qi"} {
			if _, ok := k[private]; ok {
				t.Errorf("%s key carries private member %q", alg, private)
			}
		}
		delete(wantKid, alg)
	}
	if len(wantKid) != 0 {
		t.Errorf("jwks missing algs %v", wantKid)
	}
}

func TestDelegationClaims(t *testing.T) {
	id := loadTestIdentity(t)
	now := time.Now()
	d, err := id.Delegate(now)
	if err != nil {
		t.Fatal(err)
	}

	verify(t, d.Delegation, "ES256", id.es256.Public())
	hdr, claims := parseUnverified(t, d.Delegation)

	if hdr["typ"] != "poppy-delegation+jwt" {
		t.Errorf("typ = %v", hdr["typ"])
	}
	if hdr["alg"] != "ES256" {
		t.Errorf("alg = %v", hdr["alg"])
	}
	esKid, err := Thumbprint(id.es256.Public())
	if err != nil {
		t.Fatal(err)
	}
	if hdr["kid"] != esKid {
		t.Errorf("kid = %v, want identity ES256 key %v", hdr["kid"], esKid)
	}

	if claims["iss"] != id.ClientID {
		t.Errorf("iss = %v", claims["iss"])
	}
	iat, exp := int64(claims["iat"].(float64)), int64(claims["exp"].(float64))
	if iat != now.Unix() {
		t.Errorf("iat = %d, want %d", iat, now.Unix())
	}
	if exp-iat > int64(24*time.Hour/time.Second) || exp <= iat {
		t.Errorf("exp-iat = %ds, want (0, 24h]", exp-iat)
	}
	if !d.Expiry.Equal(time.Unix(exp, 0)) {
		t.Errorf("Expiry = %v, want %v", d.Expiry, time.Unix(exp, 0))
	}
	if jti, _ := claims["jti"].(string); jti == "" {
		t.Error("jti missing")
	}
	for _, banned := range []string{"sub", "aud", "agent_id"} {
		if _, ok := claims[banned]; ok {
			t.Errorf("delegation carries %q", banned)
		}
	}

	cnf, _ := claims["cnf"].(map[string]any)
	jwk, _ := cnf["jwk"].(map[string]any)
	if jwk == nil {
		t.Fatal("cnf.jwk missing")
	}
	if _, ok := jwk["d"]; ok {
		t.Error("cnf.jwk carries private member d")
	}
	if len(jwk) != 4 || jwk["kty"] != "EC" || jwk["crv"] != "P-256" {
		t.Errorf("cnf.jwk = %v, want exactly kty, crv, x, y", jwk)
	}
	delegated := jwkToECDSA(t, jwk)
	dkid, err := Thumbprint(delegated)
	if err != nil {
		t.Fatal(err)
	}
	if dkid != d.KID {
		t.Errorf("cnf.jwk thumbprint %v != Delegated.KID %v", dkid, d.KID)
	}
}

func testSession() SessionClaims {
	return SessionClaims{
		Aud:    "https://wrenmoor.example",
		Sub:    "user-123",
		Sid:    "sid-abc",
		Method: "POST",
		URL:    "https://wrenmoor.example/poppy/v1/messages?x=1#frag",
	}
}

func TestSessionJWT(t *testing.T) {
	id := loadTestIdentity(t)
	now := time.Now()
	d, err := id.Delegate(now)
	if err != nil {
		t.Fatal(err)
	}
	compact, err := d.SignSession(testSession(), now)
	if err != nil {
		t.Fatal(err)
	}
	hdr, claims := parseUnverified(t, compact)

	if hdr["typ"] != "poppy-session+jwt" {
		t.Errorf("typ = %v", hdr["typ"])
	}
	if hdr["alg"] != "ES256" {
		t.Errorf("alg = %v", hdr["alg"])
	}
	if hdr["poppy_dlg"] != d.Delegation {
		t.Errorf("poppy_dlg is not the delegation")
	}

	// The delegated public key comes from the delegation itself, as a
	// Company would get it, and the kid must be its thumbprint.
	_, dlgClaims := parseUnverified(t, hdr["poppy_dlg"].(string))
	delegated := jwkToECDSA(t, dlgClaims["cnf"].(map[string]any)["jwk"].(map[string]any))
	wantKid, err := Thumbprint(delegated)
	if err != nil {
		t.Fatal(err)
	}
	if hdr["kid"] != wantKid {
		t.Errorf("kid = %v, want %v", hdr["kid"], wantKid)
	}
	verify(t, compact, "ES256", delegated)

	want := map[string]any{
		"iss": id.ClientID,
		"aud": "https://wrenmoor.example",
		"sub": "user-123",
		"sid": "sid-abc",
		"htm": "POST",
		"htu": "https://wrenmoor.example/poppy/v1/messages",
	}
	for k, v := range want {
		if claims[k] != v {
			t.Errorf("%s = %v, want %v", k, claims[k], v)
		}
	}
	if jti, _ := claims["jti"].(string); jti == "" {
		t.Error("jti missing")
	}
	for _, banned := range []string{"ath", "agent_id"} {
		if _, ok := claims[banned]; ok {
			t.Errorf("session JWT carries %q", banned)
		}
	}
}

func TestSessionJWTWithToken(t *testing.T) {
	id := loadTestIdentity(t)
	now := time.Now()
	d, err := id.Delegate(now)
	if err != nil {
		t.Fatal(err)
	}
	c := testSession()
	c.AccessToken = "at-xyz"
	compact, err := d.SignSession(c, now)
	if err != nil {
		t.Fatal(err)
	}
	_, claims := parseUnverified(t, compact)
	sum := sha256.Sum256([]byte("at-xyz"))
	if want := base64.RawURLEncoding.EncodeToString(sum[:]); claims["ath"] != want {
		t.Errorf("ath = %v, want %v", claims["ath"], want)
	}
}

func TestSessionSignedPerCall(t *testing.T) {
	id := loadTestIdentity(t)
	now := time.Now()
	d, err := id.Delegate(now)
	if err != nil {
		t.Fatal(err)
	}
	later := now.Add(90 * time.Second)
	a, err := d.SignSession(testSession(), later)
	if err != nil {
		t.Fatal(err)
	}
	b, err := d.SignSession(testSession(), later)
	if err != nil {
		t.Fatal(err)
	}
	_, ca := parseUnverified(t, a)
	_, cb := parseUnverified(t, b)
	if ca["jti"] == cb["jti"] {
		t.Errorf("jti repeated across calls: %v", ca["jti"])
	}
	for _, c := range []map[string]any{ca, cb} {
		if got := int64(c["iat"].(float64)); got != later.Unix() {
			t.Errorf("iat = %d, want the now passed in %d", got, later.Unix())
		}
	}
}

func TestSignSessionRejectsMissingFields(t *testing.T) {
	id := loadTestIdentity(t)
	now := time.Now()
	d, err := id.Delegate(now)
	if err != nil {
		t.Fatal(err)
	}
	blank := map[string]func(*SessionClaims){
		"Aud":    func(c *SessionClaims) { c.Aud = "" },
		"Sub":    func(c *SessionClaims) { c.Sub = "" },
		"Sid":    func(c *SessionClaims) { c.Sid = "" },
		"Method": func(c *SessionClaims) { c.Method = "" },
		"URL":    func(c *SessionClaims) { c.URL = "" },
	}
	for name, clear := range blank {
		c := testSession()
		clear(&c)
		if _, err := d.SignSession(c, now); err == nil {
			t.Errorf("SignSession accepted an empty %s", name)
		}
	}
	if _, err := d.SignSession(testSession(), d.Expiry); err == nil {
		t.Error("SignSession accepted an expired delegation")
	}
}

func TestClientAssertionClaims(t *testing.T) {
	id := loadTestIdentity(t)
	const issuer = "https://auth.larkmere.example/tenant"
	now := time.Now()
	compact, err := id.ClientAssertion(issuer, now)
	if err != nil {
		t.Fatalf("ClientAssertion: %v", err)
	}
	verify(t, compact, "ES256", &id.es256.PublicKey)

	header, claims := parseUnverified(t, compact)
	if header["alg"] != "ES256" {
		t.Errorf("alg = %v, want ES256", header["alg"])
	}
	kid, err := Thumbprint(&id.es256.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if header["kid"] != kid {
		t.Errorf("kid = %v, want the ES256 key's thumbprint %s", header["kid"], kid)
	}
	if aud, ok := claims["aud"].(string); !ok || aud != issuer {
		t.Errorf("aud = %#v, want the string %q", claims["aud"], issuer)
	}
	if claims["iss"] != id.ClientID || claims["sub"] != id.ClientID {
		t.Errorf("iss = %v, sub = %v, want both %s", claims["iss"], claims["sub"], id.ClientID)
	}
	iat, _ := claims["iat"].(float64)
	exp, _ := claims["exp"].(float64)
	if life := exp - iat; life <= 0 || life > 300 {
		t.Errorf("exp - iat = %v, want in (0, 300]", life)
	}
	if int64(iat) != now.Unix() {
		t.Errorf("iat = %v, want %d", iat, now.Unix())
	}
	jti, _ := claims["jti"].(string)
	if raw, err := base64.RawURLEncoding.DecodeString(jti); err != nil || len(raw) < 16 {
		t.Errorf("jti %q is not 128 random bits in base64url", jti)
	}

	again, err := id.ClientAssertion(issuer, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, c2 := parseUnverified(t, again); c2["jti"] == claims["jti"] {
		t.Error("two client assertions share a jti")
	}

	if _, err := id.ClientAssertion("", now); err == nil {
		t.Error("ClientAssertion accepted an empty issuer")
	}
}
