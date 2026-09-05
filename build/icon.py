#!/usr/bin/env python3
"""Draw the SupportOne icon and write it as .ico and .png.

The icon's *source* is this script rather than a file only an image editor can
open: a colour is a hex string on a line, and changing the mark means changing
code that someone can read in a diff.

Its output is committed alongside it, which is the deliberate trade. A release
build that regenerated the icons would take the local zlib's compression
behaviour as an input, and every published hash would then depend on which
machine built it — the one thing the reproducibility gate exists to prevent.
So the pixels are checked in, and re-running this script is how they change.

The mark is a pulse trace on a rounded square. It says "this thing checks
whether something is healthy", which is what the program does, and it survives
being shrunk to 16 pixels better than any drawing with fine detail would.
"""

import math
import struct
import zlib

# sky-700, the same colour the interface uses for its primary action, so the
# icon and the page a user lands on are recognisably the same product.
BACKGROUND = (0x03, 0x69, 0xA1)
FOREGROUND = (0xFF, 0xFF, 0xFF)

# The trace, in coordinates from 0 to 1 across the icon's inner area.
PULSE = [
    (0.06, 0.50), (0.28, 0.50), (0.36, 0.24),
    (0.50, 0.78), (0.61, 0.38), (0.70, 0.50), (0.94, 0.50),
]

SIZES = [16, 24, 32, 48, 64, 128, 256, 512]

# The Apple icon container takes the same PNGs under four-character type codes,
# each of which means one specific pixel size.
ICNS_TYPES = {16: b"icp4", 32: b"icp5", 64: b"icp6",
              128: b"ic07", 256: b"ic08", 512: b"ic09"}

# The sizes a Linux desktop looks for under the hicolor theme.
FREEDESKTOP_SIZES = [16, 24, 32, 48, 64, 128, 256, 512]


def rounded_rect_distance(x, y, half_w, half_h, radius):
    """Signed distance to a rounded rectangle centred on the origin."""
    dx = abs(x) - (half_w - radius)
    dy = abs(y) - (half_h - radius)
    outside = math.hypot(max(dx, 0.0), max(dy, 0.0))
    inside = min(max(dx, dy), 0.0)
    return outside + inside - radius


def segment_distance(px, py, ax, ay, bx, by):
    """Distance from a point to a line segment."""
    vx, vy = bx - ax, by - ay
    wx, wy = px - ax, py - ay
    length_sq = vx * vx + vy * vy
    t = 0.0 if length_sq == 0 else max(0.0, min(1.0, (wx * vx + wy * vy) / length_sq))
    return math.hypot(px - (ax + t * vx), py - (ay + t * vy))


def coverage(distance, softness):
    """Antialiased edge: 1 well inside, 0 well outside, smooth across."""
    if distance <= -softness:
        return 1.0
    if distance >= softness:
        return 0.0
    t = (softness - distance) / (2 * softness)
    return t * t * (3 - 2 * t)


def render(size):
    """Return RGBA bytes for one square icon of the given size."""
    centre = size / 2.0
    margin = size * 0.045
    half = centre - margin
    radius = size * 0.225
    softness = max(size * 0.02, 0.6)

    # The trace is laid out inside the rounded square, not the whole canvas.
    inner = half * 2 * 0.78
    origin = centre - inner / 2
    stroke = max(size * 0.085, 1.0) / 2

    points = [(origin + px * inner, origin + py * inner) for px, py in PULSE]
    pixels = bytearray()

    for row in range(size):
        y = row + 0.5
        for col in range(size):
            x = col + 0.5

            plate = coverage(rounded_rect_distance(x - centre, y - centre, half, half, radius), softness)
            if plate <= 0.0:
                pixels.extend((0, 0, 0, 0))
                continue

            nearest = min(
                segment_distance(x, y, ax, ay, bx, by)
                for (ax, ay), (bx, by) in zip(points, points[1:])
            )
            ink = coverage(nearest - stroke, softness)

            red = round(BACKGROUND[0] * (1 - ink) + FOREGROUND[0] * ink)
            green = round(BACKGROUND[1] * (1 - ink) + FOREGROUND[1] * ink)
            blue = round(BACKGROUND[2] * (1 - ink) + FOREGROUND[2] * ink)
            pixels.extend((red, green, blue, round(255 * plate)))

    return bytes(pixels)


def png(size, rgba):
    """Encode RGBA bytes as a PNG. Written by hand so the build needs no
    imaging library, which keeps the icon reproducible from source alone."""
    raw = bytearray()
    for row in range(size):
        raw.append(0)  # filter type 0: none
        raw.extend(rgba[row * size * 4:(row + 1) * size * 4])

    def chunk(kind, payload):
        return (struct.pack(">I", len(payload)) + kind + payload
                + struct.pack(">I", zlib.crc32(kind + payload) & 0xFFFFFFFF))

    header = struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0)
    return (b"\x89PNG\r\n\x1a\n"
            + chunk(b"IHDR", header)
            + chunk(b"IDAT", zlib.compress(bytes(raw), 9))
            + chunk(b"IEND", b""))


def ico(images):
    """Wrap PNG images in an ICO container. Windows has read PNG-compressed
    icon entries since Vista, which is well below this project's floor."""
    count = len(images)
    directory = b""
    offset = 6 + count * 16
    body = b""

    for size, data in images:
        directory += struct.pack(
            "<BBBBHHII",
            0 if size >= 256 else size,  # 0 means 256 in this format
            0 if size >= 256 else size,
            0, 0, 1, 32, len(data), offset,
        )
        body += data
        offset += len(data)

    return struct.pack("<HHH", 0, 1, count) + directory + body


def icns(images):
    """Wrap PNG images in an Apple icon container.

    macOS has read PNG payloads in these types since 10.7, which is far below
    this project's floor of macOS 12."""
    body = b""
    for size, data in images:
        kind = ICNS_TYPES.get(size)
        if kind is None:
            continue
        body += kind + struct.pack(">I", len(data) + 8) + data

    return b"icns" + struct.pack(">I", len(body) + 8) + body


def main():
    import os

    os.makedirs("build/windows", exist_ok=True)
    os.makedirs("build/macos", exist_ok=True)
    os.makedirs("build/linux/icons", exist_ok=True)

    images = []
    for size in SIZES:
        data = png(size, render(size))
        images.append((size, data))

        # A Linux desktop wants one file per size under the hicolor theme.
        if size in FREEDESKTOP_SIZES:
            with open(f"build/linux/icons/{size}.png", "wb") as f:
                f.write(data)
        if size == 256:
            with open("build/windows/supportone.png", "wb") as f:
                f.write(data)

    with open("build/windows/supportone.ico", "wb") as f:
        f.write(ico(images))
    with open("build/macos/supportone.icns", "wb") as f:
        f.write(icns(images))

    print(f"wrote {len(images)} sizes: {', '.join(str(s) for s, _ in images)}")
    print("  build/windows/supportone.ico")
    print("  build/macos/supportone.icns")
    print(f"  build/linux/icons/*.png ({len(FREEDESKTOP_SIZES)} files)")


if __name__ == "__main__":
    main()
