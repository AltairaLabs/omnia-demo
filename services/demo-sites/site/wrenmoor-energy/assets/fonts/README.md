# Wrenmoor Energy fonts

Vendored, because the cluster has no egress and a hot-linked Google Fonts
stylesheet would silently fall back to system fonts on camera.

Latin-subset variable woff2 files, downloaded from Google Fonts (fonts.gstatic.com)
on 2026-11-09 via the css2 API for the families and axes the handoff names:

| File | Family | Axes |
|---|---|---|
| `newsreader-latin.woff2` | Newsreader | opsz 6–72, wght 400–500 |
| `instrument-sans-latin.woff2` | Instrument Sans | wght 400–600 |
| `jetbrains-mono-latin.woff2` | JetBrains Mono | wght 400–500 |

All three are licensed under the SIL Open Font License 1.1, which permits
bundling and redistribution. Characters outside the Latin subset (✓, for one)
fall back to the system stack in `tokens/fonts.css`.
