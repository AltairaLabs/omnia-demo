package conversation

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/identity"
)

// Conversation is one PAP conversation (§8) the Personal Agent holds with a
// Company. Company is the Company's domain, Endpoint the poppy endpoint it
// was started on, and ParentID is set on a Direct Conversation (§8.10).
type Conversation struct {
	ID, Company, Endpoint, ParentID string
	Closed                          bool
}

// Event is one conversation event (§8.5). Role, AuthorType and Text come
// from a message event's message. Working is set when the event's JSON has
// a working field, as a stream's working notice (§8.6) does. Raw is the
// event's full JSON, as the Company sent it.
type Event struct {
	ID, Type, Role, AuthorType, Text string
	Working                          *bool
	Raw                              json.RawMessage
}

// Store keeps what the Personal Agent must remember about its Users'
// conversations: one User ID per Company (§5.1), a delegated key per User
// ID (§5.2), the conversations, their events and read cursors (§8.5).
type Store interface {
	// UserID returns the User ID for a Company's domain, creating a random
	// one on first use.
	UserID(company string) (string, error)
	// Delegation returns the delegated key for a User ID, delegating a new
	// one when there is none or the current one expires within an hour.
	Delegation(userID string) (*identity.Delegated, error)
	SaveConversation(Conversation) error
	Conversations() ([]Conversation, error)
	AppendEvent(convID string, e Event) error
	Events(convID string) ([]Event, error)
	Cursor(convID string) (string, error)
	SetCursor(convID, c string) error
}

// renewBefore is how long before a delegation's expiry it is replaced.
const renewBefore = time.Hour

// newUserID returns 128 random bits, base64url without padding. It is never
// derived from a name, email or anything else about the User (§5.1).
func newUserID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error.
	return base64.RawURLEncoding.EncodeToString(b)
}

// delegations caches one delegated key per User ID, in memory only: the
// delegated private key is not exported by identity, so it can't be stored,
// and a restart simply delegates new keys (§5.2 lets keys change freely).
type delegations struct {
	id  *identity.Identity
	now func() time.Time

	mu sync.Mutex
	m  map[string]*identity.Delegated
}

func newDelegations(id *identity.Identity, now func() time.Time) *delegations {
	if now == nil {
		now = time.Now
	}
	return &delegations{id: id, now: now, m: map[string]*identity.Delegated{}}
}

func (d *delegations) get(userID string) (*identity.Delegated, error) {
	if d.id == nil {
		return nil, errors.New("conversation: store has no identity to delegate from")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.now()
	if cur := d.m[userID]; cur != nil && now.Before(cur.Expiry.Add(-renewBefore)) {
		return cur, nil
	}
	nd, err := d.id.Delegate(now)
	if err != nil {
		return nil, err
	}
	d.m[userID] = nd
	return nd, nil
}

// MemoryStore is a Store held in memory, for tests.
type MemoryStore struct {
	*delegations

	mu      sync.Mutex
	users   map[string]string
	order   []string
	convs   map[string]Conversation
	events  map[string][]Event
	cursors map[string]string
}

// NewMemoryStore returns an empty in-memory Store that delegates keys from
// id, reading the time from now.
func NewMemoryStore(id *identity.Identity, now func() time.Time) *MemoryStore {
	return &MemoryStore{
		delegations: newDelegations(id, now),
		users:       map[string]string{},
		convs:       map[string]Conversation{},
		events:      map[string][]Event{},
		cursors:     map[string]string{},
	}
}

func (s *MemoryStore) UserID(company string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.users[company]; ok {
		return id, nil
	}
	id := newUserID()
	s.users[company] = id
	return id, nil
}

func (s *MemoryStore) Delegation(userID string) (*identity.Delegated, error) {
	return s.get(userID)
}

func (s *MemoryStore) SaveConversation(c Conversation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.convs[c.ID]; !ok {
		s.order = append(s.order, c.ID)
	}
	s.convs[c.ID] = c
	return nil
}

func (s *MemoryStore) Conversations() ([]Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Conversation, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.convs[id])
	}
	return out, nil
}

func (s *MemoryStore) AppendEvent(convID string, e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[convID] = append(s.events[convID], e)
	return nil
}

func (s *MemoryStore) Events(convID string) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.events[convID]...), nil
}

func (s *MemoryStore) Cursor(convID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cursors[convID], nil
}

func (s *MemoryStore) SetCursor(convID, c string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursors[convID] = c
	return nil
}

// RedisStore is a Store in Redis, with every key under "pa:":
//
//	pa:user:<company>     the User ID for a Company's domain
//	pa:convs              hash of conversation ID to its JSON
//	pa:convs:order        list of conversation IDs, oldest first
//	pa:events:<conv>      list of the conversation's events, as JSON
//	pa:cursor:<conv>      the conversation's read cursor
//
// Delegated keys are held in memory, not in Redis (see delegations).
type RedisStore struct {
	*delegations
	rdb *redis.Client
}

// NewRedisStore connects to the Redis at url (redis:// or rediss://) and
// delegates keys from id, reading the time from now.
func NewRedisStore(url string, id *identity.Identity, now func() time.Time) (*RedisStore, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("conversation: redis url: %w", err)
	}
	return &RedisStore{delegations: newDelegations(id, now), rdb: redis.NewClient(opt)}, nil
}

// storedEvent is how an Event is kept in Redis. Raw is kept as JSON, not
// base64, so the stored list stays readable.
type storedEvent struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Role       string          `json:"role,omitempty"`
	AuthorType string          `json:"author_type,omitempty"`
	Text       string          `json:"text,omitempty"`
	Working    *bool           `json:"working,omitempty"`
	Raw        json.RawMessage `json:"raw"`
}

func (s *RedisStore) UserID(company string) (string, error) {
	ctx := context.Background()
	key := "pa:user:" + company
	// SetNX makes the first writer win when two requests race.
	if _, err := s.rdb.SetNX(ctx, key, newUserID(), 0).Result(); err != nil {
		return "", fmt.Errorf("conversation: %w", err)
	}
	id, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("conversation: %w", err)
	}
	return id, nil
}

func (s *RedisStore) Delegation(userID string) (*identity.Delegated, error) {
	return s.get(userID)
}

func (s *RedisStore) SaveConversation(c Conversation) error {
	ctx := context.Background()
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	added, err := s.rdb.HSet(ctx, "pa:convs", c.ID, b).Result()
	if err != nil {
		return fmt.Errorf("conversation: %w", err)
	}
	if added == 1 {
		if err := s.rdb.RPush(ctx, "pa:convs:order", c.ID).Err(); err != nil {
			return fmt.Errorf("conversation: %w", err)
		}
	}
	return nil
}

func (s *RedisStore) Conversations() ([]Conversation, error) {
	ctx := context.Background()
	ids, err := s.rdb.LRange(ctx, "pa:convs:order", 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("conversation: %w", err)
	}
	all, err := s.rdb.HGetAll(ctx, "pa:convs").Result()
	if err != nil {
		return nil, fmt.Errorf("conversation: %w", err)
	}
	out := make([]Conversation, 0, len(ids))
	for _, id := range ids {
		raw, ok := all[id]
		if !ok {
			continue
		}
		var c Conversation
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, fmt.Errorf("conversation: stored conversation %s: %w", id, err)
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *RedisStore) AppendEvent(convID string, e Event) error {
	b, err := json.Marshal(storedEvent(e))
	if err != nil {
		return err
	}
	if err := s.rdb.RPush(context.Background(), "pa:events:"+convID, b).Err(); err != nil {
		return fmt.Errorf("conversation: %w", err)
	}
	return nil
}

func (s *RedisStore) Events(convID string) ([]Event, error) {
	raws, err := s.rdb.LRange(context.Background(), "pa:events:"+convID, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("conversation: %w", err)
	}
	out := make([]Event, 0, len(raws))
	for _, raw := range raws {
		var se storedEvent
		if err := json.Unmarshal([]byte(raw), &se); err != nil {
			return nil, fmt.Errorf("conversation: stored event: %w", err)
		}
		out = append(out, Event(se))
	}
	return out, nil
}

func (s *RedisStore) Cursor(convID string) (string, error) {
	c, err := s.rdb.Get(context.Background(), "pa:cursor:"+convID).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("conversation: %w", err)
	}
	return c, nil
}

func (s *RedisStore) SetCursor(convID, c string) error {
	if err := s.rdb.Set(context.Background(), "pa:cursor:"+convID, c, 0).Err(); err != nil {
		return fmt.Errorf("conversation: %w", err)
	}
	return nil
}
