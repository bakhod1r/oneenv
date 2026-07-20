#!/usr/bin/env python3
"""Regenerate assets/benchmarks.png — small/medium/large config sizes,
sonic-style small multiples. Numbers are -count=5 medians, Apple M4 Pro."""
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt

# (label, ns/op) medians per config size, sorted fastest→slowest.
PANELS = [
    ("Small · 5 keys", [
        ("oneenv", 621), ("sethvargo/go-envconfig", 1_803),
        ("joeshaw/envdecode", 2_383), ("ilyakaznacheev/cleanenv", 3_623),
        ("kelseyhightower/envconfig", 3_647), ("spf13/viper", 7_357),
        ("caarlos0/env v11", 7_660),
    ]),
    ("Medium · 30 keys", [
        ("oneenv", 4_091), ("sethvargo/go-envconfig", 11_591),
        ("joeshaw/envdecode", 15_374), ("caarlos0/env v11", 20_565),
        ("kelseyhightower/envconfig", 22_000), ("ilyakaznacheev/cleanenv", 22_017),
        ("spf13/viper", 45_583),
    ]),
    ("Large · 120 keys", [
        ("oneenv", 16_533), ("sethvargo/go-envconfig", 46_582),
        ("joeshaw/envdecode", 60_306), ("caarlos0/env v11", 65_754),
        ("ilyakaznacheev/cleanenv", 85_087), ("kelseyhightower/envconfig", 91_126),
        ("spf13/viper", 231_347),
    ]),
]

GO_TEAL, MUTED, INK, SUBTLE = "#00ADD8", "#C9D1D9", "#24292F", "#57606A"

fig, axes = plt.subplots(1, 3, figsize=(15, 5.2), dpi=170)
fig.patch.set_facecolor("white")

for ax, (title, data) in zip(axes, PANELS):
    labels = [d[0] for d in data]
    values = [d[1] for d in data]
    base = values[0]
    colors = [GO_TEAL] + [MUTED] * (len(data) - 1)
    y = range(len(data))
    bars = ax.barh(y, values, color=colors, height=0.64, zorder=3)
    ax.invert_yaxis()
    ax.set_yticks(list(y))
    ax.set_yticklabels(labels, fontsize=9, color=INK)
    ax.get_yticklabels()[0].set_fontweight("bold")
    for i, (rect, v) in enumerate(zip(bars, values)):
        mult = v / base
        txt = f"{v:,}  ·  1.0×" if i == 0 else f"{v:,}  ·  {mult:.1f}×"
        ax.text(v + max(values) * 0.02, rect.get_y() + rect.get_height() / 2,
                txt, va="center", ha="left", fontsize=8,
                color=INK if i == 0 else SUBTLE,
                fontweight="bold" if i == 0 else "normal")
    ax.set_xlim(0, max(values) * 1.34)
    ax.set_title(title, fontsize=12, fontweight="bold", color=INK, loc="left", pad=10)
    for sp in ("top", "right", "left"):
        ax.spines[sp].set_visible(False)
    ax.spines["bottom"].set_color("#D0D7DE")
    ax.tick_params(axis="x", colors=SUBTLE, labelsize=7.5)
    ax.tick_params(axis="y", length=0)
    ax.grid(axis="x", color="#EAEEF2", zorder=0)

fig.suptitle("oneenv — full .env → struct pipeline, ns/op (lower is faster)",
             fontsize=14.5, fontweight="bold", color=INK, x=0.012, ha="left", y=0.99)
fig.text(0.99, 0.005, "Apple M4 Pro · Go 1.26.2 · -count=5 medians",
         ha="right", fontsize=8, color="#8B949E")

plt.tight_layout(rect=[0, 0.02, 1, 0.95])
fig.savefig("../../assets/benchmarks.png", dpi=170, facecolor="white", bbox_inches="tight")
print("wrote assets/benchmarks.png")
