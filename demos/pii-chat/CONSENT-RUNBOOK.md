# Ellen controls her data (demo 8)

**Held** on Omnia #3984 and #3991. Everything below is built; two beats can't
be shown honestly until those are fixed.

On the Larkmere portal's **Your data** page, Ellen sees what the assistant
remembers about her, withdraws consent for a category (its memories go at
once), and requests erasure, which completes with a record of what was
deleted. Every call goes to Omnia's end-user plane (management-api
`/api/v1/me/workspaces/larkmere-mutual`) with a token for Ellen alone; Omnia
picks the subject from that token and nothing else.

## The beats

1. Ellen grants **Health and accessibility**, then tells the assistant she has
   a hearing disability and would rather be texted.
2. **Your data** lists the memory. *Blocked by #3984:* the end-user plane
   lists only memories saved with no agent, and every memory the assistant
   saves is agent-scoped, so the list is empty.
3. She withdraws the category. The memory is deleted at once (memory-api
   writes `memory_consent_prune`), and the next conversation's
   `memory_context` no longer has it. *Blocked by #3991:* the runtime caches a
   member's profile for 30 seconds and the revocation doesn't clear it, so the
   deleted memory can still reach the model for up to 30 seconds.
4. She requests erasure. The request goes `pending`, then `completed`, with
   the number of conversations deleted.

Verified 2026-11-12 without the blocked parts: consent grant and revoke
through the page's routes; `memory_consent_prune` on revocation; erasure
completed in under a second, deleting Ellen's two sessions.

## When the fixes land

Upgrade the install to the Omnia release with #3984 and #3991, run the four
beats live, and update this page and the catalogue row.

## How it's wired

- **The portal page:** `demos/demo-sites/site/larkmere-mutual/ui_kits/portal/MyData.jsx`.
- **The proxy:** `myDataProxy` in `demos/demo-sites/server.go`. It forwards
  only the page's routes, signs a token for Ellen with the audience
  `larkmere-members`, and passes nothing of the browser's.
- **The Workspace:** `spec.endUserAuth.oidc` names the portal's issuer and
  that audience (`piiChat.portal.myData`).
- **The stand-in:** `docs/stand-ins.md`, "Your data".
