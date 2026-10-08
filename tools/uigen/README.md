# uigen — gunmetal battle UI generator

`uigen` draws a remastered set of battle interface elements from three
inputs: a seamless metal material, a font, and optionally Total
Annihilation's palette. Positions come from the retail layout in 1x pixels,
so any integer scale produces the same layout at a higher resolution.

It is an experiment for review. Nothing here is wired into the engine yet:
the output is PNGs.

## What it makes

| Element | 1x size | States |
|---|---|---|
| Top resource bar (`topbar`) | 513×32 | METAL and ENERGY labels, +/− signs, bar grooves fitted to the side data's bar anchors; the game draws the fill and numbers over it |
| Command buttons (`move`, `stop`, `attack`, `patrol`, `guard`, `repair`, `reclaim`, `capture`, `load`, `unload`, `dgun`) | 54×30 | `normal`, `pressed`, `greyed` |
| Order selectors (`fireorders`, `moveorders`) | 113×21 | one file per state, its light lit |

`preview.png` lays the whole set out in a grid.

## Inputs

**Material.** A seamless texture tiled over every surface and graded per
element. The examples use a gunmetal plate generated with an image model from
this prompt; no retail art went into it:

> A seamless, tileable square texture of worn gunmetal plate for a 1990s
> sci-fi strategy game UI. Dark grey-blue steel with fine scratches, small
> dents, light grime and subtle brushed grain. Flat, even lighting with no
> shadows, highlights, vignette or bevels; no rivets, seams, panels, text or
> logos. Low contrast. 1024x1024.

Images like this are rarely perfectly seamless. `uigen tile` removes the
large-scale lighting and blends opposite edges together:

    uigen tile gunmetal.jpg gunmetal-tile.png

The material is not committed: check the image model's terms before shipping
one.

**Font.** The examples use [Saira](https://fonts.google.com/specimen/Saira)
(SIL Open Font Licence) at condensed width and weight 750, an instance cut
from its variable font with [fontTools](https://github.com/fonttools/fonttools):

    fonttools varLib.instancer 'Saira[wdth,wght].ttf' wdth=75 wght=750 -o SairaCondensed-750.ttf

The static Saira Condensed Bold (700) and ExtraBold (800) downloads work too.
`-label-font` sets a different font for the top bar labels.

**Palette (optional).** With `-ta <install>` every element is written as an
8-bit palette-indexed PNG, dithered (Floyd–Steinberg) to the install's
`palettes/palette.pal`, the form retail art takes. Index 9, the colour key
retail GAF art treats as transparent, is never used for opaque pixels.
`-palette` takes a 16×16 swatch image instead.

## Running

    go run . -material gunmetal-tile.png -caption-font SairaCondensed-750.ttf -scale 2 -out out
    go run . ... -ta ~/TotalAnnihilation -out out-8bit

## Tweaking

| Flag | Default | Effect |
|---|---|---|
| `-scale` | `2` | Output scale over retail 1x; every size, thickness and wear mark scales with it |
| `-light` | `bottom-left` | Where light comes from, as in the retail art: rims, recesses, engraved captions, LED highlights, wear and drop shadows all follow it |
| `-cool` | `0.05` | Shifts the material towards blue |
| `-pressed-lift` | `1.75` | Pressed face brightness over normal; retail roughly doubles it |
| `-recess-soft` | `0.35` | Blur on the LED recesses, in 1x pixels |
| `-material`, `-caption-font`, `-label-font` | | Any material or font: a brass or painted plate, another typeface |

Wear (gouges, dents, scratches) is seeded by each button's label, so a button
always wears the same way and neighbours differ. A selector's states share its
wear.

## Why a separate module

It needs `golang.org/x/image` for font rendering and filtering, which the
engine does not depend on. As its own module it keeps the engine's
dependencies unchanged while still reading the palette through the engine's
VFS.
