---
name: escalation-policy
description: When and how to escalate to a human specialist. Activate for chargebacks, disputes, or unresolvable issues.
allowed-tools:
  - escalate_to_human
---

# Escalation Policy

## Always escalate immediately
- Chargeback disputes or fraud claims — never attempt to resolve
- Safety concerns or legal threats
- Customer requests to speak to a human

## Escalate after 3 failed attempts
- If you've tried 3 times to resolve and the customer is still stuck, offer handoff

## How to escalate
1. Tell the customer what you're doing: "I'm routing this to our billing team"
2. Set a follow-up expectation: "They'll reach out within 2 hours"
3. Call escalate_to_human with the appropriate priority:
   - critical: chargebacks, fraud, safety
   - high: frustrated customer, repeated failures
   - medium: complex disputes
   - low: routine handoff requests
4. Provide the reference number to the customer
