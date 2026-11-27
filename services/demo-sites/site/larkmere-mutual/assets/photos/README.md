# Photography

The design system shipped striped placeholders here — it says so itself, under
FLAGGED SUBSTITUTIONS: *"no photography was supplied"*. These are stand-ins for
an **internal demo**, chosen to make the fictional company read as real on
camera.

## Source and licence

All four are from [Unsplash](https://unsplash.com), under the
[Unsplash Licence](https://unsplash.com/license): free to use, commercial use
allowed, no permission or attribution required. Attribution is recorded here
anyway, because a photo with no provenance in a repo is a photo nobody can make
a decision about later.

| File | Unsplash photo | Used by |
| --- | --- | --- |
| `home-hero.jpg` | `photo-1698331403762-076977edca56` — clapboard house on a green hillside | Marketing hero, `ui_kits/marketing/Home.jsx` |
| `valley-band.jpg` | `photo-1647976505137-25790ae5b5cc` — Vermont farmstead in autumn | Full-bleed band, `ui_kits/marketing/About.jsx` |
| `member-quote.jpg` | `photo-1779019982983-206314d45f14` — portrait, outdoors | Member quote, `ui_kits/marketing/Home.jsx` |
| `adjuster-ruth.jpg` | `photo-1615464670798-6e92fafa2a89` — portrait | "Your adjuster", `ui_kits/portal/PolicyDetail.jsx` |

## Why they are committed rather than hot-linked

**The cluster has no egress.** This is the same constraint that forced React,
Babel and Lucide to be vendored: an `images.unsplash.com` URL renders a blank
frame in-cluster with only a console error to say why, and you find out during a
take. The site is compiled into the binary with `go:embed`, so these travel with
it.

Sizes are deliberately modest — 743 KB for all four, against ~3 MB for Babel —
and each is fetched at roughly the dimensions it renders at, not full
resolution. If you add one, do the same: an unresized Unsplash original is
several megabytes and goes straight into the image.

## Two people appear here

`member-quote.jpg` sits beside an invented testimonial and `adjuster-ruth.jpg`
beside an invented employee. Both are real photographs of real people who are
neither. That is within the Unsplash licence and fine for an internal demo, but
it is the reason this file exists: **if this demo ever goes to customers or on a
public site, replace these two with commissioned or model-released photography,
or drop back to a monogram avatar.** The landscapes carry no such caveat.
