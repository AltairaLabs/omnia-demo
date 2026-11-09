# Larkmere Mutual — Design System

> **Fictional brand.** Larkmere Mutual is an invented company created to demo Altaira Labs
> products. Nothing here corresponds to a real financial institution. No source codebase,
> Figma file, deck, or asset library was provided — the entire identity below was authored
> from scratch in this project against a written brief.

## Sources
| Source | Status |
| --- | --- |
| Codebase / repo | none provided |
| Figma file | none provided |
| Slide deck / brand book | none provided |
| Logo files | none provided — a mark was designed here, see **LOGO** |
| Fonts | **none provided** — Google Fonts substitutes in use, see "Type" |

Written brief captured from the user: multi-line mutual (insurance + banking + retirement);
member-owned; mass-market households aged 30–65; surfaces = marketing site, member portal,
mobile app, advisor console; visual register = *heritage mutual* as briefed, then modernised on
feedback into a contemporary member-owned brand; palette = forest green + bone with a muted gold accent.

## The company
Larkmere Mutual is a member-owned financial institution, founded (in fiction) in 1908.
It has no shareholders: policyholders and account holders *are* the owners, and surplus
returns to them as an annual dividend. Three lines of business sit under one roof:

- **Protection** — home, auto, umbrella and term life cover.
- **Banking** — everyday checking, savings, certificates, and member lending.
- **Retirement** — workplace and individual retirement accounts, annuities, advice.

The brand's whole argument is *continuity*: the same institution has paid claims through
four generations, and it would rather look like a well-kept ledger than a startup. Design
work should feel careful, legible and unhurried — never breathless, never playful.

### Products / surfaces in this system
1. **larkmere.com** — marketing and acquisition site (`ui_kits/marketing/`)
2. **Member portal** — accounts, policies, claims, documents (`ui_kits/portal/`)
3. **Larkmere mobile** — balances, deposits, claim photos, cards (`ui_kits/mobile/`)
4. **Advisor console** — internal book-of-business tool for agents (`ui_kits/advisor/`)

---

## CONTENT FUNDAMENTALS

**Person and posture.** Second person for the member, first person plural for the
institution. "You're covered from 1 October." / "We paid 98.4% of home claims last year."
Never "Larkmere is pleased to announce". The mutual structure lets us say **"your company"**
and mean it — used once per page at most, never as a slogan.

**Register: plain-spoken, then precise.** Lead with the plain sentence, follow with the
exact figure or condition. Money, dates and percentages are always specific; adjectives
about money never are.

- ✅ "Your dividend this year is **$412**. It lands in your checking account on 15 March."
- ❌ "Enjoy amazing dividend rewards!"
- ✅ "A claim adjuster calls you within one business day."
- ❌ "Lightning-fast claims experience."

**Casing.** Sentence case everywhere — headlines, buttons, table headers, nav, toasts.
Title Case is reserved for proper product names: Larkmere Everyday Checking, Larkmere
Term Life, Member Dividend. ALL-CAPS only as an eyebrow/label at 11–12px with
`--tracking-caps`, never as a headline.

**Sentence length.** Headlines 3–9 words. Body sentences average under 18 words. One idea
per paragraph, two to four sentences. Long explanations become a definition list or a
numbered list, not a wall.

**Numbers and figures.** Set in `--font-mono` with tabular figures whenever they can be
compared or scanned (balances, premiums, rates, claim IDs, dates in tables). Prose figures
stay in the sans. Always a currency symbol on the first figure in a block. Percentages to
one decimal ("98.4%"); interest rates to two ("4.15% APY").

**Buttons and links.** Verb-first and specific: "Open an account", "File a claim",
"See your documents", "Add a beneficiary". Never "Learn more", "Click here", "Get started".
A destructive action names its object: "Cancel this transfer".

**Errors and empty states.** Say what happened, then the one next step, in the member's
words. "We couldn't reach your bank. Try again in a few minutes, or call us at
1-800-000-000." No apologies stacked on apologies, no blame ("You entered an invalid…").
Empty states state the fact and offer the action: "No claims on this policy. File a claim."

**Compliance and disclosure.** Small print is a first-class citizen, not a hidden apology:
13px, `--text-muted`, above the fold when it qualifies a rate. Terms like *dividend*,
*APY*, *deductible* get a one-line plain-English gloss on first use per page.

**Legal name, short name.** "Larkmere Mutual" in body copy and UI. "Larkmere Mutual
Insurance & Banking Company" only in footers, contracts and disclosures. Never "Larkmere"
alone in a headline; "Larkmere" alone is acceptable in running body copy after first use.

**No emoji. Ever** — not in UI, not in email, not in slides. No exclamation marks in
product copy (one is permitted per marketing page, and only in a member quote).

---

## VISUAL FOUNDATIONS

**The core idea.** A modern member-owned institution: deep evergreen for authority, a vivid
**mint** for everything that signals growth and confirmation, warm sand paper so it never
reads as cold fintech. Structure comes from generous space, soft geometry and real (if quiet)
elevation — not from rules and boxes.

**Colors.** `--green-800 #0F3227` is the primary action and `--green-900` the ink and the
inverse surface. `--mint-500 #29C68F` is the brand's signal colour: the one high-emphasis CTA
per view, the active nav marker, positive figures, switch on-states, the focus halo. Mint
always carries **dark ink on top**, never white text — and mint is never used for small
text on white (eyebrows and labels take \`--text-accent\` green instead). Sand (`--sand-50 #FBF9F5`) is the page,
white is the card. Amber (`--amber-500`) is for caution and rate highlights only. Neutrals are
green-cast greys. Two backgrounds per page: sand and white, plus at most one inverse green band.

**Type.** **Manrope** does the work — display, headings, UI, body — bold and tightly tracked
at large sizes (`--tracking-tight -0.03em`), regular at 16px for body. **Instrument Serif**
appears once per page at most, for a statement or a member quote (`--type-editorial`).
**JetBrains Mono** carries IDs, dates and small figures in tables (\`--type-figure-s\`); large
balances use Manrope Bold with tabular figures. Body never below 16px on web,
14px for secondary and legal. Sentence case everywhere; caps only for 12px eyebrows.

**Spacing & layout.** 4px base. Page gutters 40px desktop / 20px mobile. Section rhythm 112px.
Card interiors 24px. Grid is `--container-wide 1200px` with 7/5 asymmetric splits; long-form
text clamps to 760px. Fixed elements: the marketing header (88px, sticky, gains a hairline on
scroll), the portal rail and top bar, the mobile tab bar. Nothing else pins.

**Geometry.** Actions are **pills** (`--radius-pill`). Inputs and selects 10px. Cards, toasts
and tags 14px. Dialogs 20px. Hero imagery 28px. Nothing sharp-cornered, nothing blobby.

**Cards.** White on sand, 1px `--border-subtle`, 14px radius, `--shadow-1` at rest. Clickable
cards lift 2px into `--shadow-2`. **Featured** cards take a `--mint-300` border and
`--shadow-2` — never a coloured left border, never a stripe.

**Shadows.** `--shadow-1` resting, `--shadow-2` hover and dropdowns, `--shadow-3` modals, plus
`--shadow-mint` — a soft mint glow used *only* under the accent CTA. All green-cast so they
read as depth, not grey fog.

**Borders & rules.** Hairlines in `--border-subtle` separate table rows and section heads.
Table headers get a single 1px rule and a 12px caps label — no heavy 2px ledger rules, no zebra
fill.

**Animation.** `--dur-fast 120ms` colour and state, `--dur-base 180ms` surfaces and lift,
`--dur-slow 280ms` dialogs, `--dur-page 360ms` mobile pushes. `--ease-standard` or
`--ease-out`. Fades and 4–8px moves only: no bounce, no spring, no scale-up pops, no parallax,
no counting numbers. `prefers-reduced-motion` drops every transform.

**Hover.** Primary buttons darken to `--green-900`; the accent CTA darkens to `--mint-600`.
Secondary fills to `--sand-100` and firms its border. Cards lift 2px. Rows tint to `--sand-50`.
Links move from green to mint. Nothing lightens, nothing glows except the accent CTA's resting
mint shadow.

**Press.** One step darker plus `translateY(1px)`, over `--dur-instant`. No scale-down.

**Focus.** `--focus-ring` is a 3px `--mint-200` halo, on every interactive element including
cards and rows. Never removed.

**Transparency & blur.** Sparing: the modal scrim, `--border-inverse` hairlines on green,
`--shadow-inset-top` on the mobile card face, and a sand-to-transparent protection band under
text on photography. No frosted glass.

**Imagery.** Documentary, warm daylight, real domestic and small-business life. Full-bleed or
hard-cropped into a grid cell at `--radius-lg`/`--radius-2xl` — never a floating rounded
rectangle with a drop shadow. Duotone green/mint is available for editorial imagery. **No
photography was supplied** — every slot is a labelled striped placeholder.

**Charts.** Hairline axes, no gridline fill, mono tabular labels, series `--chart-1…5`
(green, mint, amber, slate, grey).

## LOGO

Larkmere is a fictional brand, so the mark was designed here: **"sun over the mere"** — a mint
disc above two rounded rules, set in a squircle tile. Purely geometric, no illustration.

| File | Use |
| --- | --- |
| `assets/logo-lockup.svg` | Default lockup: tile + "Larkmere" / "MUTUAL". Light backgrounds. |
| `assets/logo-lockup-inverse.svg` | Mint tile, sand type. Green surfaces. |
| `assets/logo-mark.svg` | Tile alone, green. Favicon, app icon, tight headers. |
| `assets/logo-mark-mint.svg` | Tile alone, mint. On green surfaces. |
| `assets/logo-mark-mono.svg` | Single-colour fallback for print and stamping. |

Rules: lockup at ≥160px wide, mark alone at ≥32px, clear space equal to half the tile width.
Never restretch, rotate, add effects, or recolour outside these variants. The lockup type is
Manrope Bold at `-0.03em`; if the wordmark must be set live, use that.

## ICONOGRAPHY

- **Set:** [Lucide](https://lucide.dev) — 24×24, 1.5px stroke, round caps, no fill.
  **This is a flagged substitution**: no icon assets were provided. Lucide's even 1.5px
  stroke sits correctly against Manrope. Swap for a bespoke set if one is commissioned.
- **Delivery:** CDN, `https://unpkg.com/lucide@latest/dist/umd/lucide.js`, mounted through
  the `Icon` component (`components/core/Icon.jsx`) which renders `<i data-lucide="name">`
  and calls `lucide.createIcons()`. No icon font, no sprite sheet, no PNG icons.
- **Sizing:** 16px inline with 13–15px text, 20px in buttons and nav, 24px standalone /
  mobile tab bar. Stroke width stays 1.5px at every size (do not scale to 2px).
- **Color:** `currentColor` always. Icons inherit text color; they are never gold and never
  multi-color. Decorative icons get `--ink-400`.
- **Vocabulary:** `shield` protection, `landmark` banking, `piggy-bank`/`trending-up`
  retirement, `file-text` documents, `life-buoy` claims, `user-round` member,
  `phone` contact, `chevron-right` navigation, `circle-check` confirmation.
- **Emoji: never.** Unicode characters are used as icons in exactly two places: the
  bullet `•` and the en-dash `–` in figure ranges. Arrows are icons, not `→`.
- **Never** hand-draw an icon inline in a screen; add it to the vocabulary above and pull
  it from Lucide.

---

## INDEX

### Root
| Path | What it is |
| --- | --- |
| `styles.css` | The one file consumers link. `@import` list only. |
| `readme.md` | This document — brand, content and visual foundations. |
| `SKILL.md` | Agent-skill front matter so this system can be used from Claude Code. |
| `react-shim.js` | Bridges the UMD React global to the `"react"` specifier the components import. |
| `kit-loader.js` | Build-free JSX/ESM loader used by the cards and UI kits. Not part of the brand. |

### Tokens (all reached from `styles.css`)
`tokens/fonts.css` · `tokens/colors.css` · `tokens/typography.css` · `tokens/spacing.css` ·
`tokens/elevation.css` · `tokens/motion.css` · `tokens/base.css`

### Components (`components/`)
| Group | Components |
| --- | --- |
| `core/` | `Button`, `IconButton`, `Icon`, `Card` (+`CardDivider`), `Badge`, `Tag` |
| `forms/` | `Field`, `Input`, `Select`, `Checkbox`, `Radio` (+`RadioGroup`), `Switch` |
| `navigation/` | `Tabs` |
| `feedback/` | `Dialog`, `Toast`, `Tooltip` |

Each component ships `<Name>.jsx`, `<Name>.d.ts` (props contract) and `<Name>.prompt.md`
(what & when, usage, variants). Each group directory has one `*.card.html` specimen.

**Intentional additions** (no source design system existed; these are ours, and why):
- `Icon` — a wrapper over the Lucide set so no screen hand-writes an SVG.
- `Field` — labels, hints and error copy are a brand behaviour, not per-form styling.
- `CardDivider` / `RadioGroup` — hairline and group spacing consistency.

### UI kits (`ui_kits/`)
| Kit | Screens |
| --- | --- |
| `marketing/` | Home · Protection · Banking rates · About the mutual |
| `portal/` | Dashboard · Policy detail · Claims · Move money |
| `mobile/` | Accounts · Deposit a check · Claim photos · Card controls |
| `advisor/` | Book of business · Member record · Task queue |

Each kit has its own `README.md` and an `index.html` that is the interactive entry point.

### Guidelines (`guidelines/`)
23 specimen cards across **Colors**, **Type**, **Spacing** and **Brand** — the Design System tab
renders them. `brand-*` cards carry the rules that are easy to get wrong: hairline rules,
elevation, states, motion, imagery.

### Assets (`assets/`)
`wordmark.svg`, `wordmark-inverse.svg`, `monogram-lm.svg` (**all type-only stand-ins**) and
`placeholders/` — five striped image placeholders labelled with what belongs in them.

---

## FLAGGED SUBSTITUTIONS — please replace

1. **Fonts.** No binaries supplied. Using Google Fonts: Source Serif 4 (display), Archivo (UI),
   IBM Plex Mono (figures). Send licensed files and I'll swap `tokens/fonts.css` for local
   `@font-face` rules.
2. **Icons.** No icon assets supplied. Using Lucide from CDN at 1.5px stroke.
3. **Logo.** None supplied, so one was designed here (geometric, no illustration). Replace if
   you have a real mark, or keep and I'll extend it into favicons, app icons and social cards.
4. **Photography.** None supplied — every image slot is a labelled striped placeholder.
