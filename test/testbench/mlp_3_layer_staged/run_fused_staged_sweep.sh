#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "${SCRIPT_DIR}/../../.." && pwd)"
FUSED_ARCH="${ROOT_DIR}/test/testbench/mlp_3_layer/arch_spec.yaml"
STAGED_ARCH="${ROOT_DIR}/test/testbench/mlp_3_layer_staged/arch_spec.yaml"

if [[ ! -f "${FUSED_ARCH}" ]]; then
  echo "missing fused arch spec: ${FUSED_ARCH}" >&2
  exit 1
fi
if [[ ! -f "${STAGED_ARCH}" ]]; then
  echo "missing staged arch spec: ${STAGED_ARCH}" >&2
  exit 1
fi

ROUNDS_LIST="${ROUNDS_LIST:-16 32 64 96 128}"
SEED="${ZEONICA_RAND_SEED:-7}"
WMIN="${ZEONICA_MLP_WEIGHT_MIN:-1}"
WMAX="${ZEONICA_MLP_WEIGHT_MAX:-3}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_DIR="${OUT_DIR:-${SCRIPT_DIR}/sweep_reports_${STAMP}}"

mkdir -p "${OUT_DIR}"
SUMMARY_CSV="${OUT_DIR}/summary.csv"
cat > "${SUMMARY_CSV}" << 'CSV'
rounds,fused_cycles,staged_cycles,delta_staged_minus_fused,fused_active_cycles,staged_active_cycles,fused_idle_cycles,staged_idle_cycles,fused_inst_count,staged_inst_count,fused_total_events,staged_total_events,fused_inst_throughput_per_cycle,staged_inst_throughput_per_cycle,fused_event_throughput_per_cycle,staged_event_throughput_per_cycle,fused_inst_throughput_per_sec,staged_inst_throughput_per_sec,fused_schedule_bubble_stall,staged_schedule_bubble_stall,fused_operand_wait_stall,staged_operand_wait_stall,fused_output_blocked_stall,staged_output_blocked_stall,fused_backpressure_count,staged_backpressure_count,fused_backpressure_cycles,staged_backpressure_cycles,fused_wall_clock_sec,staged_wall_clock_sec,fused_avg_tile_util_pct,staged_avg_tile_util_pct,fused_peak_tile_util_pct,staged_peak_tile_util_pct,staged_stage1_cycles,staged_stage2_cycles,staged_stage3_cycles,fused_report,staged_report,staged_stage1_report,staged_stage2_report,staged_stage3_report
CSV

for rounds in ${ROUNDS_LIST}; do
  echo "=== sweep rounds=${rounds} ==="

  (
    cd "${ROOT_DIR}"
    ZEONICA_ARCH_SPEC="${FUSED_ARCH}" \
    ZEONICA_MLP_ROUNDS="${rounds}" \
    ZEONICA_RAND_SEED="${SEED}" \
    ZEONICA_MLP_WEIGHT_MIN="${WMIN}" \
    ZEONICA_MLP_WEIGHT_MAX="${WMAX}" \
    go run ./test/testbench/mlp_3_layer
  )

  FUSED_REPORT_SRC="${ROOT_DIR}/mlp_3_layer.report.json"
  if [[ ! -f "${FUSED_REPORT_SRC}" ]]; then
    echo "missing fused report after run: ${FUSED_REPORT_SRC}" >&2
    exit 1
  fi
  FUSED_REPORT_DST="${OUT_DIR}/fused_r${rounds}.report.json"
  cp "${FUSED_REPORT_SRC}" "${FUSED_REPORT_DST}"

  (
    cd "${ROOT_DIR}"
    ZEONICA_ARCH_SPEC="${STAGED_ARCH}" \
    ZEONICA_MLP_ROUNDS="${rounds}" \
    ZEONICA_RAND_SEED="${SEED}" \
    ZEONICA_MLP_WEIGHT_MIN="${WMIN}" \
    ZEONICA_MLP_WEIGHT_MAX="${WMAX}" \
    go run ./test/testbench/mlp_3_layer_staged
  )

  STAGED_BASE="${ROOT_DIR}/test/testbench/mlp_3_layer_staged"
  STAGED_REPORT_SRC="${STAGED_BASE}/mlp_3_layer_staged.report.json"
  STAGED_UNIFIED_SRC="${STAGED_BASE}/mlp_3_layer_staged.unified.report.json"
  STAGE1_SRC="${STAGED_BASE}/mlp_3_layer_staged_stage1.report.json"
  STAGE2_SRC="${STAGED_BASE}/mlp_3_layer_staged_stage2.report.json"
  STAGE3_SRC="${STAGED_BASE}/mlp_3_layer_staged_stage3.report.json"

  for required in "${STAGED_REPORT_SRC}" "${STAGED_UNIFIED_SRC}" "${STAGE1_SRC}" "${STAGE2_SRC}" "${STAGE3_SRC}"; do
    if [[ ! -f "${required}" ]]; then
      echo "missing staged report after run: ${required}" >&2
      exit 1
    fi
  done

  STAGED_REPORT_DST="${OUT_DIR}/staged_r${rounds}.report.json"
  STAGED_UNIFIED_DST="${OUT_DIR}/staged_r${rounds}.unified.report.json"
  STAGE1_DST="${OUT_DIR}/staged_r${rounds}.stage1.report.json"
  STAGE2_DST="${OUT_DIR}/staged_r${rounds}.stage2.report.json"
  STAGE3_DST="${OUT_DIR}/staged_r${rounds}.stage3.report.json"
  cp "${STAGED_REPORT_SRC}" "${STAGED_REPORT_DST}"
  cp "${STAGED_UNIFIED_SRC}" "${STAGED_UNIFIED_DST}"
  cp "${STAGE1_SRC}" "${STAGE1_DST}"
  cp "${STAGE2_SRC}" "${STAGE2_DST}"
  cp "${STAGE3_SRC}" "${STAGE3_DST}"

  python3 - "${SUMMARY_CSV}" "${rounds}" "${FUSED_REPORT_DST}" "${STAGED_REPORT_DST}" "${STAGE1_DST}" "${STAGE2_DST}" "${STAGE3_DST}" << 'PY'
import csv
import json
import math
import sys

summary_csv, rounds, fused_path, staged_path, stage1_path, stage2_path, stage3_path = sys.argv[1:]

def load(path):
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)

def safe_div(a, b):
    return 0.0 if b == 0 else float(a) / float(b)

def avg_tile_util_from_active(total_tile_active, total_cycles, tile_count):
    if total_cycles == 0 or tile_count == 0:
        return 0.0
    return 100.0 * float(total_tile_active) / float(total_cycles * tile_count)

fused = load(fused_path)
staged = load(staged_path)
stage_reports = [load(stage1_path), load(stage2_path), load(stage3_path)]

fused_total_cycles = int(fused.get("totalCycles", 0))
staged_total_cycles = int(staged.get("totalCycles", 0))
delta_cycles = staged_total_cycles - fused_total_cycles

fused_active_cycles = int(fused.get("activeCyclesGlobal", 0))
fused_idle_cycles = int(fused.get("idleCyclesGlobal", 0))
fused_inst_count = int(fused.get("instCount", 0))
fused_total_events = int(fused.get("totalEvents", 0))
fused_inst_tp_cycle = float(fused.get("instThroughputPerCycle", 0.0))
fused_evt_tp_cycle = float(fused.get("eventThroughputPerCycle", 0.0))
fused_inst_tp_sec = float(fused.get("instThroughputPerSec", 0.0))
fused_sched_stall = int(fused.get("scheduleBubbleStallCount", 0))
fused_operand_stall = int(fused.get("operandWaitStallCount", 0))
fused_output_stall = int(fused.get("outputBlockedStallCount", 0))
fused_bp_count = int(fused.get("backpressureCount", 0))
fused_bp_cycles = int(fused.get("backpressureCycles", 0))
fused_wall_sec = float(fused.get("wallClockDurationSec", 0.0))

fused_tiles = fused.get("tiles", [])
fused_tile_count = len(fused_tiles)
fused_total_tile_active = sum(int(t.get("activeCycles", 0)) for t in fused_tiles)
fused_avg_tile_util = avg_tile_util_from_active(fused_total_tile_active, fused_total_cycles, fused_tile_count)
fused_peak_tile_util = max((float(t.get("utilizationPct", 0.0)) for t in fused_tiles), default=0.0)

staged_active_cycles = sum(int(r.get("activeCyclesGlobal", 0)) for r in stage_reports)
staged_idle_cycles = sum(int(r.get("idleCyclesGlobal", 0)) for r in stage_reports)
staged_inst_count = sum(int(r.get("instCount", 0)) for r in stage_reports)
staged_total_events = sum(int(r.get("totalEvents", 0)) for r in stage_reports)
staged_sched_stall = sum(int(r.get("scheduleBubbleStallCount", 0)) for r in stage_reports)
staged_operand_stall = sum(int(r.get("operandWaitStallCount", 0)) for r in stage_reports)
staged_output_stall = sum(int(r.get("outputBlockedStallCount", 0)) for r in stage_reports)
staged_bp_count = sum(int(r.get("backpressureCount", 0)) for r in stage_reports)
staged_bp_cycles = sum(int(r.get("backpressureCycles", 0)) for r in stage_reports)
staged_wall_sec = sum(float(r.get("wallClockDurationSec", 0.0)) for r in stage_reports)
staged_inst_tp_cycle = safe_div(staged_inst_count, staged_total_cycles)
staged_evt_tp_cycle = safe_div(staged_total_events, staged_total_cycles)
staged_inst_tp_sec = safe_div(staged_inst_count, staged_wall_sec)

staged_tiles_by_coord = {}
for report in stage_reports:
    for tile in report.get("tiles", []):
        coord = tile.get("coord", "")
        staged_tiles_by_coord[coord] = staged_tiles_by_coord.get(coord, 0) + int(tile.get("activeCycles", 0))

staged_tile_count = len(stage_reports[0].get("tiles", [])) if stage_reports else 0
staged_total_tile_active = sum(staged_tiles_by_coord.values())
staged_avg_tile_util = avg_tile_util_from_active(staged_total_tile_active, staged_total_cycles, staged_tile_count)
staged_peak_tile_util = 0.0
if staged_total_cycles > 0:
    staged_peak_tile_util = max((100.0 * safe_div(v, staged_total_cycles) for v in staged_tiles_by_coord.values()), default=0.0)

stage_cycles = [int(r.get("totalCycles", 0)) for r in stage_reports]

row = [
    int(rounds),
    fused_total_cycles,
    staged_total_cycles,
    delta_cycles,
    fused_active_cycles,
    staged_active_cycles,
    fused_idle_cycles,
    staged_idle_cycles,
    fused_inst_count,
    staged_inst_count,
    fused_total_events,
    staged_total_events,
    f"{fused_inst_tp_cycle:.6f}",
    f"{staged_inst_tp_cycle:.6f}",
    f"{fused_evt_tp_cycle:.6f}",
    f"{staged_evt_tp_cycle:.6f}",
    f"{fused_inst_tp_sec:.6f}",
    f"{staged_inst_tp_sec:.6f}",
    fused_sched_stall,
    staged_sched_stall,
    fused_operand_stall,
    staged_operand_stall,
    fused_output_stall,
    staged_output_stall,
    fused_bp_count,
    staged_bp_count,
    fused_bp_cycles,
    staged_bp_cycles,
    f"{fused_wall_sec:.6f}",
    f"{staged_wall_sec:.6f}",
    f"{fused_avg_tile_util:.6f}",
    f"{staged_avg_tile_util:.6f}",
    f"{fused_peak_tile_util:.6f}",
    f"{staged_peak_tile_util:.6f}",
    stage_cycles[0] if len(stage_cycles) > 0 else 0,
    stage_cycles[1] if len(stage_cycles) > 1 else 0,
    stage_cycles[2] if len(stage_cycles) > 2 else 0,
    fused_path.split("/")[-1],
    staged_path.split("/")[-1],
    stage1_path.split("/")[-1],
    stage2_path.split("/")[-1],
    stage3_path.split("/")[-1],
]

with open(summary_csv, "a", newline="", encoding="utf-8") as f:
    writer = csv.writer(f)
    writer.writerow(row)
PY
done

echo "sweep complete."
echo "output dir: ${OUT_DIR}"
echo "summary: ${SUMMARY_CSV}"
