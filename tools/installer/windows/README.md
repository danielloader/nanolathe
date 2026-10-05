# Windows shortcut icon

`Nanolathe.ico` uses the same MIT-licensed Nanolathe website mark as the Mac
app, with 16, 24, 32, 48, 64, 128 and 256 pixel images. Its vector source is
[`../macos/icon.svg`](../macos/icon.svg), copyright 2026 Nanolathe contributors.
Rasterize that SVG at each size and package those images as a multi-image ICO
to regenerate it. Icon generation is an authoring step; players need no image
conversion software.

The Windows installer copies the ICO from the resolved source archive into
the candidate release before promotion. The Start Menu shortcut names that
installed file as its icon, independently of its PowerShell launch target.
Updates replace the shortcut's icon reference alongside the release selection.
