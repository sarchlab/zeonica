#!/usr/bin/env python3
"""Single-column HPCA-ready Zeonica vs Stonne cycle comparison."""

from __future__ import annotations

import csv
from pathlib import Path

import matplotlib as mpl
import matplotlib.pyplot as plt
import numpy as np


HERE = Path(__file__).resolve().parent
DATA_CSV = HERE / "data.csv"
OUT_BASE = HERE / "comparison_plot_hpca"

ZEONICA_COLOR = "#1D9E75"
STONNE_COLOR = "#BA7517"
GRID = "#D2D2D2"


def configure_style() -> None:
    mpl.rcParams.update(
        {
            "font.family": "DejaVu Serif",
            "mathtext.fontset": "dejavuserif",
            "font.size": 9.0,
            "axes.labelsize": 9.0,
            "xtick.labelsize": 8.0,
            "ytick.labelsize": 8.0,
            "legend.fontsize": 8.5,
            "axes.linewidth": 0.55,
            "grid.linewidth": 0.4,
            "grid.alpha": 0.65,
            "grid.color": GRID,
            "grid.linestyle": ":",
            "xtick.major.width": 0.5,
            "ytick.major.width": 0.5,
            "xtick.major.size": 2.5,
            "ytick.major.size": 2.5,
            "xtick.direction": "out",
            "ytick.direction": "out",
            "savefig.dpi": 300,
            "savefig.pad_inches": 0.025,
        }
    )


def parse_num(text: str) -> float | None:
    text = (text or "").strip()
    if not text or text.upper() == "N/A":
        return None
    return float(text)


def size_to_n(size: str) -> int:
    left = size.strip().lower().split("x", 1)[0]
    return int(left) if left.isdigit() else 10**9


def read_rows() -> list[tuple[str, float, float]]:
    rows: list[tuple[str, float, float]] = []
    with DATA_CSV.open(newline="", encoding="utf-8") as f:
        reader = csv.reader(f)
        _ = next(reader, None)
        for row in reader:
            if len(row) < 4:
                continue
            size = row[0].strip()
            zeonica_cycle = parse_num(row[1])
            stonne_cycle = parse_num(row[3])
            if not size or zeonica_cycle is None or stonne_cycle is None:
                continue
            rows.append((size, zeonica_cycle, stonne_cycle))
    rows.sort(key=lambda r: size_to_n(r[0]))
    return rows


def style_axis(ax: plt.Axes) -> None:
    ax.grid(True, which="major", axis="x")
    for spine in ax.spines.values():
        spine.set_visible(True)
        spine.set_linewidth(0.55)
        spine.set_color("#777777")
    ax.tick_params(colors="#555555", pad=1.5)


def draw_plot(rows: list[tuple[str, float, float]], out_base: Path) -> None:
    n_labels = [str(size_to_n(size)) for size, _, _ in rows]
    zeonica = np.array([z for _, z, _ in rows], dtype=float)
    stonne = np.array([s for _, _, s in rows], dtype=float)
    rel_err = (zeonica - stonne) / stonne * 100.0
    avg_abs_err = float(np.mean(np.abs(rel_err)))

    y = np.arange(len(rows))
    height = 0.30
    fig, ax = plt.subplots(figsize=(3.35, 1.74))

    ax.barh(
        y - height / 2,
        zeonica,
        height=height,
        color=ZEONICA_COLOR,
        edgecolor="none",
        label="Zeonica",
        zorder=3,
    )
    ax.barh(
        y + height / 2,
        stonne,
        height=height,
        color=STONNE_COLOR,
        edgecolor="none",
        label="STONNE",
        zorder=3,
    )

    x_max = max(float(np.max(zeonica)), float(np.max(stonne)))
    for i, err in enumerate(rel_err):
        x_text = max(zeonica[i], stonne[i]) + x_max * 0.025
        sign = "+" if err >= 0 else ""
        ax.text(
            x_text,
            y[i],
            f"{sign}{err:.1f}%",
            ha="left",
            va="center",
            fontsize=8.0,
            color="#333333",
        )

    ax.set_xlim(0, x_max * 1.24)
    ax.set_yticks(y)
    ax.set_yticklabels(n_labels)
    ax.set_xlabel("Cycles", labelpad=1.5)
    ax.set_ylabel(r"Matrix size $N$", labelpad=2)
    style_axis(ax)

    ax.legend(
        loc="lower center",
        bbox_to_anchor=(0.5, 1.005),
        ncol=2,
        frameon=False,
        handlelength=1.2,
        handletextpad=0.28,
        columnspacing=0.8,
        borderpad=0.0,
        labelspacing=0.2,
    )

    ax.text(
        0.985,
        0.055,
        f"MAPE = {avg_abs_err:.2f}%",
        transform=ax.transAxes,
        ha="right",
        va="bottom",
        fontsize=8.0,
        color="#333333",
    )

    fig.subplots_adjust(left=0.19, right=0.965, bottom=0.25, top=0.83)
    for suffix in (".pdf", ".png"):
        fig.savefig(out_base.with_suffix(suffix), bbox_inches="tight")
    plt.close(fig)


def main() -> None:
    configure_style()
    rows = read_rows()
    draw_plot(rows, OUT_BASE)


if __name__ == "__main__":
    main()
