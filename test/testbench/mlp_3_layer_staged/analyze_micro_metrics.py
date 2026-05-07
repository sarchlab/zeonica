#!/usr/bin/env python3
from __future__ import annotations

import argparse
import csv
import json
import math
import re
from collections import defaultdict
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional, Tuple


VARIANTS = ("fused", "b1", "staged")
BOUNDARY_COUNT = {"fused": 0, "b1": 1, "staged": 2}
BOUNDARY_DIMS_BY_VARIANT = {
    "fused": [],
    "b1": ["D2"],
    "staged": ["D1", "D2"],
}


@dataclass
class RunPaths:
    rounds: int
    reports: Dict[str, Path]
    stage_reports: Dict[str, List[Path]]
    logs: Dict[str, List[Path]]
    stage_cycles: Dict[str, List[int]]


@dataclass
class VariantStats:
    rounds: int
    variant: str
    cycles: int
    active_cycles: int
    idle_cycles: int
    inst_count: int
    total_events: int
    operand_wait_stall: int
    schedule_bubble_stall: int
    output_blocked_stall: int
    backpressure_count: int
    backpressure_cycles: int
    avg_tile_util_pct: float
    peak_tile_util_pct: float
    inst_tp_cycle: float
    event_tp_cycle: float
    inst_tp_sec: float


def _to_int(v: Any, default: int = 0) -> int:
    if v is None or v == "":
        return default
    if isinstance(v, bool):
        return int(v)
    if isinstance(v, int):
        return v
    if isinstance(v, float):
        return int(v)
    try:
        return int(str(v).strip())
    except (TypeError, ValueError):
        return default


def _to_float(v: Any, default: float = 0.0) -> float:
    if v is None or v == "":
        return default
    if isinstance(v, (int, float)):
        return float(v)
    try:
        return float(str(v).strip())
    except (TypeError, ValueError):
        return default


def _safe_div(a: float, b: float) -> float:
    if b == 0:
        return 0.0
    return float(a) / float(b)


def _norm_key(name: str) -> str:
    return re.sub(r"[^a-z0-9]", "", name.lower())


def load_json(path: Path) -> Dict[str, Any]:
    with path.open("r", encoding="utf-8") as f:
        return json.load(f)


def read_summary(summary_csv: Path) -> List[Dict[str, str]]:
    with summary_csv.open("r", encoding="utf-8", newline="") as f:
        return list(csv.DictReader(f))


def extract_run_paths(base_dir: Path, rows: List[Dict[str, str]]) -> List[RunPaths]:
    runs: List[RunPaths] = []
    for row in rows:
        rounds = _to_int(row.get("rounds"))
        if rounds <= 0:
            continue

        def p(col: str) -> Path:
            name = row.get(col, "")
            if not name:
                return Path("")
            return base_dir / name

        fused_report = p("fused_report")
        b1_report = p("b1_report")
        staged_report = p("staged_report")

        stage_reports = {
            "fused": [fused_report],
            "b1": [p("b1_stage12_report"), p("b1_stage3_report")],
            "staged": [p("staged_stage1_report"), p("staged_stage2_report"), p("staged_stage3_report")],
        }
        reports = {
            "fused": fused_report,
            "b1": b1_report,
            "staged": staged_report,
        }

        logs = {
            "fused": [p("fused_log")],
            "b1": [p("b1_stage12_log"), p("b1_stage3_log")],
            "staged": [p("staged_stage1_log"), p("staged_stage2_log"), p("staged_stage3_log")],
        }

        stage_cycles = {
            "fused": [_to_int(row.get("fused_cycles"))],
            "b1": [_to_int(row.get("b1_stage12_cycles")), _to_int(row.get("b1_stage3_cycles"))],
            "staged": [_to_int(row.get("staged_stage1_cycles")), _to_int(row.get("staged_stage2_cycles")), _to_int(row.get("staged_stage3_cycles"))],
        }

        runs.append(
            RunPaths(
                rounds=rounds,
                reports=reports,
                stage_reports=stage_reports,
                logs=logs,
                stage_cycles=stage_cycles,
            )
        )

    runs.sort(key=lambda r: r.rounds)
    return runs


def aggregate_variant_stats_from_reports(rounds: int, variant: str, stage_report_objs: List[Dict[str, Any]]) -> VariantStats:
    if variant == "fused":
        fused = stage_report_objs[0]
        cycles = _to_int(fused.get("totalCycles"))
        tiles = fused.get("tiles", [])
        tile_active_sum = sum(_to_int(t.get("activeCycles")) for t in tiles)
        tile_count = len(tiles)
        avg_util = 100.0 * _safe_div(tile_active_sum, cycles * tile_count) if tile_count > 0 else 0.0
        peak_util = max((_to_float(t.get("utilizationPct")) for t in tiles), default=0.0)
        wall = _to_float(fused.get("wallClockDurationSec"))
        inst = _to_int(fused.get("instCount"))
        events = _to_int(fused.get("totalEvents"))
        return VariantStats(
            rounds=rounds,
            variant=variant,
            cycles=cycles,
            active_cycles=_to_int(fused.get("activeCyclesGlobal")),
            idle_cycles=_to_int(fused.get("idleCyclesGlobal")),
            inst_count=inst,
            total_events=events,
            operand_wait_stall=_to_int(fused.get("operandWaitStallCount")),
            schedule_bubble_stall=_to_int(fused.get("scheduleBubbleStallCount")),
            output_blocked_stall=_to_int(fused.get("outputBlockedStallCount")),
            backpressure_count=_to_int(fused.get("backpressureCount")),
            backpressure_cycles=_to_int(fused.get("backpressureCycles")),
            avg_tile_util_pct=avg_util,
            peak_tile_util_pct=peak_util,
            inst_tp_cycle=_to_float(fused.get("instThroughputPerCycle"), _safe_div(inst, cycles)),
            event_tp_cycle=_to_float(fused.get("eventThroughputPerCycle"), _safe_div(events, cycles)),
            inst_tp_sec=_to_float(fused.get("instThroughputPerSec"), _safe_div(inst, wall)),
        )

    cycles = sum(_to_int(r.get("totalCycles")) for r in stage_report_objs)
    active = sum(_to_int(r.get("activeCyclesGlobal")) for r in stage_report_objs)
    idle = sum(_to_int(r.get("idleCyclesGlobal")) for r in stage_report_objs)
    inst = sum(_to_int(r.get("instCount")) for r in stage_report_objs)
    events = sum(_to_int(r.get("totalEvents")) for r in stage_report_objs)
    operand = sum(_to_int(r.get("operandWaitStallCount")) for r in stage_report_objs)
    sched = sum(_to_int(r.get("scheduleBubbleStallCount")) for r in stage_report_objs)
    output = sum(_to_int(r.get("outputBlockedStallCount")) for r in stage_report_objs)
    bp_count = sum(_to_int(r.get("backpressureCount")) for r in stage_report_objs)
    bp_cycles = sum(_to_int(r.get("backpressureCycles")) for r in stage_report_objs)
    wall = sum(_to_float(r.get("wallClockDurationSec")) for r in stage_report_objs)

    tile_count = len(stage_report_objs[0].get("tiles", [])) if stage_report_objs else 0
    tile_active: Dict[str, int] = defaultdict(int)
    for report in stage_report_objs:
        for tile in report.get("tiles", []):
            tile_active[str(tile.get("coord", ""))] += _to_int(tile.get("activeCycles"))
    avg_util = 100.0 * _safe_div(sum(tile_active.values()), cycles * tile_count) if tile_count > 0 else 0.0
    peak_util = max((100.0 * _safe_div(v, cycles) for v in tile_active.values()), default=0.0)

    return VariantStats(
        rounds=rounds,
        variant=variant,
        cycles=cycles,
        active_cycles=active,
        idle_cycles=idle,
        inst_count=inst,
        total_events=events,
        operand_wait_stall=operand,
        schedule_bubble_stall=sched,
        output_blocked_stall=output,
        backpressure_count=bp_count,
        backpressure_cycles=bp_cycles,
        avg_tile_util_pct=avg_util,
        peak_tile_util_pct=peak_util,
        inst_tp_cycle=_safe_div(inst, cycles),
        event_tp_cycle=_safe_div(events, cycles),
        inst_tp_sec=_safe_div(inst, wall),
    )


def parse_tile_operand_wait(stage_report_objs: List[Dict[str, Any]]) -> Dict[Tuple[int, int], int]:
    out: Dict[Tuple[int, int], int] = defaultdict(int)
    for report in stage_report_objs:
        for tile in report.get("tiles", []):
            x = _to_int(tile.get("x"))
            y = _to_int(tile.get("y"))
            out[(x, y)] += _to_int(tile.get("operandWaitStallCount"))
    return dict(out)


def parse_operand_wait_trace(log_path: Path, cycle_offset: int = 0) -> Tuple[Dict[int, int], Dict[Tuple[int, int], int], int]:
    by_cycle: Dict[int, int] = defaultdict(int)
    by_cycle_col: Dict[Tuple[int, int], int] = defaultdict(int)
    total = 0
    with log_path.open("r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            if obj.get("msg") != "Stall":
                continue
            if obj.get("Behavior") != "operand_wait":
                continue
            t = _to_int(obj.get("Time"), -1)
            x = _to_int(obj.get("X"), -1)
            if t < 0 or x < 0:
                continue
            c = cycle_offset + t
            by_cycle[c] += 1
            by_cycle_col[(c, x)] += 1
            total += 1
    return dict(by_cycle), dict(by_cycle_col), total


def load_buffer_rows(path: Path) -> List[Dict[str, Any]]:
    if path.suffix.lower() == ".csv":
        with path.open("r", encoding="utf-8", newline="") as f:
            return list(csv.DictReader(f))

    with path.open("r", encoding="utf-8") as f:
        data = json.load(f)
    if isinstance(data, list):
        return [x for x in data if isinstance(x, dict)]
    if isinstance(data, dict):
        if "rows" in data and isinstance(data["rows"], list):
            return [x for x in data["rows"] if isinstance(x, dict)]
        return [data]
    return []


def pick_value(row: Dict[str, Any], candidates: Iterable[str]) -> Optional[Any]:
    norm_map = {_norm_key(k): v for k, v in row.items()}
    for name in candidates:
        n = _norm_key(name)
        if n in norm_map:
            return norm_map[n]
    return None


def aggregate_buffer_stats(rows: List[Dict[str, Any]]) -> Dict[Tuple[int, str], Dict[str, float]]:
    out: Dict[Tuple[int, str], Dict[str, float]] = defaultdict(lambda: {
        "write_bytes": 0.0,
        "read_bytes": 0.0,
        "write_access": 0.0,
        "read_access": 0.0,
    })

    for row in rows:
        rounds_raw = pick_value(row, ["rounds", "round", "r", "input_length"])
        mode_raw = pick_value(row, ["mode", "variant", "case", "config", "scheme", "run"])
        rounds = _to_int(rounds_raw, -1)
        if rounds < 0:
            continue
        if mode_raw is None:
            continue
        mode = str(mode_raw).strip().lower()
        if mode in {"boundary1", "1boundary", "one_boundary", "bnd1", "boundary_1"}:
            mode = "b1"
        if mode not in {"fused", "staged", "b1"}:
            continue

        write_bytes = _to_float(pick_value(row, [
            "buf_write_bytes_total",
            "buffer_write_bytes_total",
            "write_bytes_total",
            "write_bytes",
            "onchip_write_bytes",
            "buffer_write_bytes",
        ]))
        read_bytes = _to_float(pick_value(row, [
            "buf_read_bytes_total",
            "buffer_read_bytes_total",
            "read_bytes_total",
            "read_bytes",
            "onchip_read_bytes",
            "buffer_read_bytes",
        ]))
        write_access = _to_float(pick_value(row, [
            "buf_write_access_total",
            "buffer_write_access_total",
            "write_access_total",
            "write_accesses",
            "write_access",
        ]))
        read_access = _to_float(pick_value(row, [
            "buf_read_access_total",
            "buffer_read_access_total",
            "read_access_total",
            "read_accesses",
            "read_access",
        ]))

        key = (rounds, mode)
        out[key]["write_bytes"] += write_bytes
        out[key]["read_bytes"] += read_bytes
        out[key]["write_access"] += write_access
        out[key]["read_access"] += read_access

    return dict(out)


def write_csv(path: Path, fieldnames: List[str], rows: List[Dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        for row in rows:
            writer.writerow(row)


def linear_fit_r2(xs: List[float], ys: List[float]) -> Tuple[float, float, float]:
    if len(xs) < 2 or len(xs) != len(ys):
        return 0.0, 0.0, 0.0
    x_mean = sum(xs) / len(xs)
    y_mean = sum(ys) / len(ys)
    sxx = sum((x - x_mean) ** 2 for x in xs)
    sxy = sum((x - x_mean) * (y - y_mean) for x, y in zip(xs, ys))
    if sxx == 0:
        return 0.0, y_mean, 0.0
    slope = sxy / sxx
    intercept = y_mean - slope * x_mean
    ss_tot = sum((y - y_mean) ** 2 for y in ys)
    ss_res = sum((y - (slope * x + intercept)) ** 2 for x, y in zip(xs, ys))
    r2 = 1.0 - _safe_div(ss_res, ss_tot) if ss_tot > 0 else 1.0
    return slope, intercept, r2


def main() -> None:
    parser = argparse.ArgumentParser(description="Analyze MICRO supplementary metrics for fused vs staged MLP runs.")
    parser.add_argument("--input-dir", required=True, help="Directory containing sweep summary.csv and report/log artifacts")
    parser.add_argument("--summary", default="summary.csv", help="Summary CSV filename under input-dir")
    parser.add_argument("--buffer-stats", default="", help="Optional buffer stats CSV/JSON path")
    parser.add_argument("--activation-bytes", type=int, default=4, help="Activation bytes (Ba)")
    parser.add_argument("--weight-bytes", type=int, default=4, help="Weight bytes (Bw)")
    parser.add_argument("--dims", default="16,16,16,16", help="MLP dims D0,D1,D2,D3")
    parser.add_argument("--prologue-cycles", type=int, default=32, help="Prologue window used for burst-fraction stall analysis")
    parser.add_argument("--stall-round", type=int, default=0, help="Round point used for detailed temporal/spatial fig data (0 means max rounds)")
    args = parser.parse_args()

    input_dir = Path(args.input_dir).resolve()
    summary_csv = input_dir / args.summary
    if not summary_csv.exists():
        raise FileNotFoundError(f"summary csv not found: {summary_csv}")

    dims_tokens = [x.strip() for x in args.dims.split(",") if x.strip()]
    if len(dims_tokens) != 4:
        raise ValueError("--dims must be D0,D1,D2,D3")
    d0, d1, d2, d3 = [int(x) for x in dims_tokens]

    rows = read_summary(summary_csv)
    runs = extract_run_paths(input_dir, rows)
    if not runs:
        raise RuntimeError("no runs parsed from summary")

    variant_stats_rows: List[Dict[str, Any]] = []
    all_stats: Dict[Tuple[int, str], VariantStats] = {}

    tile_operand_wait: Dict[Tuple[int, str], Dict[Tuple[int, int], int]] = {}

    for run in runs:
        for variant in VARIANTS:
            stage_paths = [p for p in run.stage_reports[variant] if str(p) and p.exists()]
            if not stage_paths:
                continue
            stage_objs = [load_json(p) for p in stage_paths]
            stats = aggregate_variant_stats_from_reports(run.rounds, variant, stage_objs)
            all_stats[(run.rounds, variant)] = stats

            tile_operand_wait[(run.rounds, variant)] = parse_tile_operand_wait(stage_objs)

            variant_stats_rows.append(
                {
                    "rounds": run.rounds,
                    "variant": variant,
                    "boundary_count": BOUNDARY_COUNT[variant],
                    "cycles": stats.cycles,
                    "active_cycles": stats.active_cycles,
                    "idle_cycles": stats.idle_cycles,
                    "inst_count": stats.inst_count,
                    "total_events": stats.total_events,
                    "inst_tp_cycle": f"{stats.inst_tp_cycle:.6f}",
                    "event_tp_cycle": f"{stats.event_tp_cycle:.6f}",
                    "inst_tp_sec": f"{stats.inst_tp_sec:.6f}",
                    "operand_wait_stall": stats.operand_wait_stall,
                    "schedule_bubble_stall": stats.schedule_bubble_stall,
                    "output_blocked_stall": stats.output_blocked_stall,
                    "backpressure_count": stats.backpressure_count,
                    "backpressure_cycles": stats.backpressure_cycles,
                    "avg_tile_util_pct": f"{stats.avg_tile_util_pct:.6f}",
                    "peak_tile_util_pct": f"{stats.peak_tile_util_pct:.6f}",
                }
            )

    buffer_data_available = False
    buf_stats: Dict[Tuple[int, str], Dict[str, float]] = {}
    if args.buffer_stats:
        buffer_path = Path(args.buffer_stats).resolve()
        if not buffer_path.exists():
            raise FileNotFoundError(f"buffer stats file not found: {buffer_path}")
        buf_rows = load_buffer_rows(buffer_path)
        buf_stats = aggregate_buffer_stats(buf_rows)
        buffer_data_available = len(buf_stats) > 0

    summary_micro_rows: List[Dict[str, Any]] = []
    memory_fig_rows: List[Dict[str, Any]] = []
    traffic_vs_round_rows: List[Dict[str, Any]] = []
    ratio_rows: List[Dict[str, Any]] = []
    linear_fit_rows: List[Dict[str, Any]] = []

    for run in runs:
        rounds = run.rounds
        fused = all_stats.get((rounds, "fused"))
        if fused is None:
            continue

        fused_buf = buf_stats.get((rounds, "fused"), {"write_bytes": 0.0, "read_bytes": 0.0, "write_access": 0.0, "read_access": 0.0})
        fused_rw = fused_buf["write_bytes"] + fused_buf["read_bytes"]

        for variant in VARIANTS:
            stats = all_stats.get((rounds, variant))
            if stats is None:
                continue

            vb = buf_stats.get((rounds, variant), {"write_bytes": 0.0, "read_bytes": 0.0, "write_access": 0.0, "read_access": 0.0})
            total_rw = vb["write_bytes"] + vb["read_bytes"]

            boundary_write = max(0.0, vb["write_bytes"] - fused_buf["write_bytes"])
            boundary_read = max(0.0, vb["read_bytes"] - fused_buf["read_bytes"])
            w_meas = boundary_write
            t_meas = max(0.0, total_rw - fused_rw)
            decomp_error = abs(total_rw - (fused_rw + boundary_write + boundary_read))

            dims_map = {"D0": d0, "D1": d1, "D2": d2, "D3": d3}
            boundary_dims = [dims_map[d] for d in BOUNDARY_DIMS_BY_VARIANT[variant]]
            boundary_elems = rounds * sum(boundary_dims)

            w_min = float(rounds * sum(boundary_dims) * args.activation_bytes)
            t_min = 2.0 * w_min
            eta_w = _safe_div(w_min, w_meas) if w_meas > 0 else 0.0
            oh_wb = _safe_div(t_meas, fused_rw) if fused_rw > 0 else 0.0

            output_bytes = rounds * d3 * args.activation_bytes
            traffic_per_output_byte = _safe_div(total_rw, output_bytes) if output_bytes > 0 else 0.0
            write_read_asym = _safe_div(vb["write_bytes"], vb["read_bytes"]) if vb["read_bytes"] > 0 else 0.0

            write_access_delta = max(0.0, vb["write_access"] - fused_buf["write_access"])
            read_access_delta = max(0.0, vb["read_access"] - fused_buf["read_access"])
            write_access_per_boundary_elem = _safe_div(write_access_delta, boundary_elems) if boundary_elems > 0 else 0.0
            read_access_per_boundary_elem = _safe_div(read_access_delta, boundary_elems) if boundary_elems > 0 else 0.0

            dram_lb_fused = rounds * d0 * args.activation_bytes + (d0 * d1 + d1 * d2 + d2 * d3) * args.weight_bytes + rounds * d3 * args.activation_bytes
            dram_extra_boundary_spill = 2 * rounds * sum(boundary_dims) * args.activation_bytes
            dram_lb_optimistic = dram_lb_fused
            dram_lb_boundary_spill = dram_lb_fused + dram_extra_boundary_spill

            summary_micro_rows.append(
                {
                    "rounds": rounds,
                    "variant": variant,
                    "boundary_count": BOUNDARY_COUNT[variant],
                    "cycles": stats.cycles,
                    "operand_wait_stall": stats.operand_wait_stall,
                    "schedule_bubble_stall": stats.schedule_bubble_stall,
                    "output_blocked_stall": stats.output_blocked_stall,
                    "avg_tile_util_pct": f"{stats.avg_tile_util_pct:.6f}",
                    "inst_tp_cycle": f"{stats.inst_tp_cycle:.6f}",
                    "event_tp_cycle": f"{stats.event_tp_cycle:.6f}",
                    "buf_write_bytes_total": f"{vb['write_bytes']:.3f}",
                    "buf_read_bytes_total": f"{vb['read_bytes']:.3f}",
                    "buf_write_access_total": f"{vb['write_access']:.3f}",
                    "buf_read_access_total": f"{vb['read_access']:.3f}",
                    "boundary_write_bytes": f"{boundary_write:.3f}",
                    "boundary_read_bytes": f"{boundary_read:.3f}",
                    "w_meas": f"{w_meas:.3f}",
                    "t_meas": f"{t_meas:.3f}",
                    "w_min": f"{w_min:.3f}",
                    "t_min": f"{t_min:.3f}",
                    "eta_w": f"{eta_w:.6f}",
                    "oh_wb": f"{oh_wb:.6f}",
                    "traffic_per_output_byte": f"{traffic_per_output_byte:.6f}",
                    "write_read_asym": f"{write_read_asym:.6f}",
                    "write_access_per_boundary_elem": f"{write_access_per_boundary_elem:.6f}",
                    "read_access_per_boundary_elem": f"{read_access_per_boundary_elem:.6f}",
                    "dram_lb_optimistic_bytes": f"{dram_lb_optimistic:.3f}",
                    "dram_lb_boundary_spill_bytes": f"{dram_lb_boundary_spill:.3f}",
                    "decomp_error_bytes": f"{decomp_error:.6f}",
                    "buffer_stats_available": int(buffer_data_available),
                }
            )

            ratio_vs_fused = _safe_div(total_rw, fused_rw) if fused_rw > 0 else 0.0
            memory_fig_rows.append(
                {
                    "rounds": rounds,
                    "variant": variant,
                    "baseline_bytes": f"{fused_rw:.3f}",
                    "boundary_write_bytes": f"{boundary_write:.3f}",
                    "boundary_read_bytes": f"{boundary_read:.3f}",
                    "total_bytes": f"{total_rw:.3f}",
                    "ratio_vs_fused": f"{ratio_vs_fused:.6f}",
                }
            )
            traffic_vs_round_rows.append(
                {
                    "rounds": rounds,
                    "variant": variant,
                    "total_bytes": f"{total_rw:.3f}",
                    "t_meas": f"{t_meas:.3f}",
                }
            )
            if variant != "fused":
                ratio_rows.append(
                    {
                        "rounds": rounds,
                        "variant": variant,
                        "traffic_overhead_ratio": f"{ratio_vs_fused:.6f}",
                        "cycle_overhead_ratio": f"{_safe_div(stats.cycles, fused.cycles):.6f}",
                    }
                )

    if buffer_data_available:
        for variant in ("b1", "staged"):
            pts = [
                row
                for row in summary_micro_rows
                if row["variant"] == variant
            ]
            if not pts:
                continue
            xs = [float(_to_int(row["rounds"])) for row in pts]
            ys = [float(row["t_meas"]) for row in pts]
            slope, intercept, r2 = linear_fit_r2(xs, ys)
            linear_fit_rows.append(
                {
                    "variant": variant,
                    "fit_target": "t_meas_vs_rounds",
                    "slope": f"{slope:.6f}",
                    "intercept": f"{intercept:.6f}",
                    "r2": f"{r2:.6f}",
                }
            )

    # Spatial stall deltas per selected round.
    target_round = args.stall_round if args.stall_round > 0 else max(r.rounds for r in runs)
    if target_round not in {r.rounds for r in runs}:
        target_round = max(r.rounds for r in runs)

    fused_tile = tile_operand_wait.get((target_round, "fused"), {})
    spatial_rows: List[Dict[str, Any]] = []
    spatial_col_rows: List[Dict[str, Any]] = []
    for variant in ("b1", "staged"):
        cur = tile_operand_wait.get((target_round, variant), {})
        col_fused = defaultdict(int)
        col_variant = defaultdict(int)
        for x in range(16):
            for y in range(16):
                fv = fused_tile.get((x, y), 0)
                vv = cur.get((x, y), 0)
                spatial_rows.append(
                    {
                        "rounds": target_round,
                        "variant": variant,
                        "x": x,
                        "y": y,
                        "fused_operand_wait": fv,
                        "variant_operand_wait": vv,
                        "delta_operand_wait": vv - fv,
                    }
                )
                col_fused[x] += fv
                col_variant[x] += vv
        for x in range(16):
            spatial_col_rows.append(
                {
                    "rounds": target_round,
                    "variant": variant,
                    "column": x,
                    "fused_operand_wait": col_fused[x],
                    "variant_operand_wait": col_variant[x],
                    "delta_operand_wait": col_variant[x] - col_fused[x],
                }
            )

    # Temporal stall series + phase burst metrics.
    temporal_rows: List[Dict[str, Any]] = []
    phase_rows: List[Dict[str, Any]] = []
    staged_space_time_rows: List[Dict[str, Any]] = []

    selected_run = None
    for r in runs:
        if r.rounds == target_round:
            selected_run = r
            break

    if selected_run is not None:
        for variant in VARIANTS:
            logs = [p for p in selected_run.logs[variant] if str(p) and p.exists()]
            cycles = selected_run.stage_cycles[variant]
            cycle_offset = 0
            merged_by_cycle: Dict[int, int] = defaultdict(int)
            merged_by_cycle_col: Dict[Tuple[int, int], int] = defaultdict(int)

            for idx, log in enumerate(logs):
                c = cycles[idx] if idx < len(cycles) else 0
                by_cycle, by_cycle_col, _ = parse_operand_wait_trace(log, cycle_offset=cycle_offset)
                for k, v in by_cycle.items():
                    merged_by_cycle[k] += v
                for k, v in by_cycle_col.items():
                    merged_by_cycle_col[k] += v

                phase_start = cycle_offset
                phase_end = cycle_offset + c
                prologue_end = min(phase_end, phase_start + args.prologue_cycles)
                total_phase_stall = sum(v for cyc, v in by_cycle.items() if phase_start <= cyc < phase_end)
                prologue_stall = sum(v for cyc, v in by_cycle.items() if phase_start <= cyc < prologue_end)
                steady_cycles = max(0, phase_end - prologue_end)
                steady_stall = max(0, total_phase_stall - prologue_stall)
                burst_peak = max((v for cyc, v in by_cycle.items() if phase_start <= cyc < prologue_end), default=0)
                phase_rows.append(
                    {
                        "rounds": target_round,
                        "variant": variant,
                        "phase_idx": idx,
                        "phase_start_cycle": phase_start,
                        "phase_end_cycle": phase_end,
                        "phase_cycles": c,
                        "total_stall": total_phase_stall,
                        "prologue_cycles": prologue_end - phase_start,
                        "prologue_stall": prologue_stall,
                        "steady_cycles": steady_cycles,
                        "steady_stall": steady_stall,
                        "burst_fraction": f"{_safe_div(prologue_stall, total_phase_stall):.6f}" if total_phase_stall > 0 else "0.000000",
                        "burst_peak": burst_peak,
                        "steady_mean": f"{_safe_div(steady_stall, steady_cycles):.6f}" if steady_cycles > 0 else "0.000000",
                    }
                )

                cycle_offset = phase_end

            max_cycle = max(merged_by_cycle.keys(), default=-1)
            for cyc in range(max_cycle + 1):
                temporal_rows.append(
                    {
                        "rounds": target_round,
                        "variant": variant,
                        "cycle": cyc,
                        "operand_wait_events": merged_by_cycle.get(cyc, 0),
                    }
                )

            if variant == "staged":
                for (cyc, col), v in sorted(merged_by_cycle_col.items()):
                    staged_space_time_rows.append(
                        {
                            "rounds": target_round,
                            "cycle": cyc,
                            "column": col,
                            "operand_wait_events": v,
                        }
                    )

    summary_micro_path = input_dir / "summary_micro.csv"
    fig_data_dir = input_dir / "fig_data"

    write_csv(
        summary_micro_path,
        [
            "rounds",
            "variant",
            "boundary_count",
            "cycles",
            "operand_wait_stall",
            "schedule_bubble_stall",
            "output_blocked_stall",
            "avg_tile_util_pct",
            "inst_tp_cycle",
            "event_tp_cycle",
            "buf_write_bytes_total",
            "buf_read_bytes_total",
            "buf_write_access_total",
            "buf_read_access_total",
            "boundary_write_bytes",
            "boundary_read_bytes",
            "w_meas",
            "t_meas",
            "w_min",
            "t_min",
            "eta_w",
            "oh_wb",
            "traffic_per_output_byte",
            "write_read_asym",
            "write_access_per_boundary_elem",
            "read_access_per_boundary_elem",
            "dram_lb_optimistic_bytes",
            "dram_lb_boundary_spill_bytes",
            "decomp_error_bytes",
            "buffer_stats_available",
        ],
        sorted(summary_micro_rows, key=lambda r: (int(r["rounds"]), r["variant"])),
    )

    write_csv(
        fig_data_dir / "memory_breakdown_by_rounds.csv",
        ["rounds", "variant", "baseline_bytes", "boundary_write_bytes", "boundary_read_bytes", "total_bytes", "ratio_vs_fused"],
        sorted(memory_fig_rows, key=lambda r: (int(r["rounds"]), r["variant"])),
    )
    write_csv(
        fig_data_dir / "traffic_vs_rounds.csv",
        ["rounds", "variant", "total_bytes", "t_meas"],
        sorted(traffic_vs_round_rows, key=lambda r: (int(r["rounds"]), r["variant"])),
    )
    write_csv(
        fig_data_dir / "traffic_overhead_ratio.csv",
        ["rounds", "variant", "traffic_overhead_ratio", "cycle_overhead_ratio"],
        sorted(ratio_rows, key=lambda r: (int(r["rounds"]), r["variant"])),
    )
    write_csv(
        fig_data_dir / "traffic_linear_fit.csv",
        ["variant", "fit_target", "slope", "intercept", "r2"],
        linear_fit_rows,
    )
    write_csv(
        fig_data_dir / f"stall_spatial_delta_r{target_round}.csv",
        ["rounds", "variant", "x", "y", "fused_operand_wait", "variant_operand_wait", "delta_operand_wait"],
        spatial_rows,
    )
    write_csv(
        fig_data_dir / f"stall_column_delta_r{target_round}.csv",
        ["rounds", "variant", "column", "fused_operand_wait", "variant_operand_wait", "delta_operand_wait"],
        spatial_col_rows,
    )
    write_csv(
        fig_data_dir / f"stall_time_series_r{target_round}.csv",
        ["rounds", "variant", "cycle", "operand_wait_events"],
        temporal_rows,
    )
    write_csv(
        fig_data_dir / f"stall_phase_metrics_r{target_round}.csv",
        [
            "rounds",
            "variant",
            "phase_idx",
            "phase_start_cycle",
            "phase_end_cycle",
            "phase_cycles",
            "total_stall",
            "prologue_cycles",
            "prologue_stall",
            "steady_cycles",
            "steady_stall",
            "burst_fraction",
            "burst_peak",
            "steady_mean",
        ],
        phase_rows,
    )
    write_csv(
        fig_data_dir / f"stall_staged_space_time_r{target_round}.csv",
        ["rounds", "cycle", "column", "operand_wait_events"],
        staged_space_time_rows,
    )

    # Markdown report (concise but paper-ready structure).
    def stat(rounds: int, variant: str, field: str) -> str:
        for row in summary_micro_rows:
            if _to_int(row["rounds"]) == rounds and row["variant"] == variant:
                return str(row.get(field, ""))
        return ""

    rounds_list = sorted({r.rounds for r in runs})
    md_lines: List[str] = []
    md_lines.append("# MICRO Supplementary Report: Fused vs 1-Boundary vs 2-Boundary")
    md_lines.append("")
    md_lines.append(f"- Generated: {datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M:%S UTC')}")
    md_lines.append(f"- Input directory: `{input_dir}`")
    md_lines.append(f"- Sweep rounds: `{rounds_list}`")
    md_lines.append(f"- Dims: D0={d0}, D1={d1}, D2={d2}, D3={d3}; Ba={args.activation_bytes}B, Bw={args.weight_bytes}B")
    md_lines.append(f"- Buffer stats available: `{buffer_data_available}`")
    md_lines.append("")

    md_lines.append("## 1. Core Performance (cycle & stall)")
    md_lines.append("")
    md_lines.append("| rounds | fused cycles | b1 cycles | staged cycles | b1/fused | staged/fused | fused operand_wait | b1 operand_wait | staged operand_wait |")
    md_lines.append("|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
    for r in rounds_list:
        f_cycles = _to_int(stat(r, "fused", "cycles"))
        b1_cycles = _to_int(stat(r, "b1", "cycles"))
        st_cycles = _to_int(stat(r, "staged", "cycles"))
        md_lines.append(
            f"| {r} | {f_cycles} | {b1_cycles} | {st_cycles} | {_safe_div(b1_cycles, f_cycles):.3f} | {_safe_div(st_cycles, f_cycles):.3f} | {stat(r, 'fused', 'operand_wait_stall')} | {stat(r, 'b1', 'operand_wait_stall')} | {stat(r, 'staged', 'operand_wait_stall')} |"
        )
    md_lines.append("")

    md_lines.append("## 2. Inter-Phase Writeback Metrics")
    md_lines.append("")
    if buffer_data_available:
        md_lines.append("| rounds | variant | buf_write_bytes | buf_read_bytes | boundary_write_bytes | boundary_read_bytes | W_meas | T_meas | W_min | eta_w | OH_wb |")
        md_lines.append("|---:|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
        for r in rounds_list:
            for variant in ("b1", "staged"):
                md_lines.append(
                    f"| {r} | {variant} | {stat(r, variant, 'buf_write_bytes_total')} | {stat(r, variant, 'buf_read_bytes_total')} | {stat(r, variant, 'boundary_write_bytes')} | {stat(r, variant, 'boundary_read_bytes')} | {stat(r, variant, 'w_meas')} | {stat(r, variant, 't_meas')} | {stat(r, variant, 'w_min')} | {stat(r, variant, 'eta_w')} | {stat(r, variant, 'oh_wb')} |"
                )
        md_lines.append("")
        md_lines.append("- Decomposition consistency check: see `decomp_error_bytes` in `summary_micro.csv` (target < 1% of total traffic).")
        md_lines.append("- Linearity check: see `fig_data/traffic_linear_fit.csv` (target `R^2 > 0.99`).")
    else:
        md_lines.append("Buffer CSV/JSON was not provided or did not match expected fields; memory traffic values are left as zero.")
        md_lines.append("Recommended fields: `rounds,variant,buf_write_bytes_total,buf_read_bytes_total,buf_write_access_total,buf_read_access_total`.")
    md_lines.append("")

    md_lines.append("## 3. DRAM Lower-Bound (Analytical)")
    md_lines.append("")
    md_lines.append("- `DRAM_LB_fused = R*D0*Ba + (D0*D1 + D1*D2 + D2*D3)*Bw + R*D3*Ba`")
    md_lines.append("- `DRAM_extra_LB_staged(boundary spill) = 2*R*(D1 + D2)*Ba`")
    md_lines.append("- `DRAM_extra_LB_b1(boundary spill) = 2*R*D2*Ba`")
    md_lines.append("")

    md_lines.append("## 4. Stall Root-Cause Artifacts")
    md_lines.append("")
    md_lines.append(f"- Spatial delta heatmap data: `fig_data/stall_spatial_delta_r{target_round}.csv`")
    md_lines.append(f"- Column aggregation data: `fig_data/stall_column_delta_r{target_round}.csv`")
    md_lines.append(f"- Temporal series data: `fig_data/stall_time_series_r{target_round}.csv`")
    md_lines.append(f"- Phase burst metrics: `fig_data/stall_phase_metrics_r{target_round}.csv`")
    md_lines.append(f"- Staged cycle-column atlas data: `fig_data/stall_staged_space_time_r{target_round}.csv`")
    md_lines.append("")

    md_lines.append("## 5. Recommended Main Figures")
    md_lines.append("")
    md_lines.append("1. Memory stacked bar by rounds: use `fig_data/memory_breakdown_by_rounds.csv`.")
    md_lines.append("2. Stall atlas (time-series + cycle-column): use `fig_data/stall_time_series_*.csv` + `fig_data/stall_staged_space_time_*.csv`.")
    md_lines.append("3. Scaling line (`T_meas` vs rounds): use `fig_data/traffic_vs_rounds.csv`.")

    report_path = input_dir / "experiment_report_micro.md"
    report_path.write_text("\n".join(md_lines) + "\n", encoding="utf-8")

    print(f"wrote: {summary_micro_path}")
    print(f"wrote: {report_path}")
    print(f"wrote fig data dir: {fig_data_dir}")


if __name__ == "__main__":
    main()
