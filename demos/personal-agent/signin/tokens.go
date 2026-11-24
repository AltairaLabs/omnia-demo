package signin

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Tokens are what a Company's issuer granted one User at sign-in (§4.2) or
// at a refresh (§4.4). Access and Refresh are secrets: nothing in this
// package puts them in an error, and callers must not log them or show
// them in a tool answer, an update, a page or a URL.
type Tokens struct {
	Access  string    `json:"access"`
	Refresh string    `json:"refresh"`
	Scopes  []string  `json:"scopes"`
	Expiry  time.Time `json:"expiry"`
}

// TokenStore keeps Tokens per Company identifier (§3.2) and User ID (§5.1),
// so a token is only ever found under the Company and User it was granted
// for.
type TokenStore interface {
	// Get answers nil, nil when there are no tokens for the pair.
	Get(identifier, userID string) (*Tokens, error)
	Put(identifier, userID string, t Tokens) error
	Delete(identifier, userID string) error
}

func cloneTokens(t Tokens) Tokens {
	t.Scopes = slices.Clone(t.Scopes)
	return t
}

// MemoryTokenStore is a TokenStore held in memory, unsealed, for tests.
type MemoryTokenStore struct {
	mu sync.Mutex
	m  map[[2]string]Tokens
}

// NewMemoryTokenStore returns an empty MemoryTokenStore.
func NewMemoryTokenStore() *MemoryTokenStore {
	return &MemoryTokenStore{m: map[[2]string]Tokens{}}
}

func (s *MemoryTokenStore) Get(identifier, userID string) (*Tokens, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.m[[2]string{identifier, userID}]
	if !ok {
		return nil, nil
	}
	t = cloneTokens(t)
	return &t, nil
}

func (s *MemoryTokenStore) Put(identifier, userID string, t Tokens) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[[2]string{identifier, userID}] = cloneTokens(t)
	return nil
}

func (s *MemoryTokenStore) Delete(identifier, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, [2]string{identifier, userID})
	return nil
}

// RedisTokenStore is a TokenStore in Redis. Each pair's Tokens are kept at
//
//	pa:tokens:<identifier>|<userID>
//
// as nonce || AES-256-GCM(json(Tokens)), with identifier + "\n" + userID as
// the additional data. The raw value holds no token, and a value moved to
// another pair's key fails to open: Get reports that as an error, never as
// "no tokens".
type RedisTokenStore struct {
	rdb *redis.Client
	key []byte
}

// NewRedisTokenStore connects to the Redis at url (redis:// or rediss://)
// and seals with key, which must be 32 bytes (see LoadKey).
func NewRedisTokenStore(url string, key []byte) (*RedisTokenStore, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("signin: token key is %d bytes, want 32", len(key))
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("signin: redis url: %w", err)
	}
	return &RedisTokenStore{rdb: redis.NewClient(opt), key: bytes.Clone(key)}, nil
}

func tokenKey(identifier, userID string) string {
	return "pa:tokens:" + identifier + "|" + userID
}

// tokenAD is the additional data a pair's Tokens are sealed with.
func tokenAD(identifier, userID string) []byte {
	return []byte(identifier + "\n" + userID)
}

func (s *RedisTokenStore) Get(identifier, userID string) (*Tokens, error) {
	raw, err := s.rdb.Get(context.Background(), tokenKey(identifier, userID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("signin: %w", err)
	}
	plain, err := open(s.key, tokenAD(identifier, userID), raw)
	if err != nil {
		return nil, err
	}
	var t Tokens
	if err := json.Unmarshal(plain, &t); err != nil {
		// The JSON error is not wrapped: it can quote the plaintext.
		return nil, errors.New("signin: stored tokens are not valid JSON")
	}
	return &t, nil
}

func (s *RedisTokenStore) Put(identifier, userID string, t Tokens) error {
	plain, err := json.Marshal(t)
	if err != nil {
		return errors.New("signin: tokens can't be encoded")
	}
	blob, err := seal(s.key, tokenAD(identifier, userID), plain)
	if err != nil {
		return err
	}
	if err := s.rdb.Set(context.Background(), tokenKey(identifier, userID), blob, 0).Err(); err != nil {
		return fmt.Errorf("signin: %w", err)
	}
	return nil
}

func (s *RedisTokenStore) Delete(identifier, userID string) error {
	if err := s.rdb.Del(context.Background(), tokenKey(identifier, userID)).Err(); err != nil {
		return fmt.Errorf("signin: %w", err)
	}
	return nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("signin: token key is %d bytes, want 32", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("signin: %w", err)
	}
	return cipher.NewGCM(block)
}

// seal returns nonce || AES-256-GCM(plain) under key, bound to ad.
func seal(key, ad, plain []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	_, _ = rand.Read(nonce) // crypto/rand.Read never returns an error.
	return gcm.Seal(nonce, nonce, plain, ad), nil
}

// open reverses seal. It fails when blob was sealed under another key or
// other additional data, or has been altered.
func open(key, ad, blob []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	n := gcm.NonceSize()
	if len(blob) < n+gcm.Overhead() {
		return nil, errors.New("signin: sealed tokens are too short")
	}
	plain, err := gcm.Open(nil, blob[:n], blob[n:], ad)
	if err != nil {
		return nil, errors.New("signin: sealed tokens won't open for this company and user")
	}
	return plain, nil
}

// LoadKey reads the token sealing key from path: one line of standard
// base64 that decodes to exactly 32 bytes.
func LoadKey(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("signin: token key: %w", err)
	}
	line := bytes.TrimSuffix(bytes.TrimSuffix(b, []byte("\n")), []byte("\r"))
	if bytes.ContainsAny(line, "\r\n") {
		return nil, errors.New("signin: token key file has more than one line")
	}
	key, err := base64.StdEncoding.DecodeString(string(line))
	if err != nil {
		// The decode error is not wrapped: it can point into the key.
		return nil, errors.New("signin: token key is not standard base64")
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("signin: token key is %d bytes, want 32", len(key))
	}
	return key, nil
}
