# Demo 12: build, test and ship an agent (Halvern Partners)

## Intent

The demo set shows agents running properly in production (demos 1 to 11).
None shows how an agent gets there. This demo shows the whole loop, end to
end, inside Omnia: draft an agent, try it, catch a flaw, pin the flaw down
with a test, fix it, see the test pass, review the change, and deploy a
versioned agent. The audience is the one the demo set is for: people who
have built an agent with LangChain on a laptop and test it by chatting with
it. The point is that Omnia gives them the loop they already do by hand,
with tests and a versioned deploy in it, in one place.

The user's requirement: "it doesn't really matter what the agent is, as long
as we show the whole process". Every step of the loop appears on screen;
none is narrated over.

## The story

Halvern Partners is the build side of the demo universe: a legaltech
systems integrator that builds agents for law firms and for the in-house
legal teams at Larkmere Mutual and Wrenmoor Energy (its workspace profile,
`profiles/azure/halvern-partners.yaml`). A Halvern engineer builds a
**supplier contract triage** agent for Larkmere's in-house legal team. A
lawyer pastes a supplier contract, or a clause from one, and the agent says
which clauses need a lawyer's attention and why.

*Builder's call:* the agent and its flaw. Anything legal and small works.

The engineer is the presenter, signed in to the dashboard with their own
account (Omnia Admins). Their commits are authored as them. No Halvern
persona is added to an identity provider. *My call: a persona would mean
identity work in omnia-azure for a name on a commit.*

## The beats

All in the dashboard, in the Halvern Partners workspace, signed in through
the usual SSO.

1. **Draft.** The engineer creates a hosted project and asks the builder for
   the agent: its prompt, a few test scenarios and a deploy block bound to
   the workspace's Provider. The builder shows its brief; **Build** writes
   the files and commits them.
2. **Try.** In **Try it** the engineer pastes a sample contract. The dev agent
   flags the termination and payment clauses, and misses that the indemnity
   is uncapped (the flaw).
3. **Test.** The engineer adds a scenario with that contract and an assertion
   that the uncapped indemnity is flagged, then runs the project on the real
   Provider. The new scenario fails; the others pass. The results panel shows
   the failing assertion and the agent's reply.
4. **Fix.** The engineer changes the prompt (by hand, so the audience sees the
   change, not a model's), saves, re-checks in **Try it**, and runs again.
   Every scenario passes. The fix is general ("check liability and indemnity
   for a cap") and doesn't mention that contract.
5. **Review.** **Source control** shows the prompt diff and the new scenario as
   commits authored by the engineer (the builder's carry its trailer). The
   engineer pushes to `main`; runs and deploys build only from pushed
   commits.
6. **Deploy.** **Deploy…**, **New deployment**, agent `contract-triage`,
   WebSocket. The dialog walks **Snapshot**, **Compile**, **Publish** and
   **Agent ready**, and reads `contract-triage is running <version>`. Open
   the agent and paste the same contract: it flags the indemnity.
7. **Ship a change.** A second, small change (for example, a governing-law
   check) goes through try, run and push again, then **Update a
   deployment**, **Gradual**. The deployments list shows both versions, and
   the rollout has started. It isn't driven to completion: progressing it
   needs live traffic and a judge, which is demo 9. *My call.*

## When the live take doesn't go to plan

*My calls, all of them:*

- **The builder's brief is a fixed line in the runbook**, the kind of first
  description an engineer would really give ("triage supplier contracts for
  Larkmere's legal team: flag clauses a lawyer should look at, and say
  why"). It doesn't mention liability, the way a real first brief often
  doesn't. Changing the line to provoke a particular flaw is the fake loop.
- **The flaw is whatever the draft misses, from a fixed set.** The sample
  contracts carry three different problems: an uncapped indemnity, an
  auto-renewal with a long notice period, and a supplier's right to vary
  terms on its own. In **Try it** the presenter pastes them in order. The
  first one the draft misses becomes the test. If it catches all three, the
  presenter says so and switches to the seed project, whose prompt misses
  the indemnity.
- **Assertions are content checks** (for example, the reply names the
  indemnity and says it is uncapped), not an LLM judge: cheaper, and the
  failure reads plainly on screen. Each scenario runs three trials if
  Arena supports it; otherwise once. The presenter may re-run, and says so.
  The demo counts as verified when the fixed prompt passes three runs in a
  row and the unfixed one fails all three.
- **Over time:** beat 7 goes first. Then beat 1 starts from the seed project,
  with the builder shown on a second, small request instead.

## Goals

- Every step of the loop is real: real builder, real dev agent, real runs on
  a real Provider, real deploy. Nothing is pre-baked or replayed.
- The flaw is caught by a test before anything is deployed, and the same
  test shows the fix.
- One presenter can run it in about fifteen minutes, from a reset, with a
  runbook.

## Non-goals

- A GitHub repository or pull request. The project is hosted (the user's
  decision); review is the Source control diff and commits. External
  repositories also wait on Omnia #4010.
- Handing the agent over into Larkmere's workspace. It deploys in Halvern's.
- Production rollout with a live judge: that's demo 9.
- A deterministic or recorded version.

## Anti-goals

- **A fake loop.** Results that were run earlier and shown as if live, or a
  scripted "builder" that only replays files.
- **A fix that teaches to the test.** A prompt that names the sample contract
  or its supplier passes the scenario and proves nothing.
- **Rules in the prompt.** The flaw is about the quality of the agent's
  judgement, which belongs in the prompt and is what tests are for. It must
  not be an authorisation rule (who may do what), which belongs in a
  ToolPolicy (the repo rule). The agent needs no tools for this, so it has
  none.

## Constraints

- Only Larkmere Mutual, Wrenmoor Energy and Halvern Partners. Contracts,
  suppliers and people are fictional, with the usual disclaimer, and no
  company-registration lines.
- The cluster is `aks-omnia-demo`; every command names it.
- Provider: the workspace's existing Foundry Provider
  (`azure-foundry-gpt-5-4-mini`, the builder's own). Runs cost real tokens.
  Keep the scenarios few and the contracts short.
- Work stays local on `feat/demo-plan` until the whole demo plan is verified
  (no small public PRs, no tag).
- When Omnia does something unexpected, apply the intent rule: state the
  documented intent, classify it as config, stand-in or Omnia bug, and
  ticket only a contradiction, with the user's approval per issue.

## First checks (before building anything for it)

Each is a step of the loop that hasn't been run on this deployment yet.
- `make builder` installs the builder in `halvern-partners`, and it can
  draft and commit a project.
- **Try it** reaches **Ready** in `halvern-partners`.
- A real-Provider run of a pushed commit completes and shows per-scenario
  assertion results.
- **Deploy…** of a hosted project completes all four steps, and **Update a
  deployment**, **Gradual** works for a second version.

If one fails, it's diagnosed under the intent rule before the demo works
around anything.

## What gets built

- A sample contract set: two or three short supplier contracts for Larkmere
  (for example, a claims-handling outsourcer and a print supplier), one of
  them with an uncapped indemnity. Kept in the repo, pasted during the demo.
- A seed project in the repo (prompt, scenarios, `config.arena.yaml`), the
  state after beat 1, for when the builder is slow or drafts something
  unusable on the day. It is a fallback; the demo starts from the builder.
- A reset (`make build-loop-reset`): deletes the project, its dev agent,
  the `contract-triage` deployment and any rollout in progress.
- A RUNBOOK (`demos/build-loop/RUNBOOK.md`) in the house style: what it
  shows and doesn't, beats, verified notes, gotchas.
- A catalogue row, marked Live only once verified.
- Demo 12 added to the demo-set spec, in Act 4 ahead of demo 9 (build it,
  then roll it out safely).

## Left to the builder

- The exact prompt, scenarios, assertions and contract text.
- The second change in beat 7.
- Which of the three flaws the seed project's prompt misses, if not the
  indemnity.
