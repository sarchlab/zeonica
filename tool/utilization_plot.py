#!/usr/bin/env python3
"""
CGRA utilization plotter for Zeonica JSON logs.

Outputs:
  - tile_utilization.png: per-tile activity heatmap (zeros shown as white)
  - port_utilization.png: per-port activity heatmaps (N/E/S/W, zeros shown as white)
"""

from __future__ import annotations

import argparse
import json
import os
import re
from collections import defaultdict
from typing import Dict, Tuple, Optional

import matplotlib.pyplot as plt
import numpy as np


PORTS = ("North", "East", "South", "West")


def parse_grid_from_main_go(main_go_path: str) -> Optional[Tuple[int, int]]:
    if not main_go_path or not os.path.exists(main_go_path):
        return None
    try:
        with open(main_go_path, "r", encoding="utf-8") as f:
            content = f.read()
    except OSError:
        return None

    width_match = re.search(r"\bwidth\s*:=\s*(\d+)", content)
    height_match = re.search(r"\bheight\s*:=\s*(\d+)", content)
    if not width_match or not height_match:
        return None
    width = int(width_match.group(1))
    height = int(height_match.group(1))
    if width <= 0 or height <= 0:
        return None
    return width, height


def infer_grid_from_log(log_path: str) -> Optional[Tuple[int, int]]:
    max_x = -1
    max_y = -1
    try:
        with open(log_path, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    entry = json.loads(line)
                except json.JSONDecodeError:
                    continue
                x = entry.get("X")
                y = entry.get("Y")
                if isinstance(x, int) and isinstance(y, int):
                    if x > max_x:
                        max_x = x
                    if y > max_y:
                        max_y = y
    except OSError:
        return None

    if max_x < 0 or max_y < 0:
        return None
    return max_x + 1, max_y + 1


def parse_tile_port(device_str: str) -> Optional[Tuple[int, int, str]]:
    # Example: Device.Tile[2][3].Core.West
    match = re.search(r"Device\.Tile\[(\d+)\]\[(\d+)\]\.Core\.(\w+)", device_str)
    if not match:
        return None
    x = int(match.group(1))
    y = int(match.group(2))
    port = match.group(3)
    return x, y, port


def parse_log(
    log_path: str,
) -> Tuple[Dict[Tuple[int, int], int], Dict[Tuple[int, int, str], int]]:
    tile_activity: Dict[Tuple[int, int], int] = defaultdict(int)
    port_activity: Dict[Tuple[int, int, str], int] = defaultdict(int)

    with open(log_path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                entry = json.loads(line)
            except json.JSONDecodeError:
                continue

            msg = entry.get("msg")
            if msg in ("Inst", "Memory"):
                x = entry.get("X")
                y = entry.get("Y")
                if isinstance(x, int) and isinstance(y, int):
                    tile_activity[(x, y)] += 1

            if msg == "DataFlow":
                behavior = entry.get("Behavior")
                if behavior in ("Send", "Recv"):
                    if behavior == "Send":
                        endpoint = entry.get("Src", "")
                    else:
                        endpoint = entry.get("Dst", "")
                    parsed = parse_tile_port(endpoint)
                    if parsed:
                        x, y, port = parsed
                        port_activity[(x, y, port)] += 1

    return tile_activity, port_activity


def build_tile_matrix(
    tile_activity: Dict[Tuple[int, int], int],
    width: int,
    height: int,
) -> np.ndarray:
    matrix = np.zeros((height, width), dtype=int)
    for (x, y), count in tile_activity.items():
        if 0 <= x < width and 0 <= y < height:
            matrix[y, x] = count
    return matrix


def build_port_matrices(
    port_activity: Dict[Tuple[int, int, str], int],
    width: int,
    height: int,
) -> Dict[str, np.ndarray]:
    matrices = {p: np.zeros((height, width), dtype=int) for p in PORTS}
    for (x, y, port), count in port_activity.items():
        if port not in matrices:
            continue
        if 0 <= x < width and 0 <= y < height:
            matrices[port][y, x] = count
    return matrices


def plot_tile_utilization(matrix: np.ndarray, output_path: str) -> None:
    height, width = matrix.shape
    plt.figure(figsize=(6, 6))
    masked = np.ma.masked_where(matrix == 0, matrix)
    cmap = plt.get_cmap("Reds").copy()
    cmap.set_bad(color="white")
    plt.imshow(
        masked,
        cmap=cmap,
        origin="lower",
        extent=[-0.5, width - 0.5, -0.5, height - 0.5],
        interpolation="nearest",
    )
    plt.colorbar(label="Activity Count")
    plt.title("Tile Utilization (Inst + Memory)")
    plt.xlabel("X")
    plt.ylabel("Y")
    plt.xticks(range(width))
    plt.yticks(range(height))
    plt.gca().set_xticks([i - 0.5 for i in range(width + 1)], minor=True)
    plt.gca().set_yticks([i - 0.5 for i in range(height + 1)], minor=True)
    plt.grid(which="minor", color="lightgray", linestyle="-", linewidth=0.7)
    plt.gca().tick_params(which="minor", bottom=False, left=False)

    # annotate tiles with utilization
    for y in range(height):
        for x in range(width):
            if matrix[y, x] > 0:
                plt.text(x, y, f"({x},{y})", ha="center", va="center", fontsize=8, color="black")

    plt.tight_layout()
    plt.savefig(output_path, dpi=200)
    plt.close()


def plot_port_utilization(matrices: Dict[str, np.ndarray], output_path: str) -> None:
    fig, axes = plt.subplots(2, 2, figsize=(10, 10))
    axes = axes.flatten()

    vmax = max((m.max() for m in matrices.values()), default=0)
    cmap = plt.get_cmap("Reds").copy()
    cmap.set_bad(color="white")

    for ax, port in zip(axes, PORTS):
        mat = matrices[port]
        masked = np.ma.masked_where(mat == 0, mat)
        height, width = mat.shape
        im = ax.imshow(
            masked,
            cmap=cmap,
            origin="lower",
            vmin=0,
            vmax=vmax or None,
            extent=[-0.5, width - 0.5, -0.5, height - 0.5],
            interpolation="nearest",
        )
        ax.set_title(f"Port {port}")
        ax.set_xlabel("X")
        ax.set_ylabel("Y")
        ax.set_xticks(range(width))
        ax.set_yticks(range(height))
        ax.set_xticks([i - 0.5 for i in range(width + 1)], minor=True)
        ax.set_yticks([i - 0.5 for i in range(height + 1)], minor=True)
        ax.grid(which="minor", color="lightgray", linestyle="-", linewidth=0.7)
        ax.tick_params(which="minor", bottom=False, left=False)
        fig.colorbar(im, ax=ax, fraction=0.046, pad=0.04)

        # annotate tiles with utilization
        for y in range(height):
            for x in range(width):
                if mat[y, x] > 0:
                    ax.text(x, y, f"({x},{y})", ha="center", va="center", fontsize=7, color="black")

    plt.tight_layout()
    plt.savefig(output_path, dpi=200)
    plt.close()


def main() -> None:
    parser = argparse.ArgumentParser(description="Plot CGRA tile/port utilization from JSON logs.")
    parser.add_argument("log_file", help="Path to JSON log file (e.g. histogram.json.log)")
    parser.add_argument("--main-go", default="", help="Path to main.go to infer width/height")
    parser.add_argument("--width", type=int, default=0, help="Override grid width")
    parser.add_argument("--height", type=int, default=0, help="Override grid height")
    parser.add_argument("--out-dir", default="tool/output", help="Output directory")
    args = parser.parse_args()

    width = args.width
    height = args.height
    if width <= 0 or height <= 0:
        parsed = parse_grid_from_main_go(args.main_go)
        if parsed:
            width, height = parsed
    if width <= 0 or height <= 0:
        inferred = infer_grid_from_log(args.log_file)
        if inferred:
            width, height = inferred

    if width <= 0 or height <= 0:
        raise SystemExit("Cannot determine grid size, please provide --width/--height or --main-go.")

    tile_activity, port_activity = parse_log(args.log_file)
    tile_matrix = build_tile_matrix(tile_activity, width, height)
    port_matrices = build_port_matrices(port_activity, width, height)

    os.makedirs(args.out_dir, exist_ok=True)
    plot_tile_utilization(tile_matrix, os.path.join(args.out_dir, "tile_utilization.png"))
    plot_port_utilization(port_matrices, os.path.join(args.out_dir, "port_utilization.png"))

    print(f"Completed: {args.out_dir}/tile_utilization.png")
    print(f"Completed: {args.out_dir}/port_utilization.png")


if __name__ == "__main__":
    main()
