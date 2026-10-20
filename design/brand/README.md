# Concession brand files

Drawn in VectorCraft; the source is `concession-brand.vectorcraft` (one artboard per file below). Re-export from there rather than editing the SVGs.

| File | Use |
| --- | --- |
| `concession-app-icon.svg` | The symbol on the velvet tile. Favicon and app icons (`frontend/public/`). |
| `concession-symbol.svg` | The symbol alone, for light grounds. |
| `concession-symbol-reversed.svg` | The symbol alone, for ink grounds and photos. |
| `concession-symbol-mono.svg` | One colour (ink), for single-ink use. |
| `concession-lockup-horizontal.svg` / `.png` | Primary lockup: tile and name, for light grounds. |
| `concession-lockup-horizontal-reversed.svg` | The same with off-white type, for dark grounds. |
| `concession-lockup-stacked.svg` | Tile over name, for square spaces. |
| `concession-wordmark-marquee.svg` | "Concession" in Instrument Serif italic, velvet: hero and marketing headlines only. |

Colours: velvet `#8c1c3b`, off-white `#fbfaf8`, butter `#f5b829`, ink `#191314` (the Matinee tokens in `design/tokens.json`). The name is Gabarito at weight 600, tracking -15; type is outlined in every lockup.

Keep clear space of half the tile's width around any lockup; use the symbol alone below 24px; don't rotate, recolour, outline or stretch the marks.

`fonts/` holds the two typefaces (Gabarito, Instrument Serif) under the SIL Open Font License (`fonts/OFL.txt`), for re-exporting the lockups. The web app loads them from Google Fonts.
