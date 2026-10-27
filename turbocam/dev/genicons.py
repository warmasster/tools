# Genera los iconos de TurboCAM (ventana, .exe, página del móvil) a partir del
# dibujo de frontend/logo.svg, rehecho aquí con Pillow a 4x y reducido.
#   python dev/genicons.py
import os
from PIL import Image, ImageDraw

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

def lerp(a, b, t):
    return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(3))

def draw(size, pad=True):
    S = size * 4
    k = S / 64.0
    img = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    # degradado 135° violeta -> índigo -> cian
    grad = Image.new("RGBA", (S, S))
    gp = grad.load()
    c0, c1, c2 = (0x8b, 0x5c, 0xf6), (0x63, 0x66, 0xf1), (0x06, 0xb6, 0xd4)
    for y in range(S):
        for x in range(S):
            t = (x + y) / (2.0 * S)
            c = lerp(c0, c1, t / .5) if t < .5 else lerp(c1, c2, (t - .5) / .5)
            gp[x, y] = c + (255,)
    mask = Image.new("L", (S, S), 0)
    m = 2 * k if pad else 0
    ImageDraw.Draw(mask).rounded_rectangle([m, m, S - m, S - m], radius=15 * k if pad else 0, fill=255)
    img.paste(grad, (0, 0), mask)
    d = ImageDraw.Draw(img)
    R = lambda x, y, w, h, r, f: d.rounded_rectangle([x * k, y * k, (x + w) * k, (y + h) * k], radius=r * k, fill=f)
    C = lambda cx, cy, r, f: d.ellipse([(cx - r) * k, (cy - r) * k, (cx + r) * k, (cy + r) * k], fill=f)
    R(21, 15.5, 13, 8, 3, "#ffffff")
    R(12, 20, 40, 29, 8, "#ffffff")
    C(32, 34.5, 10, "#4f46e5")
    C(32, 34.5, 5.2, "#a5f3fc")
    C(30.2, 32.7, 1.6, "#ffffff")
    C(45.2, 26.3, 2.1, "#06b6d4")
    return img.resize((size, size), Image.LANCZOS)

big = draw(1024)
big.save(os.path.join(ROOT, "build", "appicon.png"))
sizes = [16, 20, 24, 32, 40, 48, 64, 96, 128, 256]
draw(256).save(os.path.join(ROOT, "build", "windows", "icon.ico"), sizes=[(s, s) for s in sizes])
# Iconos de la app instalable del móvil: a sangre (el sistema les pone su forma).
draw(192, pad=False).save(os.path.join(ROOT, "web", "icon-192.png"), optimize=True)
draw(512, pad=False).save(os.path.join(ROOT, "web", "icon-512.png"), optimize=True)
print("iconos listos")
