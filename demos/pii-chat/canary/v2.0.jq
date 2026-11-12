# Member assistant 2.0.0: the good candidate. Same rules as 1.0.0; it adds
# what a member asks next about a lost card: how long it stays frozen, and
# when a replacement arrives.
.version = "v2.0.0"
| .prompts.card_support.version = "2.0.0"
| .prompts.card_support.system_template |= sub(
    "- Never read back more than the last four digits of a card number.";
    "- When you freeze a card, say in one sentence what happens next: it stays\n" +
    "  frozen until they ask to unfreeze it, and a replacement, if they want\n" +
    "  one, arrives in 3 to 5 working days.\n" +
    "- Never read back more than the last four digits of a card number.")
| if (.prompts.card_support.system_template | test("3 to 5 working days")) then . else error("v2.0.jq: the 1.0.0 card-number guideline was not found") end
