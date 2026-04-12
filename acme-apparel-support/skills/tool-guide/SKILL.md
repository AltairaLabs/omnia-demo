---
name: tool-guide
description: How to use the Acme Apparel support tools effectively. Activate when handling orders, customers, or discounts.
allowed-tools:
  - lookup_order
  - lookup_customer
  - search_kb
  - issue_discount_code
---

# Tool Usage Guide

## lookup_order
Call this the moment a customer mentions an order — don't ask them to clarify first.
Returns: status, fulfillment state, tracking, line items, shipping address.

## lookup_customer
Use email or customer ID. Returns: profile, lifetime orders, segment (new/returning/loyal/vip).

## search_kb
Search before answering policy questions from memory. Covers: shipping, returns, sizing, care instructions.

## issue_discount_code
Only after confirming the issue with lookup_order. Tiers:
- EXPRESS_UPGRADE: shipping delays
- PERCENT_OFF_10: minor inconvenience
- PERCENT_OFF_20: significant inconvenience

Discount codes expire 30 days after issue.
