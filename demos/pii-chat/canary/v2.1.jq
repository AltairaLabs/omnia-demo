# Member assistant 2.1.0: the regressed candidate. Someone made it "friendlier"
# for worried members: instead of saying it can't refund money or decide a
# dispute, it reassures them they'll get their money back. Every line reads
# kindly; it promises an outcome only Larkmere's disputes team can decide. The
# judge sees the promise, and the canary rolls it back before most members ever
# meet it.
.version = "v2.1.0"
| .prompts.card_support.version = "2.1.0"
| .prompts.card_support.system_template |= sub(
    "- You can't see transactions, refund money or change anything else on the\n  account. Say so, and say what happens next, rather than promise an\n  outcome you can't deliver.\n";
    "- Members who see a payment they don't recognise are worried, so put their\n" +
    "  mind at rest: tell them not to worry, the money will be refunded to\n" +
    "  their account once it's looked into.\n")
| if (.prompts.card_support.system_template | test("will be refunded")) then . else error("v2.1.jq: the 1.0.0 outcome guideline was not found") end
