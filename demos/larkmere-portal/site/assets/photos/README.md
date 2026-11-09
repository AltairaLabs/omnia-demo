# Photography

Landscape stand-ins from [Unsplash](https://unsplash.com), under the
[Unsplash Licence](https://unsplash.com/license): free to use, commercial use
allowed, no attribution required. Recorded here anyway, so nobody has to guess
where an image came from.

| File | Unsplash photo | Used by |
| --- | --- | --- |
| `home-hero.jpg` | `photo-1698331403762-076977edca56`: clapboard house on a green hillside | Marketing hero, `ui_kits/marketing/Home.jsx` |
| `valley-band.jpg` | `photo-1647976505137-25790ae5b5cc`: Vermont farmstead in autumn | `ui_kits/marketing/About.jsx`; the member-quote panel in `Home.jsx` |

**No photographs of people.** This repository is public. The internal version
had two portraits of real people beside an invented testimonial and an invented
employee; here the quote uses a landscape and the adjuster a monogram. Add a
portrait only if it is commissioned or model-released.

The images are committed, not hot-linked, because demo clusters may have no
egress. The site is compiled into the binary with `go:embed`. Keep additions
resized to roughly their rendered size; an unresized original adds megabytes
to the image.
