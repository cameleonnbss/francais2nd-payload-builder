# -*- coding: utf-8 -*-
"""Icône de l'application « Français — Collège & Lycée ».

Dessin vectoriel avec Pillow, sans dépendance externe : carré arrondi en
dégradé indigo → violet, livre ouvert blanc vu de face (bords extérieurs plus
hauts que le pli), signet doré. Rendu en 1024 px puis réduit (LANCZOS) pour
chaque taille du .ico, afin que les petits formats restent nets.
"""
import os
from PIL import Image, ImageDraw, ImageFilter

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "assets")
os.makedirs(OUT, exist_ok=True)

S = 1024
R = 220
TOP = (31, 42, 107)      # indigo profond
BOTTOM = (124, 58, 237)  # violet


def gradient(size):
    img = Image.new("RGB", (1, size))
    d = ImageDraw.Draw(img)
    for y in range(size):
        t = y / max(1, size - 1)
        d.point((0, y), fill=tuple(int(TOP[i] + (BOTTOM[i] - TOP[i]) * t) for i in range(3)))
    return img.resize((size, size), Image.BILINEAR)


# --- fond : carré arrondi + halo -------------------------------------------------
icon = Image.new("RGBA", (S, S), (0, 0, 0, 0))
mask = Image.new("L", (S, S), 0)
ImageDraw.Draw(mask).rounded_rectangle([0, 0, S - 1, S - 1], radius=R, fill=255)
icon.paste(gradient(S).convert("RGBA"), (0, 0), mask)

halo = Image.new("RGBA", (S, S), (0, 0, 0, 0))
ImageDraw.Draw(halo).ellipse([-S * 0.35, -S * 0.55, S * 0.75, S * 0.45],
                             fill=(255, 255, 255, 46))
halo = halo.filter(ImageFilter.GaussianBlur(90))
icon = Image.alpha_composite(
    icon, Image.composite(halo, Image.new("RGBA", (S, S), (0, 0, 0, 0)), mask))

# --- livre ouvert ---------------------------------------------------------------
cx, cy = S // 2, int(S * 0.525)
half_w, half_h = int(S * 0.335), int(S * 0.205)
gap = int(S * 0.010)
lift = int(S * 0.055)     # les bords extérieurs remontent
curve = int(S * 0.030)    # creux du pli central

# ombre portée sous le livre
shadow = Image.new("RGBA", (S, S), (0, 0, 0, 0))
ImageDraw.Draw(shadow).rounded_rectangle(
    [cx - half_w, cy - half_h + 30, cx + half_w, cy + half_h + 50],
    radius=46, fill=(0, 0, 0, 95))
icon = Image.alpha_composite(icon, shadow.filter(ImageFilter.GaussianBlur(38)))
d = ImageDraw.Draw(icon)

white = (255, 255, 255, 255)
edge = (203, 213, 225, 255)
line = (148, 163, 184, 170)

# épaisseur : deux pages sous la couverture
for k, col in ((2, (203, 213, 225, 255)), (1, (226, 232, 240, 255))):
    dy = k * int(S * 0.012)
    d.polygon([
        (cx - gap, cy - half_h + curve + dy),
        (cx - gap, cy + half_h + dy),
        (cx - half_w, cy + half_h - lift + dy),
        (cx - half_w, cy - half_h - lift + dy),
    ], fill=col)
    d.polygon([
        (cx + gap, cy - half_h + curve + dy),
        (cx + gap, cy + half_h + dy),
        (cx + half_w, cy + half_h - lift + dy),
        (cx + half_w, cy - half_h - lift + dy),
    ], fill=col)

# page gauche
d.polygon([
    (cx - gap, cy - half_h + curve),
    (cx - gap, cy + half_h),
    (cx - half_w, cy + half_h - lift),
    (cx - half_w, cy - half_h - lift),
], fill=white)
# page droite, légèrement grisée
d.polygon([
    (cx + gap, cy - half_h + curve),
    (cx + gap, cy + half_h),
    (cx + half_w, cy + half_h - lift),
    (cx + half_w, cy - half_h - lift),
], fill=(241, 245, 249, 255))

# pli central
d.line([(cx, cy - half_h + curve), (cx, cy + half_h)], fill=edge, width=int(S * 0.009))

# lignes de texte, en perspective
if S >= 256:
    for i in range(4):
        y0 = cy - half_h + curve + int(S * 0.052) * (i + 1)
        d.line([(cx - half_w + int(S * 0.055), y0 - lift + int(S * 0.012)),
                (cx - gap - int(S * 0.040), y0)],
               fill=line, width=int(S * 0.006))
        d.line([(cx + gap + int(S * 0.040), y0),
                (cx + half_w - int(S * 0.055), y0 - lift + int(S * 0.012))],
               fill=line, width=int(S * 0.006))

# signet doré
rib = int(S * 0.030)
rx = cx + int(half_w * 0.50)
d.polygon([
    (rx, cy - half_h + curve + int(S * 0.015)),
    (rx + rib, cy - half_h + curve + int(S * 0.015)),
    (rx + rib, cy + int(S * 0.075)),
    (rx + rib // 2, cy + int(S * 0.045)),
    (rx, cy + int(S * 0.075)),
], fill=(245, 197, 66, 255))

icon = icon.convert("RGBA")

# --- sorties --------------------------------------------------------------------
icon.resize((1024, 1024), Image.LANCZOS).save(os.path.join(OUT, "icon-1024.png"))
icon.resize((256, 256), Image.LANCZOS).save(os.path.join(OUT, "icon-256.png"))
icon.resize((512, 512), Image.LANCZOS).save(os.path.join(OUT, "icon.png"))

sizes = [(256, 256), (128, 128), (64, 64), (48, 48), (32, 32), (24, 24), (16, 16)]
frames = [icon.resize(sz, Image.LANCZOS) for sz in sizes]
frames[0].save(os.path.join(OUT, "icon.ico"), format="ICO",
               sizes=sizes, append_images=frames[1:])
print("icône écrite :", os.path.join(OUT, "icon.ico"))
