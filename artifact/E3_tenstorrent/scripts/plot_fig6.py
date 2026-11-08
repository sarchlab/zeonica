#!/usr/bin/env python3
"""Draw Fig. 6: Zeonica vs TT-Sim device-side cycles on the 10 held-out kernels.

Horizontal grouped bars (Zeonica above TT-Sim in each group), log x axis, the
per-kernel APE at the end of each group and the MAPE in the corner. Style
(rcParams, colors, spines) follows the paper's plot_rtl_comparison.py (Fig. 4).

usage: plot_fig6.py --input results/fig6_data.csv --output-dir figures
"""
import argparse
import csv
import statistics
from pathlib import Path

import matplotlib as mpl

mpl.use("Agg")
import matplotlib.pyplot as plt  # noqa: E402
import matplotlib.ticker as mticker  # noqa: E402
import numpy as np  # noqa: E402

FONTSIZE = 10.0
mpl.rcParams.update(
    {
        "font.family": "DejaVu Serif",
        "mathtext.fontset": "dejavuserif",
        "font.size": FONTSIZE,
        "axes.labelsize": FONTSIZE,
        "xtick.labelsize": FONTSIZE,
        "ytick.labelsize": FONTSIZE,
        "legend.fontsize": FONTSIZE,
        "figure.dpi": 300,
        "savefig.dpi": 300,
        "pdf.fonttype": 42,
        "axes.linewidth": 0.5,
        "grid.linewidth": 0.4,
        "grid.alpha": 0.5,
        "grid.color": "#cccccc",
        "grid.linestyle": ":",
        "xtick.major.width": 0.5,
        "ytick.major.width": 0.5,
        "xtick.direction": "out",
        "ytick.direction": "out",
        "xtick.major.size": 3,
        "ytick.major.size": 3,
        "savefig.bbox": "tight",
        "savefig.pad_inches": 0.005,
    }
)

ZEONICA_COLOR = "#1D9E75"  # teal
TTSIM_COLOR = "#BA7517"  # amber
FIGSIZE_IN = (3.45, 2.55)
BAR_H = 0.36
ORDER = (
    "BFP8 matmul",
    "single-core matmul",
    "HiFi4 matmul",
    "3x3 matmul",
    "bcast W-mul",
    "eltwise mul",
    "SFPU exp-1",
    "SFPU sigmoid-1",
    "SFPU sigmoid-4",
    "SFPU tanh-4",
)


def read_rows(path):
    rows = {r["label"]: r for r in csv.DictReader(path.open())}
    missing = [k for k in ORDER if k not in rows]
    if missing:
        raise ValueError(f"missing kernels in {path}: {missing}")
    return [rows[k] for k in ORDER]


def kfmt(v, _pos):
    return f"{v / 1000:g}k" if v >= 1000 else f"{v:g}"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--input", type=Path, default=Path(__file__).resolve().parents[1] / "results" / "fig6_data.csv")
    ap.add_argument("--output-dir", type=Path, default=Path(__file__).resolve().parents[1] / "figures")
    ap.add_argument("--dpi", type=int, default=300)
    a = ap.parse_args()

    rows = read_rows(a.input)
    labels = [r["label"] for r in rows]
    z = np.array([float(r["zeonica_cycles"]) for r in rows])
    t = np.array([float(r["ttsim_cycles"]) for r in rows])
    ape = 100 * np.abs(z - t) / t
    mape = statistics.mean(ape)

    y = np.arange(len(rows))
    fig, ax = plt.subplots(figsize=FIGSIZE_IN)
    ax.barh(y - BAR_H / 2, z, height=BAR_H, color=ZEONICA_COLOR, edgecolor="none", label="Zeonica", zorder=3)
    ax.barh(y + BAR_H / 2, t, height=BAR_H, color=TTSIM_COLOR, edgecolor="none", label="TT-Sim", zorder=3)

    ax.set_xscale("log")
    lo, hi = 3.5e3, 1.6e6
    ax.set_xlim(lo, hi)
    ax.set_xticks([5e3, 1e4, 5e4, 2e5, 5e5])
    ax.xaxis.set_major_formatter(mticker.FuncFormatter(kfmt))
    ax.xaxis.set_minor_formatter(mticker.NullFormatter())
    ax.set_yticks(y)
    ax.set_yticklabels(labels)
    ax.set_ylim(len(rows) - 0.4, -0.75)  # top to bottom in paper order; headroom for the MAPE label
    ax.set_xlabel("Device-side cycles (log)", labelpad=2)
    ax.grid(True, which="major", axis="x", zorder=0)

    spine_color = "#888888"
    for side in ("bottom", "left", "top", "right"):
        ax.spines[side].set_visible(True)
        ax.spines[side].set_color(spine_color)
        ax.spines[side].set_linewidth(mpl.rcParams["axes.linewidth"])
    ax.tick_params(axis="both", which="major", colors="#555555")
    ax.tick_params(axis="x", which="minor", colors="#888888", length=1.5, width=0.4)
    ax.tick_params(axis="y", which="major", pad=1.5)
    ax.xaxis.label.set_color("#444444")

    for yi, zi, ti, e in zip(y, z, t, ape):
        ax.text(max(zi, ti) * 1.08, yi, f"{e:.2f}%", va="center", ha="left", fontsize=FONTSIZE - 1, color="#333333",
                zorder=4)
    ax.text(0.98, 0.985, f"MAPE = {mape:.2f}%", transform=ax.transAxes, ha="right", va="top",
            fontsize=FONTSIZE - 1, color="#333333", zorder=5)

    ax.legend(loc="lower center", bbox_to_anchor=(0.5, 1.0), ncol=2, frameon=False, handlelength=1.35,
              borderpad=0.3, handletextpad=0.4, columnspacing=1.0)

    a.output_dir.mkdir(parents=True, exist_ok=True)
    for ext in ("pdf", "png"):
        out = a.output_dir / f"fig6_tenstorrent.{ext}"
        fig.savefig(out, dpi=a.dpi if ext == "png" else None)
        print(f"Saved: {out}")
    plt.close(fig)


if __name__ == "__main__":
    main()
