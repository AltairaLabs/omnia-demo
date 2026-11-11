# Working in this repository

This repo runs demo tenants on Omnia. Most of it stands in for systems a real
customer would bring: a member portal, an identity provider, claims and
account systems, callers. Each stand-in fakes something, and what it fakes has
consequences further down the platform. Read [docs/stand-ins.md](docs/stand-ins.md)
before changing anything that touches identity, memory, sessions or policy.

These rules apply to people and coding agents alike.

## When Omnia does something unexpected

Omnia's refusals and skipped features are usually deliberate. A gate that
looks like a bug is often the platform doing its job against a demo shortcut.

1. **Find the intent first.** Read the Omnia documentation for the subsystem
   (the explanation page, not only the reference), and the Helm values comment
   for the setting involved. Write down, in one sentence, what the design
   intends to happen. Do this before you propose a fix.
2. **Classify the cause, with evidence.** It is one of:
   - **our configuration**: a value omnia-azure (or another install) does not
     set, such as a trusted issuer;
   - **a stand-in's shortcut**: something in this repo fakes a component and
     the platform is reacting to the fake (see docs/stand-ins.md);
   - **an Omnia bug**: the code contradicts its own documented intent. Only
     this one is reported upstream, and the report quotes the contradiction.
3. **Read the mechanism before tuning around it.** If a result is wrong, find
   out what the component was actually given before changing what you give
   it. Three revisions of the canary's judge criteria failed because
   `llm_judge` is shown only the current assistant message. It never saw the
   caller's turns or the tool results, so no wording could work; the right
   check was `llm_judge_session`.
4. **Fix it where it lives.** A configuration gap is fixed in the install. A
   stand-in shortcut is fixed by making the stand-in behave like the real thing
   it replaces, not by adjusting prompts, agents or charts around it.

## Never loosen a security gate to make a demo work

Authentication, subject proofs, tenancy, consent and policy checks are the
product. If a fix would weaken one, even only in the demo, stop and ask a
maintainer. A demo that works because a control was bypassed demonstrates the
opposite of what it claims.

Two examples of fixes that look reasonable and are wrong:

- **Keying memory on an unverified id.** Member memory is keyed on a verified
  user. Making it trust the plain user id the agent pod sends would let any
  pod read any member's memories.
- **Removing a prompt line.** The fix is never to delete the line that makes
  the agent use a feature the platform withheld. Find out why it was withheld.

## Stand-ins must say what they fake

Any component here that replaces a real system documents, in
docs/stand-ins.md and in a comment where it is built:

- what it fakes;
- what the real system would provide instead;
- what breaks or changes downstream because of the fake.

A shortcut nobody wrote down reads as working architecture to the next person.

## Other rules

- Keep this repo public-safe (see CONTRIBUTING.md): synthetic data only, no
  private Omnia code or endpoints.
- The fictional companies are Larkmere Mutual, Wrenmoor Energy and Halvern
  Partners. Do not add others.
- Run the checks in `.github/workflows/ci.yml` before opening a pull request.
