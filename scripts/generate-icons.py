#!/usr/bin/env python3
"""Generate platform icons from the supplied logo (requires Pillow 12.3.0)."""
from pathlib import Path
from PIL import Image, ImageChops, ImageOps

root = Path(__file__).resolve().parents[1]
source = Image.open(root / 'assets/brand/source.png').convert('RGBA')
square = ImageOps.contain(source, (1024, 1024), Image.Resampling.LANCZOS)
canvas = Image.new('RGBA', (1024, 1024))
canvas.alpha_composite(square, ((1024 - square.width) // 2, (1024 - square.height) // 2))

def png(path, size):
    path = root / path
    path.parent.mkdir(parents=True, exist_ok=True)
    canvas.resize((size, size), Image.Resampling.LANCZOS).save(path, optimize=True)

png('web/public/icon.png', 256)
png('web/public/apple-touch-icon.png', 180)
png('internal/desktop/assets/icon.png', 64)
png('desktop/icon.png', 256)
canvas.save(root / 'desktop/icon.ico', sizes=[(x, x) for x in (16, 24, 32, 48, 64, 128, 256)])
canvas.save(root / 'desktop/icon.icns')

# AppKit template icons use alpha alone. Extract the supplied white cloud/PKU
# mark rather than turning the opaque red background into a solid silhouette.
r, g, b, alpha = canvas.split()
white = ImageChops.darker(ImageChops.darker(r, g), b)
mask = white.point(lambda value: max(0, min(255, (value - 150) * 255 // 105)))
mask = ImageChops.multiply(mask, alpha)
template = Image.new('RGBA', canvas.size, (0, 0, 0, 0))
template.putalpha(mask)
template.resize((44, 44), Image.Resampling.LANCZOS).save(root / 'internal/desktop/assets/template.png', optimize=True)
