# Overview — HTML + CSS

A standalone copy of the shop overview page. Two files, no build step, no
JavaScript.

```
overview-html-css/
  index.html    the page
  styles.css    all the styling
  README.md     this file
```

## Open it

Double-click `index.html`, or serve the folder:

```
python3 -m http.server 8000
```

then visit http://localhost:8000

The fonts (Manrope for headings, DM Sans for body) load from Google Fonts, so
the page needs an internet connection to look exactly as designed. Offline it
falls back to the system sans-serif.

## Editing

Everything visual is driven by the tokens at the top of `styles.css` —
colours, corner radius, fonts and shadows. Change one line there and every
card that uses it follows.

The layout has three breakpoints:

- above 1100px — five metric cards in a row, three setup cards in a row
- 760–1100px — tighter gutters, metric labels hidden
- below 760px — metrics in two columns, setup cards stacked

The numbers on the metric cards are written directly in `index.html`, so
editing them there is the fastest way to update the page.
