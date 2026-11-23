package conversation

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// streamWait is sent as wait on stream requests. A Company that streams
	// ignores it (§8.6); one that answers with JSON holds the read that long.
	streamWait = "30"
	maxBackoff = 30 * time.Second
	maxLine    = 1 << 20 // longest SSE line accepted
)

// parseEvent reads the fields the Personal Agent uses from one event's JSON
// (§8.5, event.schema.json) and keeps the whole JSON in Raw.
func parseEvent(raw json.RawMessage) (Event, error) {
	var w struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Working *bool  `json:"working"`
		Message *struct {
			Role   string `json:"role"`
			Author struct {
				Type string `json:"type"`
			} `json:"author"`
			Text string `json:"text"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &w); err != nil {
		return Event{}, fmt.Errorf("conversation: event: %w", err)
	}
	e := Event{ID: w.ID, Type: w.Type, Working: w.Working, Raw: append(json.RawMessage(nil), raw...)}
	if w.Type == "message" && w.Message != nil {
		e.Role, e.AuthorType, e.Text = w.Message.Role, w.Message.Author.Type, w.Message.Text
	}
	return e, nil
}

// closes reports whether e is a state event with closed true (§8.7, §8.12).
func closes(e Event) bool {
	if e.Type != "state" {
		return false
	}
	var s struct {
		Closed bool `json:"closed"`
	}
	return json.Unmarshal(e.Raw, &s) == nil && s.Closed
}

// Reader follows one conversation in the background: it streams its events
// (§8.6), stores each new one with the cursor after it, and publishes it.
type Reader struct {
	Client *Client
	// Backoff is the wait before reconnecting. It doubles up to 30s after a
	// failed stream, and is back to Backoff after a stream that delivered
	// an event or ended cleanly.
	Backoff time.Duration

	sleep func(context.Context, time.Duration) error // for tests; nil is sleep
}

// Run streams conv's events until ctx ends or the conversation closes. It
// reconnects with Last-Event-ID after a dropped stream, and after
// cursor_expired reads from the beginning, skipping events already stored
// (§8.5). It stops after a conversation_closed error, or a state event that
// shows the conversation closed, and then marks it closed in the store. It
// also stops on any other 4xx but 429, which no reconnect can fix, and
// records the reason with Store.SetStopped.
// Working notices are published with no ID; they update the stored state
// but are not stored as events.
func (r *Reader) Run(ctx context.Context, conv Conversation, publish func(Conversation, Event)) {
	backoff := r.Backoff
	if backoff <= 0 {
		backoff = time.Second
	}
	pause := r.sleep
	if pause == nil {
		pause = sleep
	}
	wait := backoff
	for ctx.Err() == nil {
		n, closed, err := r.stream(ctx, conv, publish)
		var cerr *Error
		switch {
		case closed || IsCode(err, "conversation_closed"):
			r.markClosed(conv)
			return
		case IsCode(err, "cursor_expired"):
			// Read from the beginning at once, unless this read already
			// was: then dropping the cursor can't help, and asking again at
			// once would loop, so back off like any other failure.
			if cur, gerr := r.Client.Store.Cursor(conv.ID); gerr == nil && cur != "" &&
				r.Client.Store.SetCursor(conv.ID, "") == nil {
				continue
			}
		case errors.As(err, &cerr) && cerr.Status >= 400 && cerr.Status < 500 && cerr.Status != http.StatusTooManyRequests:
			// conversation_not_found, a 401 asking for sign-in,
			// version_unsupported and the like: reconnecting can't help.
			// Record why, so whoever reads the conversation can say so.
			_ = r.Client.Store.SetStopped(conv.ID, strings.TrimPrefix(cerr.Error(), "conversation: "))
			return
		case err == nil && n == 0:
			// A stream that ends without an event may be of a conversation
			// that has expired, which reads as closed with no state event.
			evs, closed, err := r.Client.read(ctx, conv, 0)
			if err == nil {
				for _, e := range evs {
					publish(conv, e)
				}
				if closed {
					r.markClosed(conv)
					return
				}
			}
		}
		if n > 0 || err == nil {
			// A stream that delivered events, or that simply ended (the
			// poppy facade closes every stream after five minutes), is not
			// a failure: reconnect at the base wait, not a growing one.
			wait = backoff
		}
		if pause(ctx, wait) != nil {
			return
		}
		wait = min(wait*2, maxBackoff)
	}
}

// markClosed records the conversation as closed, with the state §8.7 gives
// a closed conversation: not working, handoff none.
func (r *Reader) markClosed(conv Conversation) {
	conv.Closed = true
	_ = r.Client.Store.SaveConversation(conv)
	_ = r.Client.Store.SetState(conv.ID, false, "none", true)
}

// stream opens one stream and handles it until it ends. It returns how many
// new conversation events it delivered and whether one closed the
// conversation.
func (r *Reader) stream(ctx context.Context, conv Conversation, publish func(Conversation, Event)) (int, bool, error) {
	c := r.Client
	co, err := c.company(ctx, conv.Company)
	if err != nil {
		return 0, false, err
	}
	seen, err := c.seen(conv.ID)
	if err != nil {
		return 0, false, err
	}
	cursor, err := c.Store.Cursor(conv.ID)
	if err != nil {
		return 0, false, err
	}
	q := url.Values{"wait": {streamWait}}
	hdr := http.Header{"Accept": {"text/event-stream"}}
	if cursor != "" {
		// Last-Event-ID wins when both are sent (§8.6); cursor serves a
		// Company that answers with JSON instead of a stream.
		q.Set("cursor", cursor)
		hdr.Set("Last-Event-ID", cursor)
	}
	resp, err := c.do(ctx, co, http.MethodGet, convURL(conv, "events")+"?"+q.Encode(), nil, hdr)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()

	n := 0
	deliver := func(raw []byte) (bool, error) {
		e, err := parseEvent(raw)
		if err != nil || e.ID == "" || seen[e.ID] {
			return false, nil // malformed or already handled: skip it
		}
		seen[e.ID] = true
		if err := c.Store.AppendEvent(conv.ID, e); err != nil {
			return false, err
		}
		if err := c.Store.SetCursor(conv.ID, e.ID); err != nil {
			return false, err
		}
		n++
		publish(conv, e)
		return closes(e), nil
	}

	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		var rr readResponse
		if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&rr); err != nil {
			return n, false, fmt.Errorf("conversation: decoding events: %w", err)
		}
		for _, raw := range rr.Events {
			if closed, err := deliver(raw); err != nil || closed {
				return n, closed, err
			}
		}
		if rr.Cursor != "" {
			if err := c.Store.SetCursor(conv.ID, rr.Cursor); err != nil {
				return n, false, err
			}
		}
		if err := c.saveState(conv.ID, rr); err != nil {
			return n, false, err
		}
		return n, rr.Closed && !rr.HasMore, nil
	}

	// The event stream is parsed as the WHATWG HTML Standard's "Interpreting
	// an event stream" says (§9.2.6,
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation):
	//   - a line is split at its first colon into field and value, one
	//     leading space is removed from the value, and a line with no colon
	//     is a field with an empty value;
	//   - a line starting with a colon is a comment;
	//   - data lines are joined with "\n";
	//   - a blank line dispatches the event, which is dropped when its data
	//     is empty, and resets the event name;
	//   - an event not ended by a blank line when the stream ends is
	//     discarded.
	// Two departures: bufio.ScanLines ends lines at LF or CRLF but not at a
	// lone CR, and a leading BOM is not stripped. The id field is not
	// tracked, because each event's JSON carries its own id (§8.6).
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)
	var name string
	var data []string
	for sc.Scan() {
		line := sc.Text()
		if line != "" {
			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "event":
				name = value
			case "data":
				data = append(data, value)
			}
			continue // "id", comments and unknown fields need nothing here
		}
		// A blank line dispatches the event.
		payload := []byte(strings.Join(data, "\n"))
		ev := name
		name, data = "", nil
		if len(payload) == 0 {
			continue
		}
		switch ev {
		case "", "message": // a conversation event
			closed, err := deliver(payload)
			if err != nil || closed {
				return n, closed, err
			}
		case "working":
			var w struct {
				Working *bool `json:"working"`
			}
			if json.Unmarshal(payload, &w) == nil && w.Working != nil {
				if err := c.Store.SetWorking(conv.ID, *w.Working); err != nil {
					return n, false, err
				}
				publish(conv, Event{Type: "working", Working: w.Working, Raw: payload})
			}
		default:
			// text-delta and names this version doesn't know are skipped (§8.6).
		}
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		return n, false, fmt.Errorf("conversation: stream: %w", err)
	}
	return n, false, nil
}
