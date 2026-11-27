# Applied to the compiled pack by hack/build-packs.sh, for a field the arena
# source cannot set.
#
# The pack's provider requirements (RFC 0012). packc does not emit `requires`,
# so the slot the scope guardrail names (params.provider: guard) would be
# undeclared: the pack would say nothing about needing a classifier, and the
# host would learn it only when the guardrail fails closed. Declared here, the
# host can check the binding at load, and Omnia's AgentRuntime binds it by name
# (spec.providers[].name: guard).
#
# And the judge Dreaming studies: a conversation where the member left without
# something they could act on fails it. The pack has a real gap here, left in
# on purpose: it knows nothing about using the card abroad, so "will my card
# work in Lisbon?" gets a vague answer and a failed eval, which is what a
# Dreaming night selects first (evalFailuresThenRecent). The canary's candidate
# packs replace .evals with their own gate.
#
# Session-level and with no provider, for the same reasons as the canary's
# judge (demos/pii-chat/canary/eval.jq).
.evals = [
  {
    id: "card-support-actionable-v2",
    type: "llm_judge_session",
    trigger: "on_session_complete",
    params: {
      criteria: (
        "You are reviewing a conversation between a bank's card assistant and a member. Judge only whether " +
        "the member left with something they can act on. Fail the conversation (score 0.0) if ANY assistant " +
        "reply does one of these: (a) tells the member to contact support, the team, card services or the " +
        "bank without giving a specific channel (a phone number, an app or website section, or an address); " +
        "(b) states a bank policy, timescale or fee as fact without having it from a tool result (for " +
        "example 'no need to tell us you're travelling' or 'we'll get it to you promptly'); (c) answers a " +
        "different question from the one asked. Refusing an out-of-scope request (investment, tax or legal " +
        "advice) with a pointer to a qualified adviser is fine. Score 1.0 only if no reply did any of " +
        "these. Use only 0.0 or 1.0. passed is true only for 1.0."
      )
    }
  }
]
| .requires.providers = [
  {key: "default", role: "llm", required: true},
  {key: "guard", role: "inference", required: true,
   description: "Topic classifier for the scope guardrail (Jev, TypeSafe System One)"}
]
