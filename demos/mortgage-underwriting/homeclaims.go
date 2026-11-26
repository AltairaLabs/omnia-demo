package main

// Larkmere Mutual's home claims, reported and added to by the signed-in member.
//
// STAND-IN: the home claims system. It fakes the insurer's claims handling for
// a member who reports a claim herself, through her Personal Agent. The member
// is the verified email of the exchanged token (obo.go) and nothing else: a
// claim belongs to the member who opened it, and no other member can read or
// add to it. A real claims system would link claims to a policyholder record
// and validate cover, excess and the incident; this one records what it is
// told. Downstream: the claims-triage routes (claims.go) do not see these
// claims, and the £500 delegated-authority limit is not enforced here, as
// claimsdata.go says; milestone 3 adds that as policy. State is in memory, so
// a restart loses every claim.
//
// Money is in pence, never a float.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	homeClaimReported = "reported"
	firstHomeClaimNum = 41001
	maxHomeClaimBody  = 64 << 10
)

type homeClaimItem struct {
	Description string `json:"description"`
	ValuePence  int    `json:"value_pence"`
	Cause       string `json:"cause,omitempty"`
}

type homeClaim struct {
	ClaimRef     string          `json:"claim_ref"`
	PolicyRef    string          `json:"policy_ref"`
	MemberName   string          `json:"member_name"`
	Peril        string          `json:"peril"`
	Description  string          `json:"description"`
	OccurredOn   string          `json:"occurred_on"`
	ReportedOn   string          `json:"reported_on"`
	Status       string          `json:"status"`
	Items        []homeClaimItem `json:"items"`
	ClaimedPence int             `json:"claimed_pence"`
	NextSteps    string          `json:"next_steps"`

	owner string // the verified email; never on the wire
}

// homeClaims holds the claims. Unlike the claims-triage maps it takes a mutex,
// because these routes are written concurrently.
type homeClaims struct {
	now func() time.Time

	mu     sync.Mutex
	next   int
	claims map[string]*homeClaim // by claim_ref
}

func newHomeClaims(now func() time.Time) *homeClaims {
	return &homeClaims{now: now, next: firstHomeClaimNum, claims: map[string]*homeClaim{}}
}

// register adds the routes, each guarded by v with the claims scope.
func (h *homeClaims) register(mux *http.ServeMux, v *oboVerifier) {
	mux.HandleFunc("/api/claims/open", v.guard("claim_open", "claims", h.handleOpen))
	mux.HandleFunc("/api/claims/add-item", v.guard("claim_add_item", "claims", h.handleAddItem))
	mux.HandleFunc("/api/claims/status", v.guard("claim_status", "claims", h.handleStatus))
}

// snapshot copies c so it can be encoded after the lock is released.
func (c *homeClaim) snapshot() homeClaim {
	cp := *c
	cp.Items = append([]homeClaimItem{}, c.Items...)
	return cp
}

func (h *homeClaims) open(owner, peril, description, occurredOn string) (homeClaim, bool) {
	pol, ok := homePolicies[owner]
	if !ok {
		return homeClaim{}, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	c := &homeClaim{
		ClaimRef:    "LM-" + strconv.Itoa(h.next),
		PolicyRef:   pol.PolicyRef,
		MemberName:  pol.MemberName,
		Peril:       peril,
		Description: description,
		OccurredOn:  occurredOn,
		ReportedOn:  h.now().Format("2006-01-02"),
		Status:      homeClaimReported,
		Items:       []homeClaimItem{},
		NextSteps:   "Add each damaged or lost item with its value. A claims handler will review the claim once the list is complete.",
		owner:       owner,
	}
	h.next++
	h.claims[c.ClaimRef] = c
	return c.snapshot(), true
}

type addItemResult int

const (
	itemAdded addItemResult = iota
	itemNotFound
	itemBad
	itemConflict
)

func (h *homeClaims) addItem(owner, ref string, item homeClaimItem) (homeClaim, addItemResult) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c, ok := h.claims[ref]
	if !ok || c.owner != owner {
		return homeClaim{}, itemNotFound
	}
	if item.ValuePence <= 0 || strings.TrimSpace(item.Description) == "" {
		return homeClaim{}, itemBad
	}
	if c.Status != homeClaimReported {
		return homeClaim{}, itemConflict
	}
	c.Items = append(c.Items, item)
	c.ClaimedPence += item.ValuePence
	return c.snapshot(), itemAdded
}

func (h *homeClaims) get(owner, ref string) (homeClaim, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c, ok := h.claims[ref]
	if !ok || c.owner != owner {
		return homeClaim{}, false
	}
	return c.snapshot(), true
}

// list answers the member's own claims, oldest first.
func (h *homeClaims) list(owner string) []homeClaim {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []homeClaim{}
	for _, c := range h.claims {
		if c.owner == owner {
			out = append(out, c.snapshot())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ClaimRef < out[j].ClaimRef })
	return out
}

// setStatus moves a claim on; the routes here never do, so it is for tests.
func (h *homeClaims) setStatus(ref, status string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c, ok := h.claims[ref]; ok {
		c.Status = status
	}
}

func decodeAccountBody(w http.ResponseWriter, r *http.Request, into any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxHomeClaimBody))
	if err := dec.Decode(into); err != nil && !errors.Is(err, io.EOF) {
		writeAccountError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func writeAccountJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (h *homeClaims) handleOpen(member string, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Peril       string `json:"peril"`
		Description string `json:"description"`
		OccurredOn  string `json:"occurred_on"`
	}
	if !decodeAccountBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Peril) == "" {
		writeAccountError(w, http.StatusBadRequest, "peril is required")
		return
	}
	c, ok := h.open(member, req.Peril, req.Description, req.OccurredOn)
	if !ok {
		writeAccountError(w, http.StatusNotFound, "no home policy for this member")
		return
	}
	writeAccountJSON(w, http.StatusCreated, c)
}

func (h *homeClaims) handleAddItem(member string, w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClaimRef    string `json:"claim_ref"`
		Description string `json:"description"`
		ValuePence  int    `json:"value_pence"`
		Cause       string `json:"cause"`
	}
	if !decodeAccountBody(w, r, &req) {
		return
	}
	c, res := h.addItem(member, req.ClaimRef, homeClaimItem{Description: req.Description, ValuePence: req.ValuePence, Cause: req.Cause})
	switch res {
	case itemNotFound:
		writeAccountError(w, http.StatusNotFound, "claim not found")
	case itemBad:
		writeAccountError(w, http.StatusBadRequest, "an item needs a description and a value_pence above zero")
	case itemConflict:
		writeAccountError(w, http.StatusConflict, "claim is no longer open for items")
	default:
		writeAccountJSON(w, http.StatusOK, c)
	}
}

func (h *homeClaims) handleStatus(member string, w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClaimRef string `json:"claim_ref"`
	}
	if !decodeAccountBody(w, r, &req) {
		return
	}
	if req.ClaimRef == "" {
		writeAccountJSON(w, http.StatusOK, map[string]any{"claims": h.list(member)})
		return
	}
	c, ok := h.get(member, req.ClaimRef)
	if !ok {
		writeAccountError(w, http.StatusNotFound, "claim not found")
		return
	}
	writeAccountJSON(w, http.StatusOK, c)
}
