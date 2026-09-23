#!/usr/bin/env python3
"""Generate original CC0-style pixel sprites for the platform demo (not Nintendo IP)."""

from __future__ import annotations

import json
from pathlib import Path

from PIL import Image, ImageDraw

OUT = Path(__file__).resolve().parents[1] / "frontend" / "public" / "assets" / "platform"


def px(img: Image.Image, x: int, y: int, c: tuple[int, int, int, int]) -> None:
    if 0 <= x < img.width and 0 <= y < img.height:
        img.putpixel((x, y), c)


def fill_rect(draw: ImageDraw.ImageDraw, box, c) -> None:
    draw.rectangle(box, fill=c)


def make_player(frame: str) -> Image.Image:
    """Blue-hoodie adventurer — distinct from Mario palette."""
    w, h = 32, 64
    img = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)

    skin = (255, 214, 170, 255)
    hair = (40, 40, 55, 255)
    hoodie = (56, 132, 220, 255)
    hoodie_dk = (36, 90, 170, 255)
    pants = (45, 55, 80, 255)
    shoe = (30, 30, 40, 255)
    outline = (20, 20, 30, 255)

    crouch = frame == "crouch"
    jump = frame == "jump"
    walk = frame.startswith("walk")

    # feet baseline near bottom
    foot_y = h - 4
    body_h = 18 if crouch else 28
    body_top = foot_y - 8 - body_h
    if jump:
        body_top -= 2

    leg_y0 = foot_y - 8
    # legs
    if walk and frame.endswith("1"):
        fill_rect(d, (10, leg_y0, 14, foot_y), pants)
        fill_rect(d, (18, leg_y0 - 2, 22, foot_y - 2), pants)
        fill_rect(d, (9, foot_y - 2, 15, foot_y + 2), shoe)
        fill_rect(d, (17, foot_y - 4, 23, foot_y), shoe)
    elif walk and frame.endswith("2"):
        fill_rect(d, (10, leg_y0 - 2, 14, foot_y - 2), pants)
        fill_rect(d, (18, leg_y0, 22, foot_y), pants)
        fill_rect(d, (9, foot_y - 4, 15, foot_y), shoe)
        fill_rect(d, (17, foot_y - 2, 23, foot_y + 2), shoe)
    elif jump:
        fill_rect(d, (11, leg_y0 - 4, 15, foot_y - 6), pants)
        fill_rect(d, (17, leg_y0 - 2, 21, foot_y - 2), pants)
        fill_rect(d, (10, foot_y - 6, 16, foot_y - 3), shoe)
        fill_rect(d, (16, foot_y - 2, 22, foot_y + 1), shoe)
    else:
        fill_rect(d, (11, leg_y0, 15, foot_y), pants)
        fill_rect(d, (17, leg_y0, 21, foot_y), pants)
        fill_rect(d, (10, foot_y - 2, 16, foot_y + 2), shoe)
        fill_rect(d, (16, foot_y - 2, 22, foot_y + 2), shoe)

    # torso / hoodie
    fill_rect(d, (8, body_top, 24, leg_y0 + 2), hoodie)
    fill_rect(d, (8, body_top, 24, body_top + 4), hoodie_dk)
    # pocket
    fill_rect(d, (12, body_top + body_h // 2, 20, body_top + body_h // 2 + 5), hoodie_dk)

    # arms
    arm_y = body_top + 6
    if jump:
        fill_rect(d, (4, arm_y - 6, 9, arm_y + 4), hoodie)
        fill_rect(d, (23, arm_y - 6, 28, arm_y + 4), hoodie)
        fill_rect(d, (3, arm_y - 8, 8, arm_y - 4), skin)
        fill_rect(d, (24, arm_y - 8, 29, arm_y - 4), skin)
    elif crouch:
        fill_rect(d, (5, arm_y + 4, 10, arm_y + 12), hoodie)
        fill_rect(d, (22, arm_y + 4, 27, arm_y + 12), hoodie)
    else:
        fill_rect(d, (5, arm_y, 10, arm_y + 12), hoodie)
        fill_rect(d, (22, arm_y, 27, arm_y + 12), hoodie)
        fill_rect(d, (5, arm_y + 10, 10, arm_y + 14), skin)
        fill_rect(d, (22, arm_y + 10, 27, arm_y + 14), skin)

    # head
    head_top = body_top - 14 if not crouch else body_top - 12
    fill_rect(d, (10, head_top, 22, head_top + 14), skin)
    fill_rect(d, (9, head_top, 23, head_top + 5), hair)
    # eyes
    eye_y = head_top + 7
    fill_rect(d, (13, eye_y, 15, eye_y + 3), outline)
    fill_rect(d, (18, eye_y, 20, eye_y + 3), outline)

    # outline hint
    d.rectangle((8, head_top, 23, foot_y + 1), outline=(20, 20, 30, 80))
    return img


def make_enemy() -> Image.Image:
    img = Image.new("RGBA", (32, 32), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    body = (200, 70, 70, 255)
    dark = (140, 40, 40, 255)
    eye = (255, 240, 200, 255)
    pupil = (20, 20, 30, 255)
    fill_rect(d, (4, 8, 28, 28), body)
    fill_rect(d, (4, 8, 28, 14), dark)
    # feet
    fill_rect(d, (6, 26, 12, 31), dark)
    fill_rect(d, (20, 26, 26, 31), dark)
    # eyes
    fill_rect(d, (9, 14, 15, 20), eye)
    fill_rect(d, (17, 14, 23, 20), eye)
    fill_rect(d, (11, 16, 13, 19), pupil)
    fill_rect(d, (19, 16, 21, 19), pupil)
    return img


def make_bullet() -> Image.Image:
    img = Image.new("RGBA", (24, 13), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    fill_rect(d, (0, 3, 18, 10), (240, 200, 60, 255))
    fill_rect(d, (14, 1, 23, 12), (255, 120, 40, 255))
    fill_rect(d, (18, 4, 22, 9), (255, 240, 180, 255))
    return img


def make_box(hit: bool) -> Image.Image:
    img = Image.new("RGBA", (32, 32), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    wood = (196, 140, 70, 255) if not hit else (160, 120, 80, 255)
    edge = (120, 80, 40, 255)
    fill_rect(d, (1, 1, 30, 30), wood)
    d.rectangle((1, 1, 30, 30), outline=edge, width=2)
    # question / empty mark
    if not hit:
        fill_rect(d, (12, 8, 20, 22), (255, 220, 100, 255))
        fill_rect(d, (14, 10, 18, 18), wood)
        fill_rect(d, (14, 19, 18, 21), wood)
    else:
        fill_rect(d, (10, 10, 22, 22), edge)
    return img


def make_item_key() -> Image.Image:
    """Gold key pickup — clearly a collectible, not enemy/hazard."""
    img = Image.new("RGBA", (32, 32), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    gold = (240, 190, 50, 255)
    dark = (170, 120, 30, 255)
    shine = (255, 240, 160, 255)
    # ring (donut)
    fill_rect(d, (5, 5, 17, 17), gold)
    fill_rect(d, (8, 8, 14, 14), (0, 0, 0, 0))
    # clear hole properly
    for y in range(8, 15):
        for x in range(8, 15):
            img.putpixel((x, y), (0, 0, 0, 0))
    d.rectangle((5, 5, 17, 17), outline=dark, width=1)
    # shaft
    fill_rect(d, (15, 9, 28, 13), gold)
    fill_rect(d, (15, 9, 28, 10), shine)
    # teeth
    fill_rect(d, (24, 13, 28, 20), gold)
    fill_rect(d, (20, 13, 23, 17), gold)
    return img


def make_ground() -> Image.Image:
    img = Image.new("RGBA", (32, 32), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    fill_rect(d, (0, 0, 31, 31), (110, 170, 70, 255))
    fill_rect(d, (0, 8, 31, 31), (140, 100, 55, 255))
    for x in range(0, 32, 8):
        fill_rect(d, (x, 8, x + 1, 31), (110, 80, 45, 255))
    return img


def make_water() -> Image.Image:
    img = Image.new("RGBA", (32, 32), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    fill_rect(d, (0, 10, 31, 31), (50, 130, 210, 200))
    fill_rect(d, (0, 10, 31, 14), (120, 200, 255, 220))
    return img


def make_brick() -> Image.Image:
    """Grey masonry fill under the grass/dirt surface tile."""
    img = Image.new("RGBA", (32, 32), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    mortar = (70, 72, 78, 255)
    bricks = [
        (128, 130, 138, 255),
        (118, 120, 128, 255),
        (138, 140, 148, 255),
        (110, 112, 120, 255),
    ]
    fill_rect(d, (0, 0, 31, 31), mortar)
    rows = [
        (1, 14, [1, 16]),
        (16, 30, [-7, 8, 24]),
    ]
    for yi, (y0, y1, xs) in enumerate(rows):
        for xi, x0 in enumerate(xs):
            x1 = x0 + 14
            xa, xb = max(0, x0), min(32, x1)
            if xa >= xb:
                continue
            c = bricks[(yi + xi) % len(bricks)]
            fill_rect(d, (xa, y0, xb - 1, y1), c)
            hi = tuple(min(255, c[i] + 18) for i in range(3)) + (255,)
            sh = tuple(max(0, c[i] - 22) for i in range(3)) + (255,)
            d.line([(xa, y0), (xb - 1, y0)], fill=hi)
            d.line([(xa, y1), (xb - 1, y1)], fill=sh)
    return img


def main() -> None:
    OUT.mkdir(parents=True, exist_ok=True)
    files = {
        "player_idle.png": make_player("idle"),
        "player_walk_1.png": make_player("walk1"),
        "player_walk_2.png": make_player("walk2"),
        "player_jump.png": make_player("jump"),
        "player_crouch.png": make_player("crouch"),
        "enemy.png": make_enemy(),
        "bullet.png": make_bullet(),
        "box.png": make_box(False),
        "box_hit.png": make_box(True),
        "item_key.png": make_item_key(),
        "tile_ground.png": make_ground(),
        "tile_brick.png": make_brick(),
        "tile_water.png": make_water(),
    }
    for name, im in files.items():
        path = OUT / name
        im.save(path)
        print("wrote", path)

    manifest = {
        "tile_px": 32,
        "license": "Original pixel art generated for laya-decision-web. Free to use with the project (treat as CC0).",
        "note": "Not Nintendo / Super Mario assets. Replace with Kenney CC0 packs if desired.",
        "player": {
            "idle": "player_idle.png",
            "walk": ["player_walk_1.png", "player_walk_2.png"],
            "jump": "player_jump.png",
            "crouch": "player_crouch.png",
            "frame_px": [32, 64],
        },
        "enemy": {"idle": "enemy.png", "frame_px": [32, 32]},
        "bullet": {"idle": "bullet.png", "frame_px": [24, 13]},
        "box": {"idle": "box.png", "hit": "box_hit.png", "frame_px": [32, 32]},
        "item": {"idle": "item_key.png", "kind": "key", "frame_px": [32, 32]},
        "tiles": {
            "ground": "tile_ground.png",
            "brick": "tile_brick.png",
            "water": "tile_water.png",
        },
        "kenney_alternatives": [
            "https://kenney.nl/assets/platformer-characters",
            "https://kenney.nl/assets/new-platformer-pack",
            "https://kenney.nl/assets/pixel-platformer",
        ],
    }
    (OUT / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    (OUT / "README.md").write_text(
        """# Platform demo sprites

Original placeholder pixel art for the horizontal platformer demo.

- **Not** Nintendo Super Mario art (do not substitute scraped Mario sheets).
- Generated by `scripts/gen_platform_sprites.py` — regenerate anytime.
- License: treat as **CC0** for this repository.
- For higher quality, swap in [Kenney](https://kenney.nl) CC0 packs listed in `manifest.json` and keep attribution in this folder.
""",
        encoding="utf-8",
    )
    print("manifest + README ok")


if __name__ == "__main__":
    main()
