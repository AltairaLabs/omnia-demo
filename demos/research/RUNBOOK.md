# Research on a budget (demo 10)

Halvern Partners' knowledge team runs a research function once per client
matter, asking what a regulatory change means for it. The workspace has a
daily budget with `block`. Partway through the matters the budget is spent,
and the rest are refused rather than spent; cost analytics shows what used
it.

The function runs in a workspace of its own, `halvern-research`, because a
budget covers every agent in its workspace: reaching it here blocks nothing
else.

## Spends no real money

The function runs on a mock Provider (`research-mock`), which answers with a
fixed summary and reports a cost at PromptKit's built-in mock rate, $0.01 per
1,000 tokens: about $0.0008 a run. The daily cap is half a cent
(`profiles/azure/halvern-research.yaml`), so seven or so runs reach it.
Spending real money to show that spending stops would be burning it.

A Provider's `pricing` is not applied to the mock on 0.9.0-beta.23, which is
why the cap is set to the mock's own rate rather than the mock's price raised.

## The beats

1. Show the budget (dashboard, Costs, Halvern Partners Research): $0.005 a
   day, `block`.
2. Run the function from the dashboard (Functions, research), one matter at
   a time. Each run is served until the budget is spent; after that it is
   refused with `budget_exceeded`.
3. Show cost analytics: the runs that used the budget, and the workspace's
   `WithinBudget` condition false.

Verified live 2026-11-12, 20 calls 30 seconds apart from the dashboard as the
operator: 10 served, then 10 refused with `budget_exceeded`; the workspace's
`WithinBudget` condition went false (`DailyBudgetExceeded`, $0.0084 spent).

## Gotchas

- **Refusals start up to about 5½ minutes after the cap is reached.** The
  operator reads spend from session-api about every five minutes, and the
  facade refuses from what it last heard. Runs fired all at once are all
  served before the block lands, so pace them: 20 runs 30 seconds apart.
- **Reset for the next take:** the budget is daily, so the next day's batch
  starts afresh. To run again the same day, raise `dailyBudget` and
  `make demos TENANTS=halvern-research`.
- **The summary is the same for every matter**, from the mock's script
  (`research-mock-script`). The demo is about the spend, not the summaries.
