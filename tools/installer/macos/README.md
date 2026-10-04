# macOS launcher resources

`icon.svg` is the Nanolathe website's favicon mark, copied from
`nanolathe-website/static/brand/favicon.svg`. It is MIT licensed, copyright
2026 Nanolathe contributors; the repository's LICENSE retains that notice.
`Nanolathe.icns` contains this mark at the standard 16, 32, 128, 256 and 512
point icon sizes, each at 1× and 2× resolution. To regenerate it, rasterize
the SVG to an `Nanolathe.iconset` directory using these names:

```text
icon_16x16.png       icon_16x16@2x.png
icon_32x32.png       icon_32x32@2x.png
icon_128x128.png     icon_128x128@2x.png
icon_256x256.png     icon_256x256@2x.png
icon_512x512.png     icon_512x512@2x.png
```

The `@2x` image has twice the named width and height. On macOS, run
`iconutil -c icns Nanolathe.iconset -o Nanolathe.icns`.
Icon generation is an authoring step; installers use the committed ICNS.

`update-progress.js` runs under `/usr/bin/osascript -l JavaScript` and uses
AppKit directly. Its arguments are the status-file path and ICNS path.
The launcher owns the private status file and progress process; it closes and
reaps the window on either update outcome. Installer stage messages replace
the status file atomically, and presentation failure never blocks installing
or starting the previous release. The activity bar is indeterminate because
source compilation has no reliable completion fraction.
