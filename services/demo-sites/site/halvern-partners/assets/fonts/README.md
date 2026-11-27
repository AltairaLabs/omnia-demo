# Halvern Partners fonts

Vendored, because the cluster has no egress and a hot-linked Google Fonts
stylesheet would silently fall back to system fonts on camera.

Latin-subset woff2 files, downloaded from Google Fonts (fonts.gstatic.com) on
2026-11-10 via the css2 API for the families and weights the handoff names.
`tokens/fonts.css` declares them. IBM Plex Sans, IBM Plex Mono and Newsreader
are variable fonts, so one file covers each weight range and the file name
carries only its lowest weight.

| Family | Weights | Used on |
|---|---|---|
| Libre Caslon Display | 400 | Display headlines, big stats |
| Libre Caslon Text | 400, 400 italic, 700 | Sub-heads, quotes, firm names, file notes |
| IBM Plex Sans | 400–600 | Body and UI |
| IBM Plex Mono | 400–500 | Data on the Practice Desk and design system |
| Newsreader | 400–500 | The Wrenmoor Energy wordmark only |

All are licensed under the SIL Open Font License 1.1, which permits bundling
and redistribution. Manrope, which the Larkmere Mutual lockup names, is not
vendored: the lockup is an `<img>`, which cannot load web fonts, so its text
falls back as the handoff says.
