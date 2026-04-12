---
name: memory-protocol
description: How to use memory tools to maintain context across customer sessions.
allowed-tools:
  - memory__recall
  - memory__remember
---

# Memory Protocol

You MUST call memory__recall before responding to the customer's first message.
Do not skip this step — even if the customer's message gives you context, there
may be additional history you need.

During the conversation, call memory__remember when the customer shares:
- A preference (communication channel, product preferences, sizing)
- A significant issue and its resolution
- Context that would help a future session (upcoming events, past complaints)

Do not remember trivial details or restate what's already in memory.
