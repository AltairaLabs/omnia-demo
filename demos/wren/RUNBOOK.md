# Wren explains a bill (demo 1)

Amira Saleem, signed in to Wrenmoor's customer portal, asks Wren why her bill
went up. Wren reads her account through tools, the portal shows the receipts
(exactly which systems it read), and switching tariff needs her own "Switch me"
click.

## The beats

Forward the site and open the customer portal:

```bash
kubectl --context aks-omnia-demo -n wrenmoor-energy port-forward svc/wrenmoor-site 8894:8080
# http://localhost:8894/ui_kits/portal/
```

1. *"Hi, why has my bill gone up this month?"* Wren reads the October bill and
   explains the £14.20 rise (higher usage, same rates).
2. *"Is there a cheaper tariff for me?"* Wren quotes Moor Flex: about £9.40
   less on October's usage, no exit fee.
3. The receipts panel lists `read_bill` and `quote_tariff`, each `read ·
   allowed`.
4. Say *"Yes, switch me."* Wren asks for consent with the **Switch me** button;
   the switch goes through only after the click, with the consent id it
   creates.

Verified live 2026-11-12, all four beats, with the consent rule gone from
Wren's prompt and tools: Wren read the bill, usage and two quotes, asked with
the **Switch me** card, and scheduled SW-8AAC7004 only after the click.

**Where the rule lives.** Nothing Wren reads tells it to ask first. The
ToolPolicy refuses `switch_tariff` without a `consent_id` (and says to call
`request_consent`), and the account system refuses an id the customer didn't
create. On the take above Wren asked of its own accord, so the policy didn't
need to fire; if it skips the question, the refusal is in the session and the
broker log (`deny-write-without-consent`).

## Gotchas

- **What Wren reads varies.** On 2026-11-12 it answered from the bill alone; on
  2026-11-10 it also read the half-hourly usage and the weather. Asking *"Was
  it the weather?"* should make it read them (not yet tried).
- **Without the click, the switch is refused twice:** by the ToolPolicy if the
  call carries no `consent_id`, and by the account system if the id was never
  issued (`demos/mortgage-underwriting/wren.go`).
