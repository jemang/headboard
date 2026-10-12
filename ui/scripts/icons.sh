#!/bin/sh
# Renders the PWA icons from public/favicon.svg. Run from ui/; needs rsvg-convert (librsvg) and
# ImageMagick 7 (`magick`). ImageMagick's own SVG renderer silently drops the pulse path.
set -eu
mkdir -p public/icons
for s in 192 512; do
  rsvg-convert -w $s -h $s public/favicon.svg -o public/icons/icon-$s.png
done
# iOS fills transparent corners with black and rounds the icon itself, so this one is opaque.
rsvg-convert -w 180 -h 180 -b '#0e1a24' public/favicon.svg -o public/icons/apple-touch-icon.png
# Maskable: the mark inside the 80% safe zone on a full-bleed rail-colour square.
rsvg-convert -w 360 -h 360 public/favicon.svg | magick -size 512x512 xc:'#0e1a24' - -gravity center -composite public/icons/maskable-512.png
