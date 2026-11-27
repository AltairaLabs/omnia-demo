package signin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	testAccess  = "access-token-ellen-larkmere"
	testRefresh = "refresh-token-ellen-larkmere"
)

func testTokens() Tokens {
	return Tokens{
		Access:  testAccess,
		Refresh: testRefresh,
		Scopes:  []string{"policies:read"},
		Expiry:  time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC),
	}
}

func testKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestMemoryTokenStoreRoundTrip(t *testing.T) {
	s := NewMemoryTokenStore()
	got, err := s.Get("https://larkmere.example", "u1")
	if err != nil || got != nil {
		t.Fatalf("Get before Put = %v, %v; want nil, nil", got, err)
	}
	want := testTokens()
	if err := s.Put("https://larkmere.example", "u1", want); err != nil {
		t.Fatal(err)
	}
	got, err = s.Get("https://larkmere.example", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !reflect.DeepEqual(*got, want) {
		t.Fatal("Get returned different tokens from those Put") // never print them
	}
	if other, err := s.Get("https://wrenmoor.example", "u1"); err != nil || other != nil {
		t.Fatalf("Get for another identifier = %v, %v; want nil, nil", other, err)
	}
	if other, err := s.Get("https://larkmere.example", "u2"); err != nil || other != nil {
		t.Fatalf("Get for another User ID = %v, %v; want nil, nil", other, err)
	}
	// The store keeps its own copy: changing the caller's scopes changes
	// nothing stored.
	got.Scopes[0] = "changed"
	again, _ := s.Get("https://larkmere.example", "u1")
	if again.Scopes[0] != "policies:read" {
		t.Fatalf("stored scopes changed through a returned value: %v", again.Scopes)
	}
	if err := s.Delete("https://larkmere.example", "u1"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get("https://larkmere.example", "u1"); err != nil || got != nil {
		t.Fatalf("Get after Delete = %v, %v; want nil, nil", got, err)
	}
}

func TestLoadKeyRejectsWrongLength(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	short := write("short", base64.StdEncoding.EncodeToString(make([]byte, 16))+"\n")
	if _, err := LoadKey(short); err == nil {
		t.Fatal("LoadKey accepted a 16-byte key")
	}
	if _, err := LoadKey(write("junk", "not base64!\n")); err == nil {
		t.Fatal("LoadKey accepted a line that isn't base64")
	}
	good := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if _, err := LoadKey(write("two", good+"\n"+good+"\n")); err == nil {
		t.Fatal("LoadKey accepted a file of two lines")
	}
	if _, err := LoadKey(write("crlf", good+"\r\n")); err != nil {
		t.Fatalf("LoadKey refused a CRLF line ending: %v", err)
	}
	want := testKey(t)
	k, err := LoadKey(write("good", base64.StdEncoding.EncodeToString(want)+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(k, want) {
		t.Fatal("LoadKey returned a different key")
	}
}

func TestSealedTokenWontOpenUnderAnotherKey(t *testing.T) {
	key := testKey(t)
	plain := []byte(`{"access":"` + testAccess + `"}`)
	blob, err := seal(key, tokenAD("https://larkmere.example", "u1"), plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, []byte(testAccess)) {
		t.Fatal("sealed blob contains the access token")
	}
	got, err := open(key, tokenAD("https://larkmere.example", "u1"), blob)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("open under its own key = %q, %v", got, err)
	}
	for name, ad := range map[string][]byte{
		"another identifier": tokenAD("https://wrenmoor.example", "u1"),
		"another User ID":    tokenAD("https://larkmere.example", "u2"),
	} {
		if got, err := open(key, ad, blob); err == nil {
			t.Errorf("blob moved to %s opened: %q", name, got)
		}
	}
	if _, err := open(testKey(t), tokenAD("https://larkmere.example", "u1"), blob); err == nil {
		t.Error("blob opened under another encryption key")
	}
	if _, err := open(key, tokenAD("https://larkmere.example", "u1"), blob[:5]); err == nil {
		t.Error("a truncated blob opened")
	}
	flipped := bytes.Clone(blob)
	flipped[len(flipped)-1] ^= 1
	if _, err := open(key, tokenAD("https://larkmere.example", "u1"), flipped); err == nil {
		t.Error("a blob with a flipped bit opened")
	}
}

func TestTokensNeverPrinted(t *testing.T) {
	tk := testTokens()
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("tokens", "t", tk, "p", &tk)
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("tokens", "t", tk)
	for name, out := range map[string]string{
		"fmt":  fmt.Sprintf("%v %+v %#v %s", tk, &tk, tk, tk),
		"slog": buf.String(),
	} {
		if strings.Contains(out, testAccess) || strings.Contains(out, testRefresh) {
			t.Errorf("%s output contains a token", name)
		}
		if !strings.Contains(out, "[redacted]") || !strings.Contains(out, "policies:read") {
			t.Errorf("%s output lacks the redaction or the scopes: %s", name, out)
		}
	}
	if s := (Tokens{}).String(); !strings.Contains(s, "Access:absent") {
		t.Errorf("empty Tokens = %s, want Access:absent", s)
	}
	// JSON, which sealing uses, still carries the tokens.
	b, err := json.Marshal(tk)
	if err != nil || !strings.Contains(string(b), testAccess) || !strings.Contains(string(b), testRefresh) {
		t.Fatal("JSON encoding lost the tokens")
	}
}

func TestTokenStoresRejectAmbiguousPairs(t *testing.T) {
	stores := map[string]TokenStore{"memory": NewMemoryTokenStore()}
	if rs, err := NewRedisTokenStore("redis://localhost:1/0", testKey(t)); err == nil {
		stores["redis"] = rs // the check comes before any Redis call
	}
	for name, s := range stores {
		for _, p := range [][2]string{{"https://larkmere.example\nx", "u1"}, {"https://larkmere.example", "u|1"}} {
			if err := s.Put(p[0], p[1], testTokens()); err == nil {
				t.Errorf("%s: Put(%q, %q) accepted", name, p[0], p[1])
			}
			if _, err := s.Get(p[0], p[1]); err == nil {
				t.Errorf("%s: Get(%q, %q) accepted", name, p[0], p[1])
			}
		}
	}
}

func TestPutNormalisesExpiry(t *testing.T) {
	s := NewMemoryTokenStore()
	tk := testTokens()
	tk.Expiry = time.Now().In(time.FixedZone("X", 3600)) // has a monotonic reading
	if err := s.Put("https://larkmere.example", "u1", tk); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get("https://larkmere.example", "u1")
	if got.Expiry.Location() != time.UTC || got.Expiry != tk.Expiry.UTC().Round(0) {
		t.Fatalf("stored expiry %v is not UTC without a monotonic reading", got.Expiry)
	}
}

// TestRedisTokenStoreSealed runs only with PA_TEST_REDIS_URL set, such as
// redis://localhost:6379/15. It uses database 14 of that server, not the
// URL's, and flushes it, so it can't flush the conversation tests' database
// when both run at once.
func TestRedisTokenStoreSealed(t *testing.T) {
	u := os.Getenv("PA_TEST_REDIS_URL")
	if u == "" {
		t.Skip("PA_TEST_REDIS_URL not set")
	}
	pu, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	pu.Path = "/14"
	st, err := NewRedisTokenStore(pu.String(), testKey(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := st.rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	if got, err := st.Get("https://larkmere.example", "u1"); err != nil || got != nil {
		t.Fatalf("Get before Put = %v, %v; want nil, nil", got, err)
	}
	want := testTokens()
	if err := st.Put("https://larkmere.example", "u1", want); err != nil {
		t.Fatal(err)
	}
	raw, err := st.rdb.Get(ctx, "pa:tokens:https://larkmere.example|u1").Result()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, testAccess) || strings.Contains(raw, testRefresh) {
		t.Fatal("the raw Redis value contains a token")
	}
	got, err := st.Get("https://larkmere.example", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !reflect.DeepEqual(*got, want) {
		t.Fatal("Get returned different tokens from those Put") // never print them
	}
	// The blob copied under another Company's key is an error, not "none".
	if err := st.rdb.Set(ctx, "pa:tokens:https://wrenmoor.example|u1", raw, 0).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get("https://wrenmoor.example", "u1"); err == nil {
		t.Fatal("Get of a moved blob succeeded, want an error")
	}
	if err := st.Delete("https://larkmere.example", "u1"); err != nil {
		t.Fatal(err)
	}
	if got, err := st.Get("https://larkmere.example", "u1"); err != nil || got != nil {
		t.Fatalf("Get after Delete = %v, %v; want nil, nil", got, err)
	}
}
