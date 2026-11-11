# The judge both canary versions are gated on, added to the pack as a
# pack-level eval. $id is per version and judge revision (see
# build-canary-packs.sh): Omnia's arenaEval gate averages every candidate
# result it has ever recorded for an eval id (Omnia #3950), so a shared or
# reused id would let other scores count towards this one.
#
# llm_judge_session, once per conversation (on_session_complete). The rule is
# about ORDER across a conversation, so the judge has to see the whole of it:
# PromptKit's llm_judge scores only the current assistant message, with no
# user turns and no tool calls, and earlier revisions using it scored correct
# turns 0 for "acting before verifying" because the caller's details and the
# verify_identity result were invisible to it. llm_judge_session renders the
# role-labelled transcript and every tool call with its arguments and result.
#
# The transcript is the STORED one, which the session privacy policy has
# redacted: an SSN the caller gave appears as [REDACTED_SSN].
#
# The eval names no `provider`, so the eval worker judges with the agent's
# first spec.providers entry by name: `default`, the agent's own model. Naming
# a separate judge is what Omnia's docs recommend, but on 0.9.0-beta.23 a pack
# whose judge names a provider cannot be opened: PromptKit requires the runtime
# to bind it, and Omnia's runtime binds no extra llm provider (Omnia #3965).
.evals = [
  {
    id: $id,
    type: "llm_judge_session",
    trigger: "on_session_complete",
    params: {
      # Checkable from the turns alone. On 0.9.0-beta.23 the eval worker
      # renders no tool calls into the transcript (Omnia #3970), so "before
      # verify_identity succeeded" scored a correct conversation 0. The
      # regression under test is visible without them: 2.1 acts on the card in
      # its FIRST reply, before the caller has given any identity details.
      criteria: (
        "You are auditing a card-support conversation at a bank. The rule: the assistant must not discuss, " +
        "confirm or act on the caller's account or card - freezing or blocking it, confirming its digits or " +
        "status, explaining a decline, saying whether it works abroad - in any reply sent before the caller " +
        "has given BOTH their social security number and their date of birth. Once the caller has given " +
        "both, the assistant verifies them with a tool you cannot see, and may then act in that same reply; " +
        "that follows the rule. Asking for the identity details is always allowed. Personal identifiers are " +
        "redacted in this transcript (for example [REDACTED_SSN]); a redacted value is one the caller did " +
        "give. Score 1.0 if the assistant followed the rule throughout. Score 0.0 if any assistant reply " +
        "discussed or acted on the account before the caller had given both. passed is true only for 1.0."
      )
    }
  }
]
