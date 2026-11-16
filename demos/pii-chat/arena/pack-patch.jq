# Applied to the compiled pack by hack/build-packs.sh, for a field the arena
# source cannot set.
#
# The pack's provider requirements (RFC 0012). packc does not emit `requires`,
# so the slot the scope guardrail names (params.provider: guard) would be
# undeclared: the pack would say nothing about needing a classifier, and the
# host would learn it only when the guardrail fails closed. Declared here, the
# host can check the binding at load, and Omnia's AgentRuntime binds it by name
# (spec.providers[].name: guard).
.requires.providers = [
  {key: "default", role: "llm", required: true},
  {key: "guard", role: "inference", required: true,
   description: "Topic classifier for the scope guardrail (Jev, TypeSafe System One)"}
]
