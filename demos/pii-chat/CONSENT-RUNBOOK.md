# Ellen controls her data (demo 8)

On the Larkmere portal's **Your data** page, Ellen sees what the assistant
remembers about her, withdraws consent for a category (its memories go at
once), and requests erasure, which completes with a record of what was
deleted. Every call goes to Omnia's end-user plane (management-api
`/api/v1/me/workspaces/larkmere-mutual`) with a token for Ellen alone; Omnia
picks the subject from that token and nothing else.

## The beats

1. Ellen grants **Health and accessibility**, then tells the assistant: *"Please
   text me rather than call in future. I have a hearing disability, so phone
   calls are hard for me."*
2. **Your data** lists the memory, tagged **Health and accessibility**. The
   assistant may have filed it as a preference; Omnia's classifier detects
   health and the stricter category is the one stored.
3. She withdraws the category. The memory is deleted at once (memory-api
   writes `memory_consent_prune`) and the list empties.
4. Or, instead of 3: **Forget** removes one memory, and the next conversation
   no longer knows it.
5. She requests erasure. The request goes `pending`, then `completed`, with
   the number of conversations deleted, and the memory list empties.

Verified live 2026-11-14 on Omnia main `6359a66ac480`: the memory "Prefers to
be contacted via text rather than phone calls due to a hearing disability."
was stored as `memory:health` (#4088), and withdrawing Health removed it at
once. On 2026-11-13 (`896d399a7eea`): the list shows the assistant's
agent-scoped memories (#3984); a memory forgotten on the page did not reach a
conversation started seconds later (#3991); erasure completed and emptied the
list.

## Before each take

- Erasure does not reset consent: a Health grant from an earlier take is
  still there. Check the toggles before beat 1.
- After withdrawal Health is denied, so a new health note is refused
  (`memory write suppressed … consent-not-granted`) until it is granted again.

## How it's wired

- **The portal page:** `demos/demo-sites/site/larkmere-mutual/ui_kits/portal/MyData.jsx`.
- **The proxy:** `myDataProxy` in `demos/demo-sites/server.go`. It forwards
  only the page's routes, signs a token for Ellen with the audience
  `larkmere-members`, and passes nothing of the browser's.
- **The Workspace:** `spec.endUserAuth.oidc` names the portal's issuer and
  that audience (`piiChat.portal.myData`).
- **The stand-in:** `docs/stand-ins.md`, "Your data".
