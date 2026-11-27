# UI kit — Larkmere member portal

The signed-in surface: accounts, policies, claims and money movement. Fixed left rail plus a
sticky top bar; the rail switches screens.

| File | Screen |
| --- | --- |
| `index.html` | Entry point |
| `App.jsx` | Route + toast state, chrome composition |
| `Chrome.jsx` | `Rail` (fixed, green), `TopBar` (sticky, hairline), `SectionHead` (ledger heading) |
| `Dashboard.jsx` | Dividend banner, three account cards, activity ledger, policy list |
| `PolicyDetail.jsx` | Tabbed coverage / claims / documents with a premium + adjuster sidebar |
| `Claims.jsx` | Open-claim progress, claim history with filters, file-a-claim form |
| `Transfer.jsx` | Money movement form → confirmation Dialog → success Toast |
| `Assistant.jsx` | "Ask Larkmere" — mint pill launcher + 440px slide-over chat with inline action cards |

## Interactions worth clicking
- Rail navigation between all four screens.
- Policy detail tabs; **Download** on a document raises a toast.
- Claims: filter chips, "Start this claim" raises a toast with a reference number.
- Transfer: "Review this transfer" opens the Dialog; "Send it" confirms with a Toast.
- **Ask Larkmere** (bottom-right pill, on every screen): four suggested openers, a typing state,
  and answers that carry an inline action card. The transfer answer is the one accent action —
  the assistant never acts without a confirm step.
