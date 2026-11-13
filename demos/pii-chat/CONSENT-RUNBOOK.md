# Ellen controls her data (demo 8)

**Held** on Omnia #4088. #3984 and #3991 are fixed on main; the withdrawal
beat still can't be shown honestly.

On the Larkmere portal's **Your data** page, Ellen sees what the assistant
remembers about her, withdraws consent for a category (its memories go at
once), and requests erasure, which completes with a record of what was
deleted. Every call goes to Omnia's end-user plane (management-api
`/api/v1/me/workspaces/larkmere-mutual`) with a token for Ellen alone; Omnia
picks the subject from that token and nothing else.

## The beats

1. Ellen grants **Health and accessibility**, then tells the assistant she has
   a hearing disability and would rather be texted.
2. **Your data** lists the memory, with its category. **Forget** removes it,
   and the next conversation no longer knows it.
3. She withdraws the category. The memory should be deleted at once
   (memory-api writes `memory_consent_prune`). *Blocked by #4088:* the
   assistant files "texts, because of a hearing disability" as
   **Preferences**, and Omnia's classifier, which detects health, is
   overruled by that claim when the memory is stored. Withdrawing Health then
   leaves it. The page shows the wrong tag, so don't take this beat.
4. She requests erasure. The request goes `pending`, then `completed`, with
   the number of conversations deleted, and the memory list empties.

Verified 2026-11-13 on Omnia main `896d399a7eea`: the list shows the
assistant's agent-scoped memories (#3984); a memory forgotten on the page did
not reach a conversation started seconds later (#3991); erasure completed and
emptied the list. Withdrawal is blocked on #4088.

## When the fix lands

Rebuild on the Omnia commit with #4088 (`hack/omnia-dev.sh build` in
omnia-azure), take beat 3, and update this page and the catalogue row.

## How it's wired

- **The portal page:** `demos/demo-sites/site/larkmere-mutual/ui_kits/portal/MyData.jsx`.
- **The proxy:** `myDataProxy` in `demos/demo-sites/server.go`. It forwards
  only the page's routes, signs a token for Ellen with the audience
  `larkmere-members`, and passes nothing of the browser's.
- **The Workspace:** `spec.endUserAuth.oidc` names the portal's issuer and
  that audience (`piiChat.portal.myData`).
- **The stand-in:** `docs/stand-ins.md`, "Your data".
