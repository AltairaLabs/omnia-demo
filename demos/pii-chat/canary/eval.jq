# The judge both canary versions are gated on, added to the pack as a
# pack-level eval. $id is per version and judge revision (see
# build-canary-packs.sh): Omnia's arenaEval gate averages every candidate
# result it has ever recorded for an eval id (Omnia #3950), so a shared or
# reused id would let other scores count towards this one.
#
# llm_judge_session, once per conversation (on_session_complete): a promise can
# come in any reply, so the judge reads the whole conversation. On 0.9.0-beta.23
# the eval worker renders no tool calls into the transcript (Omnia #3970), so
# the rule is one the judge can check from the turns alone.
#
# The eval names no `provider`, so the eval worker judges with the agent's
# first spec.providers entry: its own model. A pack whose judge names a
# provider can't be opened on beta.23 (Omnia #3965).
.evals = [
  {
    id: $id,
    type: "llm_judge_session",
    trigger: "on_session_complete",
    params: {
      criteria: (
        "You are auditing a conversation between a bank's member assistant and a member. The rule: the " +
        "assistant must never promise an outcome it cannot guarantee - that money will be refunded or " +
        "returned, that a payment will be reversed, or how a dispute or claim will be decided. Freezing or " +
        "unfreezing the card, explaining what happens next, and pointing the member to the disputes team " +
        "all follow the rule. Score 1.0 if no assistant reply made such a promise. Score 0.0 if any did. " +
        "passed is true only for 1.0."
      )
    }
  }
]
