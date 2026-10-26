package main

import (
	"fmt"
	"math/rand"
	"strings"
)

// The six product consent categories. Each maps to exactly one semantic topic
// below, so the Memory Galaxy's COLOUR (consent category) and POSITION
// (embedding meaning) line up — every coloured blob is one topic.
const (
	catIdentity    = "memory:identity"
	catHealth      = "memory:health"
	catLocation    = "memory:location"
	catPreferences = "memory:preferences"
	catHistory     = "memory:history"
	catContext     = "memory:context"
)

// Categories lists all six for tests and coverage assertions.
var Categories = []string{
	catIdentity, catHealth, catLocation,
	catPreferences, catHistory, catContext,
}

// Memory types. memory-api validates user- and agent-tier writes against the
// MemoryPolicy schema, or without one against its built-in schema (general,
// fact, preference, observation, temporal_summary for these tiers), so the
// seed uses only built-in types. Institutional facts are not constrained by
// the built-in schema and keep a domain type.
const (
	typeFact        = "fact"
	typePreference  = "preference"
	typeObservation = "observation"
	typePolicy      = "policy"
)

// UserTierTypes are the built-in schema's types allowed in the user tier.
var UserTierTypes = []string{"general", typeFact, typePreference, typeObservation, "temporal_summary"}

// AgentTierTypes are the built-in schema's types allowed in the agent tier.
var AgentTierTypes = UserTierTypes

type Doc struct{ Title, URL, Site, Text string }
type UserMemory struct {
	RawUserID, Category, Type, Content string
	Confidence                         float64
}
type AgentMemory struct {
	AgentID, Type, Content string
	Confidence             float64
}

// Observation is a system-generated observation on one topic, attributed to
// the synthetic operations user. A run of them per topic gives the Galaxy
// dense, related clusters.
type Observation struct{ Topic, Category, Content string }

// Generated is the full deterministic seed payload.
type Generated struct {
	Docs          []Doc
	AgentMemories []AgentMemory
	UserMemories  []UserMemory
	Observations  []Observation
}

// topic is a semantic cluster: one domain, one consent category, with several
// interchangeable phrasings of the SAME idea. Generating from these pools
// yields content that is semantically tight (one topic) but lexically varied
// (different words for the same concept) — so the dense embedding projection
// forms a clear, separated cluster that the lexical tf-idf projection blurs
// together. That contrast is the whole point of the galaxy demo.
type topic struct {
	key      string
	category string
	memType  string   // the user-tier memory type this topic's facts take
	leads    []string // subject / opening clause
	cores    []string // the core fact — many wordings of one meaning
	tails    []string // closing detail clause
}

// Repeated subject clauses, pulled out so the same literal isn't duplicated
// across topics (and to keep the pools readable).
const (
	subjCustomer = "The customer"
	subjUser     = "The user"
)

// topics: six clearly-distinct domains with non-overlapping vocabularies, so
// their embeddings separate cleanly. Within each, the cores deliberately use
// different words for the same idea (signed in / authenticated / logged on) so
// term-frequency can't cluster them the way meaning can.
var topics = []topic{
	{
		key: "identity", category: catIdentity, memType: typeFact,
		leads: []string{
			subjCustomer, subjUser, "This account holder",
			"During onboarding the member", "On their last visit the client",
		},
		cores: []string{
			"signed in through single sign-on", "authenticated with their corporate SSO",
			"reset a forgotten password", "enabled two-factor authentication",
			"verified their identity with a one-time code", "linked a new device to the account",
			"recovered access after a lockout", "rotated their API credentials",
		},
		tails: []string{
			"without further issues.", "after a brief verification step.",
			"and confirmed it by email.", "following the standard identity check.",
		},
	},
	{
		key: "health", category: catHealth, memType: typePreference,
		leads: []string{subjCustomer, "This member", "The account holder", "The primary contact"},
		cores: []string{
			"requested a high-contrast, screen-reader friendly layout",
			"mentioned a repetitive-strain injury and asked for keyboard-only navigation",
			"noted a temporary medical leave and paused notifications",
			"asked for larger text due to a visual impairment",
			"requested captions on all training videos",
			"shared that they rely on assistive technology daily",
			"flagged a dietary restriction for the on-site workshop",
		},
		tails: []string{
			"so the team adjusted the experience.", "and we logged the accommodation.",
			"to keep the product accessible.", "and support noted it on the profile.",
		},
	},
	{
		key: "location", category: catLocation, memType: typeFact,
		leads: []string{subjCustomer, subjUser, "This tenant", "The operations lead"},
		cores: []string{
			"moved their billing address to Berlin", "requested data residency in the EU region",
			"updated the shipping destination to a new warehouse", "switched their timezone to US Pacific",
			"asked for invoices to go to the Singapore office", "relocated the team to a London hub",
			"set the default region to ap-southeast", "added a second site in Toronto",
		},
		tails: []string{
			"effective next cycle.", "and we updated the records.",
			"for compliance reasons.", "to match their operations.",
		},
	},
	{
		key: "preferences", category: catPreferences, memType: typePreference,
		leads: []string{subjCustomer, subjUser, "The member", "The workspace admin"},
		cores: []string{
			"prefers email over phone for support", "opted into the monthly product newsletter",
			"turned on dark mode as the default theme", "asked to receive only critical alerts",
			"chose annual billing over monthly", "wants release notes in plain language",
			"set Slack as the preferred notification channel", "opted out of marketing emails",
		},
		tails: []string{
			"going forward.", "and we saved the preference.",
			"across all their workspaces.", "to cut down on noise.",
		},
	},
	{
		key: "history", category: catHistory, memType: typeFact,
		leads: []string{
			"Last quarter the customer", "Earlier this year the account",
			"Previously the user", "In a prior ticket the client",
		},
		cores: []string{
			"disputed an invoice that was later credited", "experienced a brief outage during a deployment",
			"upgraded from the starter to the growth plan", "filed a chargeback that was resolved amicably",
			"hit the API rate limit during a migration", "renewed the contract for another term",
			"reported a billing discrepancy that was corrected", "churned briefly and then reactivated",
		},
		tails: []string{
			"and the matter is now closed.", "with no recurrence since.",
			"per the audit trail.", "as recorded in their history.",
		},
	},
	{
		key: "context", category: catContext, memType: typeObservation,
		leads: []string{
			"Right now the customer", "Currently the team",
			"At the moment the user", "In the active ticket the client",
		},
		cores: []string{
			"is troubleshooting a failing webhook integration", "is migrating data from a legacy system",
			"is evaluating the new analytics dashboard", "is waiting on a fix for an export bug",
			"is rolling out SSO to a second department", "is testing the beta of the workflow engine",
			"is onboarding three new seats this week", "is tuning alert thresholds for production",
		},
		tails: []string{
			"and support is actively involved.", "with a follow-up scheduled.",
			"as the current priority.", "and we're tracking progress.",
		},
	},
}

func pick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

// sentence renders one natural, lexically-varied sentence for a topic.
func (t topic) sentence(r *rand.Rand) string {
	return fmt.Sprintf("%s %s %s", pick(r, t.leads), pick(r, t.cores), pick(r, t.tails))
}

// Generate produces the deterministic seed for s using r.
func Generate(s Scenario, r *rand.Rand) Generated {
	return Generated{
		Docs:          genDocs(s, r),
		AgentMemories: genAgentMemories(s, r),
		UserMemories:  genUserMemories(s, r),
		Observations:  genObservations(s, r),
	}
}

// genDocs builds institutional knowledge-base docs, one topic each, from many
// DISTINCT varied sentences (never a repeated paragraph) so the chunks stay
// topic-coherent and diverse instead of collapsing into one blob.
func genDocs(s Scenario, r *rand.Rand) []Doc {
	docs := make([]Doc, s.InstitutionalDocs)
	for i := range docs {
		t := topics[i%len(topics)]
		var b strings.Builder
		// ~22 distinct sentences ≈ 250+ words → one or two topical chunks.
		for range 22 {
			b.WriteString(t.sentence(r))
			b.WriteByte(' ')
		}
		docs[i] = Doc{
			Title: fmt.Sprintf("KB-%04d: %s playbook", i, t.key),
			URL:   fmt.Sprintf("kb://hawkridge/%s/%04d", t.key, i),
			Site:  "hawkridge-support",
			Text:  strings.TrimSpace(b.String()),
		}
	}
	return docs
}

// genAgentMemories are learned resolution patterns, kept topical so they land
// in their topic's cluster, with varied wording.
func genAgentMemories(s Scenario, r *rand.Rand) []AgentMemory {
	out := make([]AgentMemory, s.AgentMemories)
	for i := range out {
		t := topics[r.Intn(len(topics))]
		out[i] = AgentMemory{
			AgentID:    s.AgentUID,
			Type:       typeFact,
			Content:    fmt.Sprintf("For %s cases: %s", t.key, t.sentence(r)),
			Confidence: 0.5 + r.Float64()*0.5,
		}
	}
	return out
}

// genUserMemories spreads each user's memories across the six topics, so the
// galaxy shows six coloured semantic clusters. Category == topic category.
func genUserMemories(s Scenario, r *rand.Rand) []UserMemory {
	out := make([]UserMemory, 0, s.Users*s.MemoriesPerUser)
	for u := range s.Users {
		raw := fmt.Sprintf("customer-%03d", u)
		for j := range s.MemoriesPerUser {
			// Cover all categories evenly, then jitter so it isn't lockstep.
			t := topics[(u+j)%len(topics)]
			if r.Intn(4) == 0 {
				t = topics[r.Intn(len(topics))]
			}
			out = append(out, UserMemory{
				RawUserID:  raw,
				Category:   t.category,
				Type:       t.memType,
				Content:    t.sentence(r),
				Confidence: 0.6 + r.Float64()*0.4,
			})
		}
	}
	return out
}

// genObservations writes several topical observations for each of a few
// operational themes, each theme pinned to one topic so its observations
// cluster together.
func genObservations(s Scenario, r *rand.Rand) []Observation {
	out := make([]Observation, 0, s.ObservationThemes*s.ObsPerTheme)
	for e := range s.ObservationThemes {
		t := topics[e%len(topics)]
		for range s.ObsPerTheme {
			out = append(out, Observation{
				Topic:    t.key,
				Category: t.category,
				Content:  t.sentence(r),
			})
		}
	}
	return out
}
