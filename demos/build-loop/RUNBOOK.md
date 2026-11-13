# Build, test and ship an agent (demo 12)

A Halvern Partners engineer builds a supplier contract triage agent for
Larkmere Mutual's in-house legal team, and takes it from first draft to a
deployed, versioned agent, all in the dashboard: draft, try, catch a flaw,
test it, fix it, push, deploy.

Everything is live: the builder, the dev agent, the runs on the real Provider
and the deploy. Nothing is replayed. Contracts and companies are fictional.

## What it shows, and what it does not

**Shows:**
- the loop people already run by hand (change the prompt, chat with it, see
  if it's better), with a test that fails before the fix and passes after;
- a dev agent per engineer, serving their uncommitted edits;
- runs and deploys built only from pushed commits, so what was tested is what
  ships;
- a deploy that produces an immutable, versioned agent, and a second version
  going out gradually.

**Does not show:**
- a pull request on GitHub: the project is hosted in Omnia;
- production rollout judged on live traffic: that's demo 9;
- a policy rule: the flaw is the agent's judgement, which is what prompts and
  tests are for.

## Before the take

1. Sign in to `https://omnia-demo.azure.altairalabs.ai` and switch to the
   **Halvern Partners** workspace.
2. Reset: delete the project `contract-triage` (Projects) and the agent
   `contract-triage` (Agents) if a previous take left them.
3. Open the three contracts in `contracts/` ready to paste.

## Dry run 2026-11-13 (what to expect)

- **New project, Generate with AI** is the builder. It asks three questions
  (what the agent does, which scenarios, tone), writes a brief, and **Build**
  writes and validates the kit in about 30 seconds. It bound the deploy to
  `azure-foundry-gpt-4o` but gave the kit a **mock** provider for runs: a run
  tests the prompt only after **Import > Provider...** of a Foundry Provider
  (or ask the builder to run on the real provider).
- **Every new agent needs a federated credential before it can call
  Foundry.** The dev agent (named after the project and you) and a deployed
  `contract-triage` each get a new service account, and AKS workload identity
  refuses it ("an internal error occurred", `AADSTS700213` in the pod log)
  until it is federated. Right after **Try it** shows the agent, and again
  right after **Deploy**, run:
  ```bash
  make -C ../omnia-azure workspace-identity WS=halvern-partners
  ```
  then wait about 30 seconds and start a **New session**. A stopgap: the
  target federates only the agents that exist when it runs, and Azure has no
  wildcard subject. The fix is upstream, Omnia #4052 (one cloud identity per
  workspace for Provider calls, separate from each agent's own).
- On the builder's draft, Brightpost's indemnity **was** flagged (gpt-4o).
  Use Fernside and Quillon next; whichever is missed is the flaw. If none,
  use the seed.
- **Try it** has **Save as scenario**: it turns the conversation into a
  scenario, which is beat 3 without pasting a file.

## The beats

### 1. Draft (builder)

Projects, **New project**, hosted, name `contract-triage`. In the editor,
**Builder** tab, say exactly:

> Triage supplier contracts for Larkmere Mutual's in-house legal team: a
> lawyer pastes a contract, and the agent flags the clauses a lawyer should
> look at, and says why. Include two or three test scenarios.

Select **Build**. The builder writes the prompt, scenarios and
`config.arena.yaml`, and commits.

**Fallback (the builder is slow or unusable):** create the project and add
the files in `seed/` instead (prompt and one scenario), then Import >
Provider... the workspace's Foundry Provider.

### 2. Try it

**Try it** tab. Fix any deploy-plan problem with its one-click button (for
example **Add a deploy block**). When it's **Ready**, paste
`contracts/brightpost-print.md`, then `fernside-claims.md`, then
`quillon-software.md`, starting a **New session** for each.

Each contract has one trap:
- Brightpost: clause 6, Larkmere indemnifies the supplier **without limit**;
- Fernside: clause 3, auto-renews for **36 months** unless notice is given
  **9 months** ahead;
- Quillon: clause 5, the supplier can **change the terms and fees at any
  time** by posting them on its website.

The first trap the draft misses is the flaw. The seed's prompt misses the
Brightpost indemnity.

### 3. Test: the new scenario fails

Add a scenario for the miss. For the indemnity, `stage/uncapped-indemnity.scenario.yaml`
is ready: New > Scenario in `scenarios/`, paste it, add it to
`config.arena.yaml` under `scenarios:`, save.

**Source control**: commit ("test: flag an uncapped indemnity") and push.
**Run this project**, **Evaluation**. On the run page, the new scenario fails
on "The agent says the indemnity has no cap"; the others pass.

### 4. Fix: everything passes

Edit the prompt by hand. Add a checklist item that's general, never naming
the contract:

```
- Liability and indemnity: flag any indemnity Larkmere gives, and any
  liability, that has no cap, or a cap out of proportion to the fees.
```

Save, **New session** in **Try it**, paste Brightpost again: it flags clause
6. Commit ("fix: check liability and indemnity for a cap"), push, run again:
every scenario passes.

### 5. Review

**Source control**: the history shows the builder's commits (with its
`Co-authored-by: Omnia builder` trailer), the test and the fix, each as you.
Open the fix commit's diff.

### 6. Deploy

**Deploy…**, **New deployment**, agent name `contract-triage`, WebSocket.
Watch **Snapshot**, **Compile**, **Publish**, **Agent ready**, then
`contract-triage is running <version>`. **Open the agent** and paste
Brightpost: it flags the indemnity.

### 7. Ship a change (cut first if short of time)

Repeat 3 and 4 for the next miss (Fernside's renewal, or Quillon's
variation clause). Then **Deploy…**, **Update a deployment**,
`contract-triage`, **Gradual**. **Deployments** lists both versions. Demo 9
is what happens next: the canary judged on live conversations.

## Gotchas

- **Runs use pushed commits only.** An unpushed fix runs the old prompt. The
  run dialog offers to sync first.
- **Try it compiles prompts when a session opens.** After saving the prompt,
  click **New session**.
- **The dev agent goes after 30 minutes unused.** Click **Start again**.
- **Results vary run to run** (a real model). If a run surprises you, say so
  and run again; don't edit the assertion to match.
- **The fix must not name the contract.** A prompt that mentions Brightpost
  passes the test and proves nothing.
