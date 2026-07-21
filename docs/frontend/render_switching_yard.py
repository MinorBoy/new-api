"""
Switching Yard — landing page canvas for new-api.
Second pass: refined craftsmanship.
- Real cubic Bezier curves for the convergence diagram.
- Faint dot grid for the scientific-plate feel.
- Fixed, disciplined header dispatch row.
- Tick-marked stat counters.
- Zebra-striped upstream board with hanging punctuation.
- Crosshair registration marks on cards.
"""
import math
import os
from PIL import Image, ImageDraw, ImageFont, ImageFilter

# ─── canvas ────────────────────────────────────────────────────────────
W, H = 1440, 2400
MARGIN_X = 120
COL_W = (W - 2 * MARGIN_X) / 12

FONT_DIR = r"C:\Users\880pro\.zcode\skills\canvas-design\canvas-fonts"
OUT = r"C:\Users\880pro\Documents\new-api\docs\frontend\landing-switching-yard.png"

# ─── palette ───────────────────────────────────────────────────────────
BG          = (20, 22, 27)
BG_CARD     = (32, 34, 40)
BG_POPOVER  = (36, 38, 45)
BG_ZEBRA    = (26, 28, 33)      # subtle row tint
INK         = (240, 241, 243)
INK_MUTED   = (167, 170, 178)
INK_DIM     = (110, 114, 124)
INK_FAINT   = (72, 75, 84)
RULE        = (44, 47, 54)
RULE_SOFT   = (32, 34, 40)
SIGNAL      = (99, 145, 255)
SIGNAL_DEEP = (63, 102, 214)
SIGNAL_GLOW = (140, 178, 255)
SUCCESS     = (130, 200, 160)

# ─── fonts ─────────────────────────────────────────────────────────────
def F(name, size):
    return ImageFont.truetype(os.path.join(FONT_DIR, name), size)

f_display   = lambda s: F("Lora-Bold.ttf", s)
f_display_r = lambda s: F("Lora-Regular.ttf", s)
f_display_i = lambda s: F("Lora-Italic.ttf", s)
f_mono      = lambda s: F("GeistMono-Regular.ttf", s)
f_mono_b    = lambda s: F("GeistMono-Bold.ttf", s)
f_sans      = lambda s: F("InstrumentSans-Regular.ttf", s)
f_sans_b    = lambda s: F("InstrumentSans-Bold.ttf", s)
f_sans_i    = lambda s: F("InstrumentSans-Italic.ttf", s)


def measure(draw, text_str, font):
    bbox = draw.textbbox((0, 0), text_str, font=font)
    return bbox[2] - bbox[0], bbox[3] - bbox[1]


def text(draw, xy, s, font, fill, anchor="lm", ls=None):
    """typeset with optional letter-spacing; supports lm/rm/mm anchors."""
    if ls and ls > 0:
        widths = [measure(draw, ch, font)[0] for ch in s]
        total = sum(widths) + ls * (len(s) - 1)
        x, y = xy
        ax = anchor[0]
        if ax == "l":
            cx = x
        elif ax == "m":
            cx = x - total / 2
        elif ax == "r":
            cx = x - total
        ay = anchor[1] if len(anchor) > 1 else "m"
        if ay == "m":
            bb = draw.textbbox((0, 0), s, font=font)
            cy = y - (bb[3] - bb[1]) / 2 - bb[1]
        elif ay == "t":
            cy = y
        else:  # bottom
            bb = draw.textbbox((0, 0), s, font=font)
            cy = y - (bb[3] - bb[1])
        for i, ch in enumerate(s):
            draw.text((cx, cy), ch, font=font, fill=fill)
            cx += widths[i] + ls
        return
    draw.text(xy, s, font=font, fill=fill, anchor=anchor)


def bezier(p0, p1, p2, p3, n=60):
    """cubic bezier polyline."""
    pts = []
    for i in range(n + 1):
        t = i / n
        mt = 1 - t
        x = mt**3 * p0[0] + 3*mt**2*t*p1[0] + 3*mt*t**2*p2[0] + t**3*p3[0]
        y = mt**3 * p0[1] + 3*mt**2*t*p1[1] + 3*mt*t**2*p2[1] + t**3*p3[1]
        pts.append((x, y))
    return pts


# ─── base ──────────────────────────────────────────────────────────────
img = Image.new("RGBA", (W, H), BG + (255,))
d = ImageDraw.Draw(img, "RGBA")

# ─── dot grid (felt, not seen) ─────────────────────────────────────────
dot = Image.new("RGBA", (W, H), (0, 0, 0, 0))
dd = ImageDraw.Draw(dot)
for gx in range(MARGIN_X, W - MARGIN_X + 1, 40):
    for gy in range(0, H, 40):
        dd.point((gx, gy), fill=(255, 255, 255, 9))
img.alpha_composite(dot)

# 12-col vertical hairline guides (felt)
for c in range(13):
    x = MARGIN_X + c * COL_W
    d.line([(x, 0), (x, H)], fill=RULE_SOFT + (30,), width=1)


# ======================================================================
# SECTION 1 — HEADER DISPATCH BAR
# ======================================================================
HEADER_Y = 70
by = HEADER_Y
bx = MARGIN_X

# brand glyph: three lines converging into one signal trunk
for dy in [-7, 0, 7]:
    d.line([(bx, by + dy), (bx + 22, by)], fill=INK, width=2)
d.line([(bx + 22, by), (bx + 42, by)], fill=SIGNAL, width=2)
d.ellipse([bx + 19, by - 3, bx + 25, by + 3], fill=BG, outline=SIGNAL, width=1)

BRAND = "YSRouter"
text(d, (bx + 58, by), BRAND, f_mono_b(15), INK, ls=2)
brand_w = measure(d, BRAND, f_mono_b(15))[0] + 2 * (len(BRAND) - 1)
text(d, (bx + 58 + brand_w + 16, by), "/  GATEWAY", f_mono(11), INK_DIM, ls=2)

# Right-side dispatch row — clean, monospaced, no overlap.
# Format:  ● ALL LINES CLEAR     REV 2k6e8r7p     EN·ZH
rx_right = W - MARGIN_X
items_right = [
    ("EN·ZH", INK_MUTED, f_mono(11)),
    ("2k6e8r7p", INK_MUTED, f_mono(11)),
    ("●  ALL LINES CLEAR", SUCCESS, f_mono(11)),
]
cursor_x = rx_right
for val, col, fnt in items_right:
    w = measure(d, val, fnt)[0] + (sum(1 for _ in val) * 1.5)  # +ls slack
    if val.startswith("●"):
        text(d, (cursor_x, by), val, fnt, col, anchor="rm", ls=1.5)
    else:
        text(d, (cursor_x, by), val, fnt, col, anchor="rm", ls=1.5)
    cursor_x -= measure(d, val, fnt)[0] + 28

# hairline
d.line([(MARGIN_X, HEADER_Y + 32), (W - MARGIN_X, HEADER_Y + 32)], fill=RULE, width=1)


# ======================================================================
# SECTION 2 — TRACK DIAGRAM (real bezier convergence)
# ======================================================================
DIAGRAM_TOP = HEADER_Y + 80
DIAGRAM_BOT = HEADER_Y + 460
TRUNK_X = W // 2
trunk_y = (DIAGRAM_TOP + DIAGRAM_BOT) / 2

ARRIVAL_COUNT = 7
# arriving (left → trunk)
for i in range(ARRIVAL_COUNT):
    t = i / (ARRIVAL_COUNT - 1)
    y_start = DIAGRAM_TOP + t * (DIAGRAM_BOT - DIAGRAM_TOP)
    y_mid = trunk_y + (t - 0.5) * 60
    alpha = 50 + int(25 * (1 - abs(t - 0.5) * 2))
    pts = bezier(
        (MARGIN_X - 20, y_start),
        (MARGIN_X + 220, y_start),
        (TRUNK_X - 220, y_mid),
        (TRUNK_X - 40, y_mid),
        n=70,
    )
    d.line(pts, fill=INK_DIM + (alpha,), width=1)
    d.ellipse([MARGIN_X - 24, y_start - 2, MARGIN_X - 18, y_start + 2],
              fill=INK_FAINT + (200,))

# departing (trunk → right)
for i in range(ARRIVAL_COUNT):
    t = i / (ARRIVAL_COUNT - 1)
    y_end = DIAGRAM_TOP + t * (DIAGRAM_BOT - DIAGRAM_TOP)
    y_mid = trunk_y + (t - 0.5) * 60
    alpha = 50 + int(25 * (1 - abs(t - 0.5) * 2))
    pts = bezier(
        (TRUNK_X + 40, y_mid),
        (TRUNK_X + 220, y_mid),
        (W - MARGIN_X - 220, y_end),
        (W - MARGIN_X + 20, y_end),
        n=70,
    )
    d.line(pts, fill=INK_DIM + (alpha,), width=1)
    d.ellipse([W - MARGIN_X + 18, y_end - 2, W - MARGIN_X + 24, y_end + 2],
              fill=INK_FAINT + (200,))

# trunk (signal blue, with soft glow)
for w, a in [(14, 18), (8, 32), (4, 60)]:
    d.line([(TRUNK_X - 40, trunk_y), (TRUNK_X + 40, trunk_y)],
           fill=SIGNAL + (a,), width=w)
d.line([(TRUNK_X - 40, trunk_y), (TRUNK_X + 40, trunk_y)], fill=SIGNAL_GLOW, width=2)
for x in [TRUNK_X - 40, TRUNK_X + 40]:
    d.ellipse([x - 5, trunk_y - 5, x + 5, trunk_y + 5], fill=BG, outline=SIGNAL, width=1)
    d.ellipse([x - 2, trunk_y - 2, x + 2, trunk_y + 2], fill=SIGNAL_GLOW)

# engraved captions
text(d, (MARGIN_X, DIAGRAM_TOP - 18), "FIG.01  CONVERGENCE", f_mono(9), INK_FAINT, ls=1.5)
text(d, (W - MARGIN_X, DIAGRAM_BOT + 18), "n=7  →  1  →  n=7",
     f_mono(9), INK_FAINT, anchor="rm", ls=1.5)


# ======================================================================
# SECTION 3 — HERO STATEMENT
# ======================================================================
HERO_Y = 720
hero_x = MARGIN_X + 10

text(d, (hero_x, HERO_Y - 150), "— 01 / AI APPLICATION INFRASTRUCTURE FOUNDATION",
     f_mono(10), SIGNAL, ls=2.5)

line1 = "Unified API Gateway"
line2_a = "for a "
line2_b = "Vast Range"
line2_c = " of AI Models"

text(d, (hero_x, HERO_Y - 60), line1, f_display(96), INK, anchor="lm")

l2a = measure(d, line2_a, f_display(96))[0]
l2b = measure(d, line2_b, f_display_i(96))[0]
ly = HERO_Y + 60
text(d, (hero_x, ly), line2_a, f_display(96), INK, anchor="lm")
text(d, (hero_x + l2a, ly), line2_b, f_display_i(96), SIGNAL_GLOW, anchor="lm")
d.line([(hero_x + l2a, ly + 56), (hero_x + l2a + l2b, ly + 56)],
       fill=SIGNAL + (120,), width=2)
text(d, (hero_x + l2a + l2b, ly), line2_c, f_display(96), INK, anchor="lm")

text(d, (hero_x, HERO_Y + 170),
     "Access a vast selection of models via a standard, unified protocol.",
     f_sans(18), INK_MUTED, anchor="lm")
text(d, (hero_x, HERO_Y + 200),
     "Power applications, manage digital assets, and connect the Future.",
     f_sans(18), INK_MUTED, anchor="lm")

# CTAs
cta_y = HERO_Y + 280
pw, ph = 220, 56
d.rounded_rectangle([hero_x, cta_y, hero_x + pw, cta_y + ph], radius=6, fill=SIGNAL)
text(d, (hero_x + pw / 2, cta_y + ph / 2), "GET  STARTED",
     f_mono_b(13), (255, 255, 255), anchor="mm", ls=2)
ax = hero_x + pw / 2 + 72
ay = cta_y + ph / 2
d.polygon([(ax, ay - 5), (ax + 8, ay), (ax, ay + 5)], fill=(255, 255, 255))

sx = hero_x + pw + 20
sw, sh = 170, 56
d.rounded_rectangle([sx, cta_y, sx + sw, cta_y + sh], radius=6, outline=RULE, width=1)
text(d, (sx + sw / 2, cta_y + sh / 2), "VIEW  PRICING",
     f_mono(12), INK, anchor="mm", ls=2)

tx = sx + sw + 36
text(d, (tx, cta_y + sh / 2 - 8), "DOCS", f_mono_b(12), INK_MUTED, ls=2)
text(d, (tx, cta_y + sh / 2 + 10), "documentation  →", f_mono(9), INK_FAINT, ls=1)


# ======================================================================
# SECTION 4 — STAT STRIP (dispatch counters with tick marks)
# ======================================================================
STAT_Y = HERO_Y + 400
d.line([(MARGIN_X, STAT_Y - 30), (W - MARGIN_X, STAT_Y - 30)], fill=RULE, width=1)

stats = [
    ("50+", "upstream services"),
    ("100+", "model billing"),
    ("50+", "compatible routes"),
    ("10+", "scheduling controls"),
]
col_w = (W - 2 * MARGIN_X) / 4
for i, (num, lbl) in enumerate(stats):
    cx = MARGIN_X + col_w * i + 20
    text(d, (cx, STAT_Y), num, f_display_r(64), INK, anchor="lm")
    nw = measure(d, num, f_display_r(64))[0]
    # vertical tick beside number (like a counter gauge)
    d.line([(cx + nw + 14, STAT_Y - 22), (cx + nw + 14, STAT_Y + 22)],
           fill=SIGNAL + (140,), width=1)
    # tiny scale ticks on the gauge
    for tk in range(-2, 3):
        d.line([(cx + nw + 11, STAT_Y + tk * 10),
                (cx + nw + 14, STAT_Y + tk * 10)], fill=INK_FAINT + (160,), width=1)
    text(d, (cx, STAT_Y + 52), lbl.upper(), f_mono(10), INK_MUTED, ls=1.5)
    if i < 3:
        dx = MARGIN_X + col_w * (i + 1)
        d.line([(dx, STAT_Y - 20), (dx, STAT_Y + 60)], fill=RULE, width=1)
d.line([(MARGIN_X, STAT_Y + 90), (W - MARGIN_X, STAT_Y + 90)], fill=RULE, width=1)


# ======================================================================
# SECTION 5 — CORE FEATURES BENTO (yard bays)
# ======================================================================
FEAT_Y = STAT_Y + 170
text(d, (MARGIN_X, FEAT_Y), "— 02 / CORE FEATURES", f_mono(10), SIGNAL, ls=2.5)
text(d, (MARGIN_X, FEAT_Y + 36), "Four bays of capability.", f_display_r(40), INK)
text(d, (MARGIN_X, FEAT_Y + 80), "Built for developers, designed for scale.",
     f_sans_i(15), INK_MUTED)

BAY_Y = FEAT_Y + 140
bay_w = (W - 2 * MARGIN_X - 30) / 2
bay_h = 200
bays = [
    ("01", "Lightning Fast", "Optimized network architecture ensures millisecond response across every routed path."),
    ("02", "Secure & Reliable", "Enterprise-grade security with comprehensive permission management at every junction."),
    ("03", "Global Coverage", "Multi-region deployment for stable access from any terminus on the map."),
    ("04", "Developer Friendly", "Compatible API routes for common AI application workflows, drop-in base URL."),
]
for i, (idx, title, desc) in enumerate(bays):
    col = i % 2
    row = i // 2
    bx0 = MARGIN_X + col * (bay_w + 30)
    by0 = BAY_Y + row * (bay_h + 24)
    d.rounded_rectangle([bx0, by0, bx0 + bay_w, by0 + bay_h],
                        radius=10, fill=BG_CARD)
    d.line([(bx0 + 16, by0), (bx0 + 60, by0)], fill=SIGNAL, width=2)
    text(d, (bx0 + 28, by0 + 32), idx, f_mono(13), INK_FAINT, ls=2)
    text(d, (bx0 + 28, by0 + 60), title, f_display_r(28), INK)
    # wrap description to card width
    words = desc.split()
    lines, cur = [], ""
    for w_ in words:
        if len(cur) + len(w_) + 1 <= 44:
            cur = (cur + " " + w_).strip()
        else:
            lines.append(cur); cur = w_
    if cur: lines.append(cur)
    for li, ln in enumerate(lines[:3]):
        text(d, (bx0 + 28, by0 + 110 + li * 24), ln, f_sans(14), INK_MUTED)
    # crosshair registration mark at corner
    cx_, cy_ = bx0 + bay_w - 24, by0 + 24
    d.line([(cx_ - 5, cy_), (cx_ + 5, cy_)], fill=INK_FAINT, width=1)
    d.line([(cx_, cy_ - 5), (cx_, cy_ + 5)], fill=INK_FAINT, width=1)


# ======================================================================
# SECTION 6 — UPSTREAM SERVICES BOARD (departure board, zebra-striped)
# ======================================================================
MOD_Y = BAY_Y + 2 * bay_h + 24 + 110
text(d, (MARGIN_X, MOD_Y), "— 03 / UPSTREAM SERVICES", f_mono(10), SIGNAL, ls=2.5)
text(d, (MARGIN_X, MOD_Y + 36), "Many lines, one protocol.", f_display_r(40), INK)
text(d, (MARGIN_X, MOD_Y + 80),
     "Each upstream arrives on its own terms; all depart in one shape.",
     f_sans_i(15), INK_MUTED)

BOARD_Y = MOD_Y + 140
BOARD_H = 380
board_x0 = MARGIN_X
board_x1 = W - MARGIN_X
d.rounded_rectangle([board_x0, BOARD_Y, board_x1, BOARD_Y + BOARD_H],
                    radius=10, fill=BG_CARD)

hdr_y = BOARD_Y + 28
cols = [
    ("ID",        board_x0 + 28,  "l"),
    ("UPSTREAM",  board_x0 + 100, "l"),
    ("FAMILY",    board_x0 + 380, "l"),
    ("PROTOCOL",  board_x1 - 230, "l"),
    ("STATUS",    board_x1 - 60,  "l"),
]
for label, cx_, _ in cols:
    text(d, (cx_, hdr_y), label, f_mono_b(10), INK_FAINT, ls=2)
d.line([(board_x0 + 28, hdr_y + 20), (board_x1 - 28, hdr_y + 20)], fill=RULE, width=1)

providers = [
    ("001", "OpenAI",     "GPT",        "/v1/chat"),
    ("002", "Anthropic",  "Claude",     "/v1/messages"),
    ("003", "Google",     "Gemini",     "/v1beta"),
    ("004", "Mistral",    "Mistral",    "/v1/chat"),
    ("005", "DeepSeek",   "DeepSeek",   "/v1/chat"),
    ("006", "xAI",        "Grok",       "/v1/chat"),
    ("007", "Cohere",     "Command",    "/v1/chat"),
    ("008", "Moonshot",   "Kimi",       "/v1/chat"),
    ("009", "Zhipu",      "GLM",        "/v1/chat"),
    ("010", "Volcano",    "Doubao",     "/v1/chat"),
]
row_h = 28
for i, (pid, name, fam, proto) in enumerate(providers):
    ry = hdr_y + 40 + i * row_h
    # zebra stripe
    if i % 2 == 1:
        d.rectangle([board_x0 + 2, ry - row_h/2 + 4,
                     board_x1 - 2, ry + row_h/2 - 4], fill=BG_ZEBRA + (120,))
    text(d, (board_x0 + 28, ry), pid, f_mono(12), INK_DIM, ls=1)
    text(d, (board_x0 + 100, ry), name, f_mono(12), INK, ls=0.5)
    text(d, (board_x0 + 380, ry), fam, f_mono(12), INK_MUTED, ls=0.5)
    text(d, (board_x1 - 230, ry), proto, f_mono(12), INK_DIM, ls=0.5)
    col = SIGNAL if i % 5 == 2 else SUCCESS
    d.ellipse([board_x1 - 60, ry - 4, board_x1 - 52, ry + 4], fill=col)
    text(d, (board_x1 - 44, ry), "CLEAR" if i % 5 == 2 else "READY",
         f_mono(10), INK_MUTED, ls=1.5)

more_y = hdr_y + 40 + len(providers) * row_h + 20
d.line([(board_x0 + 28, more_y), (board_x1 - 28, more_y)], fill=RULE, width=1)
text(d, (board_x0 + 28, more_y + 18), "+  40  MORE  UPSTREAM  SERVICES  INTEGRATED",
     f_mono(11), SIGNAL, ls=2)


# ======================================================================
# SECTION 7 — PRICING TIERS
# ======================================================================
PR_Y = BOARD_Y + BOARD_H + 110
text(d, (MARGIN_X, PR_Y), "— 04 / PRICING", f_mono(10), SIGNAL, ls=2.5)
text(d, (MARGIN_X, PR_Y + 36), "Three platforms, one yard.", f_display_r(40), INK)
text(d, (MARGIN_X, PR_Y + 80),
     "Pay for what you route. Transparent billing, metered to the token.",
     f_sans_i(15), INK_MUTED)

tiers = [
    ("STARTER",   "0",      "/mo", ["50+ upstreams", "Standard routes", "Community support"], False),
    ("PRO",       "29",     "/mo", ["All upstreams", "Priority routing", "Usage analytics", "Email support"], True),
    ("ENTERPRISE","Custom", "",    ["Self-hosted", "SSO / SAML", "Audit logs", "Dedicated support"], False),
]
TIER_Y = PR_Y + 140
tier_w = (W - 2 * MARGIN_X - 60) / 3
tier_h = 320
for i, (name, price, unit, feats, highlight) in enumerate(tiers):
    tx0 = MARGIN_X + i * (tier_w + 30)
    ty0 = TIER_Y
    fill = BG_POPOVER if highlight else BG_CARD
    d.rounded_rectangle([tx0, ty0, tx0 + tier_w, ty0 + tier_h],
                        radius=10, fill=fill,
                        outline=(SIGNAL if highlight else None), width=1)
    d.line([(tx0 + 20, ty0), (tx0 + 70, ty0)],
           fill=(SIGNAL if highlight else RULE), width=(2 if highlight else 1))
    text(d, (tx0 + 28, ty0 + 28), name,
         f_mono_b(11), (SIGNAL if highlight else INK_MUTED), ls=3)

    price_y = ty0 + 90
    if price == "Custom":
        text(d, (tx0 + 28, price_y), price, f_display_r(48), INK)
    else:
        dw = measure(d, "$", f_display_r(24))[0]
        text(d, (tx0 + 28, price_y), "$", f_display_r(24), INK_DIM)
        text(d, (tx0 + 28 + dw + 4, price_y - 4), price, f_display_r(56), INK)
        pw_ = measure(d, price, f_display_r(56))[0]
        text(d, (tx0 + 28 + dw + 4 + pw_ + 8, price_y + 20), unit, f_mono(12), INK_DIM)

    for fi, feat in enumerate(feats):
        fy = ty0 + 180 + fi * 26
        d.line([(tx0 + 28, fy - 3), (tx0 + 34, fy + 3)], fill=SIGNAL, width=1)
        d.line([(tx0 + 34, fy + 3), (tx0 + 42, fy - 5)], fill=SIGNAL, width=1)
        text(d, (tx0 + 54, fy), feat, f_sans(14), INK_MUTED)


# ======================================================================
# SECTION 8 — FINAL CTA (terminus)
# ======================================================================
CTA_Y = TIER_Y + tier_h + 130
trunk_y2 = CTA_Y + 30
# three faint feeding lines on each side (real beziers)
for dy in [-22, 0, 22]:
    pts_l = bezier((MARGIN_X, trunk_y2 + dy), (MARGIN_X + 200, trunk_y2 + dy),
                   (W/2 - 240, trunk_y2), (W/2 - 120, trunk_y2), n=50)
    d.line(pts_l, fill=INK_FAINT + (110,), width=1)
    pts_r = bezier((W/2 + 120, trunk_y2), (W/2 + 240, trunk_y2),
                   (W - MARGIN_X - 200, trunk_y2 + dy), (W - MARGIN_X, trunk_y2 + dy), n=50)
    d.line(pts_r, fill=INK_FAINT + (110,), width=1)
d.line([(W/2 - 120, trunk_y2), (W/2 + 120, trunk_y2)], fill=SIGNAL, width=2)
for x in [W/2 - 120, W/2 + 120]:
    d.ellipse([x - 4, trunk_y2 - 4, x + 4, trunk_y2 + 4], fill=BG, outline=SIGNAL, width=1)

text(d, (W / 2, CTA_Y + 80), "Ready to simplify your AI integration?",
     f_display_r(44), INK, anchor="mm")
text(d, (W / 2, CTA_Y + 130),
     "Deploy your own gateway. Start routing through configured upstreams.",
     f_sans_i(16), INK_MUTED, anchor="mm")

bw_, bh_ = 200, 54
d.rounded_rectangle([W/2 - bw_ - 12, CTA_Y + 175, W/2 - 12, CTA_Y + 175 + bh_],
                    radius=6, fill=SIGNAL)
text(d, (W/2 - 12 - bw_/2, CTA_Y + 175 + bh_/2), "GET  STARTED",
     f_mono_b(13), (255, 255, 255), anchor="mm", ls=2)
d.rounded_rectangle([W/2 + 12, CTA_Y + 175, W/2 + 12 + bw_, CTA_Y + 175 + bh_],
                    radius=6, outline=RULE, width=1)
text(d, (W/2 + 12 + bw_/2, CTA_Y + 175 + bh_/2), "VIEW  PRICING",
     f_mono(12), INK, anchor="mm", ls=2)


# ======================================================================
# SECTION 9 — FOOTER (colophon)
# ======================================================================
FOOT_Y = H - 100
d.line([(MARGIN_X, FOOT_Y - 40), (W - MARGIN_X, FOOT_Y - 40)], fill=RULE, width=1)
text(d, (MARGIN_X, FOOT_Y), "YSROUTER  /  GATEWAY", f_mono_b(11), INK, ls=2)
text(d, (MARGIN_X, FOOT_Y + 22), "Next-generation LLM gateway & AI asset management",
     f_sans_i(12), INK_DIM)
text(d, (W - MARGIN_X, FOOT_Y), "©  2023–2026  QUANTUMNOUS",
     f_mono(10), INK_MUTED, anchor="rm", ls=1.5)
text(d, (W - MARGIN_X, FOOT_Y + 22), "AGPL-3.0  ·  self-hostable",
     f_mono(9), INK_DIM, anchor="rm", ls=1.5)

# plate registration marks at four corners
for cx, cy in [(MARGIN_X - 30, HEADER_Y), (W - MARGIN_X + 30, HEADER_Y),
               (MARGIN_X - 30, H - 60), (W - MARGIN_X + 30, H - 60)]:
    d.line([(cx - 6, cy), (cx + 6, cy)], fill=INK_FAINT, width=1)
    d.line([(cx, cy - 6), (cx, cy + 6)], fill=INK_FAINT, width=1)

img = img.convert("RGB")
img.save(OUT, "PNG", optimize=True)
print(f"saved: {OUT}  {img.size}")
