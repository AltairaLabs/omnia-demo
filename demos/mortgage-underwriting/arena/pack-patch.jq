# Applied to the compiled pack by hack/build-packs.sh, for a field the arena
# source cannot set.
#
# The pack's provider middleware timeout. packc emits 30000ms, and this
# function's final turn is one long structured answer after seven tool calls:
# at 30s the provider call is cut mid-answer. 180s matches the Provider's
# streamIdleTimeout headroom in the chart (120s) with room for a slow first
# token.
.prompts.underwriting.pipeline.middleware |= map(
  if .type == "provider" then .config.timeout_ms = 180000 else . end
)
