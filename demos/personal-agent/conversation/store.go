package conversation

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/identity"
)

// Conversation is one PAP conversation (§8) the Personal Agent holds with a
// Company. Company is the Company's domain, Name its name as discovery
// found it when the conversation started (so a closed conversation is still
// named after a restart, with nothing rediscovered), Endpoint the poppy
// endpoint it was started on, and ParentID is set on a Direct Conversation
// (§8.10).
type Conversation struct {
	ID, Company, Name, Endpoint, ParentID string
	Closed                                bool
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
	// UserID returns the User ID for a Company's identifier (§3.2, §5.1),
	// creating a random one on first use. A Company that later names an
	// issuer has a new identifier, and so a new User ID.
	UserID(identifier string) (string, error)
	// Delegation returns the delegated key for a User ID, delegating a new
	// one when there is none or the current one expires within an hour.
	Delegation(userID string) (*identity.Delegated, error)
	SaveConversation(Conversation) error
	Conversations() ([]Conversation, error)
	AppendEvent(convID string, e Event) error
	Events(convID string) ([]Event, error)
	Cursor(convID string) (string, error)
	SetCursor(convID, c string) error
	// State is the conversation's last known state (§8.7): working from
	// stream notices and reads, handoff and closed from state events and
	// reads. A conversation with none yet is not working, handoff none,
	// not closed. AppendEvent applies a state event's fields.
	State(convID string) (working bool, handoff string, closed bool, err error)
	// SetWorking records a working notice (§8.6).
	SetWorking(convID string, working bool) error
	// SetState records the state fields of a read once has_more is false
	// (§8.5), or of a conversation known to be closed.
	SetState(convID string, working bool, handoff string, closed bool) error
	// Stopped is why the conversation's Reader gave up for good, such as a
	// 401 asking for sign-in, or "" while it hasn't. SetStopped records it.
	Stopped(convID string) (string, error)
	SetStopped(convID, reason string) error
}

// stateUpdate is what a state event, or an event that carries working,
// changes in a conversation's state. Nil fields are unchanged.
type stateUpdate struct {
	Working *bool   `json:"working"`
	Handoff *string `json:"handoff"`
	Closed  *bool   `json:"closed"`
}

func eventState(e Event) stateUpdate {
	var u stateUpdate
	if e.Type == "state" {
		_ = json.Unmarshal(e.Raw, &u)
	}
	if e.Working != nil {
		u.Working = e.Working
	}
	return u
}

// convState is a conversation's state as the stores keep it.
type convState struct {
	working bool
	handoff string
	closed  bool
}

func (cs *convState) apply(u stateUpdate) {
	if u.Working != nil {
		cs.working = *u.Working
	}
	if u.Handoff != nil {
		cs.handoff = *u.Handoff
	}
	if u.Closed != nil {
		cs.closed = *u.Closed
	}
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
	states  map[string]convState
	seen    map[string]map[string]bool // event IDs stored, by conversation
	stopped map[string]string
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
		states:      map[string]convState{},
		seen:        map[string]map[string]bool{},
		stopped:     map[string]string{},
	}
}

func (s *MemoryStore) UserID(identifier string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.users[identifier]; ok {
		return id, nil
	}
	id := newUserID()
	s.users[identifier] = id
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

// AppendEvent stores e, or does nothing when the conversation already holds
// an event with its ID.
func (s *MemoryStore) AppendEvent(convID string, e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.ID != "" {
		if s.seen[convID][e.ID] {
			return nil
		}
		if s.seen[convID] == nil {
			s.seen[convID] = map[string]bool{}
		}
		s.seen[convID][e.ID] = true
	}
	s.events[convID] = append(s.events[convID], e)
	s.applyLocked(convID, eventState(e))
	return nil
}

func (s *MemoryStore) applyLocked(convID string, u stateUpdate) {
	cs, ok := s.states[convID]
	if !ok {
		cs = convState{handoff: "none"}
	}
	cs.apply(u)
	s.states[convID] = cs
}

func (s *MemoryStore) State(convID string) (bool, string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cs, ok := s.states[convID]
	if !ok {
		return false, "none", false, nil
	}
	return cs.working, cs.handoff, cs.closed, nil
}

func (s *MemoryStore) SetWorking(convID string, working bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyLocked(convID, stateUpdate{Working: &working})
	return nil
}

func (s *MemoryStore) SetState(convID string, working bool, handoff string, closed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applyLocked(convID, stateUpdate{Working: &working, Handoff: &handoff, Closed: &closed})
	return nil
}

func (s *MemoryStore) Stopped(convID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped[convID], nil
}

func (s *MemoryStore) SetStopped(convID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped[convID] = reason
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
//	pa:user:<identifier>  the User ID for a Company's identifier
//	pa:convs              hash of conversation ID to its JSON
//	pa:convs:order        list of conversation IDs, oldest first
//	pa:events:<conv>      list of the conversation's events, as JSON
//	pa:seen:<conv>        set of the IDs of the events in pa:events:<conv>
//	pa:cursor:<conv>      the conversation's read cursor
//	pa:state:<conv>       hash of working, handoff and closed (§8.7), and
//	                      stopped, why its Reader gave up
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

func (s *RedisStore) UserID(identifier string) (string, error) {
	ctx := context.Background()
	key := "pa:user:" + identifier
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

// appendScript stores one event unless its ID is already in the
// conversation's seen set, and applies its state fields, as one atomic step.
// A MULTI can't do this: its commands are queued before SADD's answer is
// known, so it could not skip the RPUSH.
//
//	KEYS: pa:seen:<conv>, pa:events:<conv>, pa:state:<conv>
//	ARGV: event ID ("" for none), event JSON, then state field/value pairs
var appendScript = redis.NewScript(`
if ARGV[1] ~= "" and redis.call("SADD", KEYS[1], ARGV[1]) == 0 then
  return 0
end
redis.call("RPUSH", KEYS[2], ARGV[2])
if #ARGV > 2 then
  redis.call("HSET", KEYS[3], unpack(ARGV, 3))
end
return 1
`)

// AppendEvent stores e, or does nothing when the conversation already holds
// an event with its ID.
func (s *RedisStore) AppendEvent(convID string, e Event) error {
	b, err := json.Marshal(storedEvent(e))
	if err != nil {
		return err
	}
	args := []any{e.ID, b}
	for k, v := range stateFields(eventState(e)) {
		args = append(args, k, v)
	}
	keys := []string{"pa:seen:" + convID, "pa:events:" + convID, "pa:state:" + convID}
	if err := appendScript.Run(context.Background(), s.rdb, keys, args...).Err(); err != nil {
		return fmt.Errorf("conversation: %w", err)
	}
	return nil
}

func (s *RedisStore) Stopped(convID string) (string, error) {
	r, err := s.rdb.HGet(context.Background(), "pa:state:"+convID, "stopped").Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("conversation: %w", err)
	}
	return r, nil
}

func (s *RedisStore) SetStopped(convID, reason string) error {
	if err := s.rdb.HSet(context.Background(), "pa:state:"+convID, "stopped", reason).Err(); err != nil {
		return fmt.Errorf("conversation: %w", err)
	}
	return nil
}

// stateFields is a stateUpdate as pa:state hash fields.
func stateFields(u stateUpdate) map[string]any {
	f := map[string]any{}
	if u.Working != nil {
		f["working"] = strconv.FormatBool(*u.Working)
	}
	if u.Handoff != nil {
		f["handoff"] = *u.Handoff
	}
	if u.Closed != nil {
		f["closed"] = strconv.FormatBool(*u.Closed)
	}
	return f
}

func (s *RedisStore) setState(convID string, u stateUpdate) error {
	if err := s.rdb.HSet(context.Background(), "pa:state:"+convID, stateFields(u)).Err(); err != nil {
		return fmt.Errorf("conversation: %w", err)
	}
	return nil
}

func (s *RedisStore) State(convID string) (bool, string, bool, error) {
	f, err := s.rdb.HGetAll(context.Background(), "pa:state:"+convID).Result()
	if err != nil {
		return false, "", false, fmt.Errorf("conversation: %w", err)
	}
	handoff := f["handoff"]
	if handoff == "" {
		handoff = "none"
	}
	return f["working"] == "true", handoff, f["closed"] == "true", nil
}

func (s *RedisStore) SetWorking(convID string, working bool) error {
	return s.setState(convID, stateUpdate{Working: &working})
}

func (s *RedisStore) SetState(convID string, working bool, handoff string, closed bool) error {
	return s.setState(convID, stateUpdate{Working: &working, Handoff: &handoff, Closed: &closed})
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
