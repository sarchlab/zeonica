#!/usr/bin/env python3
"""Plot the Zeonica/STONNE host-time ratio from data.csv.

The canvas, typography, and export settings intentionally match the standalone
simulation-cost panels in test/testbench/sim_cost_fixed128/results.
"""

from __future__ import annotations

import csv
from pathlib import Path

import matplotlib as mpl
import matplotlib.pyplot as plt
import numpy as np


HERE = Path(__file__).resolve().parent
DATA_CSV = HERE / "data.csv"
OUT_BASE = HERE / "zeonica_stonne_host_time_ratio"

SERIES_COLOR = "#BA7517"
GRID = "#D2D2D2"
TEXT = "#333333"


def configure_style() -> None:
    mpl.rcParams.update(
        {
            "font.family": "DejaVu Serif",
            "mathtext.fontset": "dejavuserif",
            "font.size": 9.0,
            "axes.labelsize": 9.0,
            "xtick.labelsize": 8.0,
            "ytick.labelsize": 8.0,
            "axes.linewidth": 0.55,
            "grid.linewidth": 0.4,
            "grid.alpha": 0.65,
            "grid.color": GRID,
            "grid.linestyle": ":",
            "lines.linewidth": 1.35,
            "lines.markersize": 3.2,
            "xtick.major.width": 0.5,
            "ytick.major.width": 0.5,
            "xtick.major.size": 2.5,
            "ytick.major.size": 2.5,
            "xtick.direction": "out",
            "ytick.direction": "out",
            "savefig.dpi": 300,
            "savefig.bbox": "tight",
            "savefig.pad_inches": 0.025,
        }
    )


def size_key(label: str) -> int:
    return int(label.lower().split("x", 1)[0])


def parse_float(value: str) -> float | None:
    text = value.strip()
    if not text or text.upper() == "N/A":
        return None
    return float(text)


def read_ratios() -> list[tuple[str, float]]:
    rows: list[tuple[str, float]] = []
    with DATA_CSV.open(newline="", encoding="utf-8") as stream:
        reader = csv.reader(stream)
        next(reader, None)
        for row in reader:
            if len(row) < 5:
                continue
            size = row[0].strip()
            zeonica_sec = parse_float(row[2])
            stonne_sec = parse_float(row[4])
            if not size or zeonica_sec is None or stonne_sec is None:
                continue
            if zeonica_sec <= 0 or stonne_sec <= 0:
                raise ValueError(f"host times must be positive for {size}")
            rows.append((size, zeonica_sec / stonne_sec))
    rows.sort(key=lambda item: size_key(item[0]))
    if not rows:
        raise ValueError(f"no comparable host-time rows found in {DATA_CSV}")
    return rows


def style_axis(ax: plt.Axes) -> None:
    ax.grid(True, which="major", axis="both")
    for spine in ax.spines.values():
        spine.set_visible(True)
        spine.set_linewidth(0.55)
        spine.set_color("#777777")
    ax.tick_params(colors="#555555", pad=1.8)


def draw_plot(rows: list[tuple[str, float]]) -> None:
    labels = [str(size_key(size)) for size, _ in rows]
    ratios = np.asarray([ratio for _, ratio in rows], dtype=float)
    x = np.arange(len(rows), dtype=float)

    fig, ax = plt.subplots(figsize=(2.8, 1.65), constrained_layout=True)
    ax.plot(x, ratios, marker="o", color=SERIES_COLOR)
    ax.fill_between(x, 1.0, ratios, color=SERIES_COLOR, alpha=0.12)
    ax.axhline(1.0, color="#777777", linestyle="--", linewidth=0.8)

    ax.set_xlabel("Array size (N)", labelpad=1.75)
    ax.set_ylabel("Host-time ratio", labelpad=-1.75)
    ax.set_xticks(x)
    ax.set_xticklabels(labels)
    ax.set_xlim(-0.65, len(rows) - 0.30)
    ax.set_yscale("log", base=2)
    ax.set_ylim(0.2, 45.0)
    ticks = np.asarray([0.25, 0.5, 1, 2, 4, 8, 16, 32], dtype=float)
    ax.set_yticks(ticks)
    tick_labels = [
        r"$\frac{1}{4}\times$",
        r"$\frac{1}{2}\times$",
        r"$1\times$",
        r"$2\times$",
        r"$4\times$",
        r"$8\times$",
        r"$16\times$",
        r"$32\times$",
    ]
    ax.set_yticklabels(tick_labels)

    for index in (0, 2, len(rows) - 1):
        y = ratios[index]
        if index == len(rows) - 1:
            offset = (-7, 0)
            horizontal_alignment = "right"
            vertical_alignment = "center"
        else:
            offset = (0, 5)
            horizontal_alignment = "center"
            vertical_alignment = "bottom"
        ax.annotate(
            rf"${y:.2f}\times$",
            xy=(x[index], y),
            xytext=offset,
            textcoords="offset points",
            ha=horizontal_alignment,
            va=vertical_alignment,
            fontsize=7.3,
            color=TEXT,
        )

    ax.text(
        0.985,
        0.345,
        r"Parity ($1\times$)",
        transform=ax.transAxes,
        ha="right",
        va="bottom",
        fontsize=7.3,
        color=TEXT,
    )
    style_axis(ax)

    for suffix in (".pdf", ".png"):
        fig.savefig(OUT_BASE.with_suffix(suffix))
    plt.close(fig)


def main() -> None:
    configure_style()
    draw_plot(read_ratios())


if __name__ == "__main__":
    main()
