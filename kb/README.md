# Acme Apparel Knowledge Base

**Status**: Draft v0.1
**Part of**: SH4 in `demo-build-plan.md`
**Consumed by**: The support agent's `search_kb` tool (T4 stub service) and, at H2 time, bundled into the `omnia-demo` Helm chart

Eight knowledge base articles for the fictional Acme Apparel storefront. The support agent's `search_kb` tool performs keyword or BM25 matching over these articles and returns excerpts. The stub KB service (hero-demo-proposal §6.4 T4) serves them as a single Go binary over HTTP.

---

## Articles

| File | Topic | Agent uses it when... |
|---|---|---|
| `shipping-policy.md` | Shipping rates, timing, free shipping threshold, carrier choice | Customer asks about when an order will arrive, what shipping costs, free shipping minimum |
| `returns-and-exchanges.md` | 30-day return window, return process, exchanges, final sale items | Customer wants to return or exchange an item, asks about return eligibility |
| `sizing-guide.md` | How Acme sizes run relative to standard US sizing, measurements | Customer is unsure about sizing, asks about fit |
| `care-instructions.md` | How to care for linen, cotton, denim, wool | Customer asks about washing, drying, ironing, long-term care |
| `order-tracking.md` | How tracking works, when customers get tracking emails, what to do if tracking is stale | Customer asks where their order is, tracking link doesn't work |
| `damaged-or-incorrect-items.md` | Process for items that arrive damaged, wrong color/size, missing | Customer received a wrong or damaged item |
| `international-shipping.md` | International rates, duties, timing, eligible countries | Customer is outside the US or asking about international orders |
| `faq.md` | Consolidated short answers to common questions | Catch-all for things not covered specifically above |

Eight articles is intentional — enough to demonstrate that the agent is searching a real corpus (not just answering from a single hardcoded doc), but small enough that every scene in the hero demo triggers a meaningful match.

---

## Editorial principles

1. **Written in Acme's voice.** Acme is a modern D2C apparel brand, late-20s to 40s urban professional target market, linen-heavy, quality-focused. Warm but not saccharine. Direct but not corporate. Says "we" naturally; acknowledges customer situations without over-apologizing.

2. **Specific numbers, not vague language.** "5-7 business days" not "a few days." "$75 free shipping threshold" not "we offer free shipping on larger orders." A support agent quoting the KB should be able to give exact answers.

3. **Section headings the agent can quote.** Each article has H2/H3 sections that a search excerpt can return cleanly. "## Free shipping" is a better section than "## The thing you all love."

4. **Consistent facts across articles.** Return window is 30 days in every article it's mentioned. Free shipping threshold is $75 everywhere. Standard shipping is 5-7 business days. If any article contradicts another, the agent will quote conflicting policies and look incoherent.

5. **No legal boilerplate.** This is a demo, not a real compliance document. The articles read like something a real brand's copywriter wrote, not something scraped from a Terms of Service.

6. **Cross-references where natural.** Return policy mentions exchanges. Sizing guide mentions free exchanges. Damaged-items mentions the return process. Makes the corpus feel interconnected rather than like 8 isolated files.

---

## Consistent facts across the corpus

These are the numbers and policies that must be the same everywhere. If you change one article, check the others.

| Fact | Value |
|---|---|
| Free shipping threshold | Orders over $75 (domestic US) |
| Standard shipping | 5-7 business days |
| Express shipping | 2 business days |
| Overnight shipping | Next business day |
| Return window | 30 days from delivery |
| Return condition | Original condition, tags attached |
| Sale items | Final sale, not returnable |
| Refund processing time | 5-7 business days after we receive the return |
| Exchange processing | Free; new item ships after we receive the original |
| Warehouse location | West Coast fulfillment (ships from California) |
| International rates | Calculated at checkout, customer pays duties |
| International delivery | 10-14 business days |
| Damaged item policy | Full refund or replacement, shipping on us |
| Support hours | 9am-6pm Pacific, Monday-Friday |
| Support email | support@acme-apparel.example.com |

---

## How the agent actually uses this

1. Customer asks something the agent doesn't have directly in its system prompt
2. Agent calls `search_kb(query="...")` with a natural-language query
3. Stub KB service does keyword/BM25 matching over the markdown files, returns top N excerpts with section headers
4. Agent synthesizes a reply quoting the relevant facts, optionally asking a follow-up
5. Agent should NOT fabricate facts not in the KB — that's what the `no-hallucinated-urls` eval tests for

For the demo, the KB service can be very simple: read all .md files at startup, do substring or BM25 matching on queries, return matching chunks. No embeddings needed at this scale (~8 articles, ~50-100 chunks). If embedding search becomes relevant later (e.g., for a real merchant with hundreds of KB docs), the service is easy to swap.

---

## Not in scope for these drafts

- **Legal content**: terms of service, privacy policy, accessibility statement. Real brands need these; our demo agent doesn't.
- **Product-specific care cards**: per-SKU care instructions (dry clean only vs machine wash etc.) belong in product data, not KB. Our agent answers care questions at the fabric-type level.
- **Affiliate / partner content**: wholesale, retailer locations, press kit. Out of scope for a consumer support agent.
- **Marketing content**: blog posts, lookbooks, style guides. Not what `search_kb` is for.
