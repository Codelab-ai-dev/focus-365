#!/usr/bin/env bash
# Genera los PNG de la PWA desde los SVG fuente. Re-ejecutar si cambian los SVG.
# Requiere rsvg-convert (preferido) o magick/convert como fallback.
set -euo pipefail
cd "$(dirname "$0")/../public"

render() { # <svg> <size> <out>
  local svg="$1" size="$2" out="$3"
  if command -v rsvg-convert >/dev/null; then
    rsvg-convert -w "$size" -h "$size" "$svg" -o "$out"
  elif command -v magick >/dev/null; then
    magick -background none "$svg" -resize "${size}x${size}" "$out"
  elif command -v convert >/dev/null; then
    convert -background none "$svg" -resize "${size}x${size}" "$out"
  else
    echo "FALLO: instalá rsvg-convert o imagemagick"; exit 1
  fi
}

render icon.svg          192 pwa-192.png
render icon.svg          512 pwa-512.png
render icon-maskable.svg 512 pwa-512-maskable.png
render icon.svg          180 apple-touch-icon.png
echo "Íconos generados: pwa-192.png pwa-512.png pwa-512-maskable.png apple-touch-icon.png"
