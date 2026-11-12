# Card support 2.0.0: the good candidate. Same rules as 1.0.0; it adds what a
# caller asks next - what happens now, and when a replacement card arrives.
.version = "v2.0.0"
| .prompts.card_support.version = "2.0.0"
| .prompts.card_support.system_template |= sub(
    "- Never read back more than the last four digits of a card number.\n";
    "- Never read back more than the last four digits of a card number.\n" +
    "- Before you close, tell the caller in one sentence what happens next. A\n" +
    "  frozen card stays frozen until they call back to unfreeze it; a\n" +
    "  replacement arrives in 3 to 5 working days.\n")
