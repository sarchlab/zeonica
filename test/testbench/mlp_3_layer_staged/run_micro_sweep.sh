#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "${SCRIPT_DIR}/../../.." && pwd)"

FUSED_ARCH="${ROOT_DIR}/test/testbench/mlp_3_layer/arch_spec.yaml"
B1_ARCH="${ROOT_DIR}/test/testbench/mlp_3_layer_boundary1/arch_spec.yaml"
STAGED_ARCH="${ROOT_DIR}/test/testbench/mlp_3_layer_staged/arch_spec.yaml"

for f in "${FUSED_ARCH}" "${B1_ARCH}" "${STAGED_ARCH}"; do
  if [[ ! -f "${f}" ]]; then
    echo "missing arch spec: ${f}" >&2
    exit 1
  fi
done

ROUNDS_LIST="${ROUNDS_LIST:-16 32 64 96 128}"
SEED="${ZEONICA_RAND_SEED:-7}"
WMIN="${ZEONICA_MLP_WEIGHT_MIN:-1}"
WMAX="${ZEONICA_MLP_WEIGHT_MAX:-3}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_DIR="${OUT_DIR:-${SCRIPT_DIR}/sweep_reports_micro_${STAMP}}"

mkdir -p "${OUT_DIR}"
SUMMARY_CSV="${OUT_DIR}/summary.csv"
cat > "${SUMMARY_CSV}" << 'CSV'
rounds,fused_cycles,b1_cycles,staged_cycles,delta_b1_minus_fused,delta_staged_minus_fused,fused_active_cycles,b1_active_cycles,staged_active_cycles,fused_idle_cycles,b1_idle_cycles,staged_idle_cycles,fused_inst_count,b1_inst_count,staged_inst_count,fused_total_events,b1_total_events,staged_total_events,fused_inst_tp_cycle,b1_inst_tp_cycle,staged_inst_tp_cycle,fused_event_tp_cycle,b1_event_tp_cycle,staged_event_tp_cycle,fused_inst_tp_sec,b1_inst_tp_sec,staged_inst_tp_sec,fused_sched_stall,b1_sched_stall,staged_sched_stall,fused_operand_stall,b1_operand_stall,staged_operand_stall,fused_output_stall,b1_output_stall,staged_output_stall,fused_backpressure_count,b1_backpressure_count,staged_backpressure_count,fused_backpressure_cycles,b1_backpressure_cycles,staged_backpressure_cycles,fused_wall_sec,b1_wall_sec,staged_wall_sec,fused_avg_tile_util_pct,b1_avg_tile_util_pct,staged_avg_tile_util_pct,fused_peak_tile_util_pct,b1_peak_tile_util_pct,staged_peak_tile_util_pct,b1_stage12_cycles,b1_stage3_cycles,staged_stage1_cycles,staged_stage2_cycles,staged_stage3_cycles,fused_report,b1_report,staged_report,fused_log,b1_stage12_report,b1_stage3_report,b1_stage12_log,b1_stage3_log,staged_stage1_report,staged_stage2_report,staged_stage3_report,staged_stage1_log,staged_stage2_log,staged_stage3_log
CSV

for rounds in ${ROUNDS_LIST}; do
  echo "=== micro sweep rounds=${rounds} ==="

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
  FUSED_LOG_SRC="${ROOT_DIR}/mlp_3_layer.json.log"
  for required in "${FUSED_REPORT_SRC}" "${FUSED_LOG_SRC}"; do
    if [[ ! -f "${required}" ]]; then
      echo "missing fused artifact after run: ${required}" >&2
      exit 1
    fi
  done
  FUSED_REPORT_DST="${OUT_DIR}/fused_r${rounds}.report.json"
  FUSED_LOG_DST="${OUT_DIR}/fused_r${rounds}.json.log"
  cp "${FUSED_REPORT_SRC}" "${FUSED_REPORT_DST}"
  cp "${FUSED_LOG_SRC}" "${FUSED_LOG_DST}"

  (
    cd "${ROOT_DIR}"
    ZEONICA_ARCH_SPEC="${B1_ARCH}" \
    ZEONICA_MLP_ROUNDS="${rounds}" \
    ZEONICA_RAND_SEED="${SEED}" \
    ZEONICA_MLP_WEIGHT_MIN="${WMIN}" \
    ZEONICA_MLP_WEIGHT_MAX="${WMAX}" \
    go run ./test/testbench/mlp_3_layer_boundary1
  )

  B1_BASE="${ROOT_DIR}/test/testbench/mlp_3_layer_boundary1"
  B1_REPORT_SRC="${B1_BASE}/mlp_3_layer_boundary1.report.json"
  B1_STAGE12_REPORT_SRC="${B1_BASE}/mlp_3_layer_boundary1_stage12.report.json"
  B1_STAGE3_REPORT_SRC="${B1_BASE}/mlp_3_layer_boundary1_stage3.report.json"
  B1_STAGE12_LOG_SRC="${B1_BASE}/mlp_3_layer_boundary1_stage12.json.log"
  B1_STAGE3_LOG_SRC="${B1_BASE}/mlp_3_layer_boundary1_stage3.json.log"

  for required in "${B1_REPORT_SRC}" "${B1_STAGE12_REPORT_SRC}" "${B1_STAGE3_REPORT_SRC}" "${B1_STAGE12_LOG_SRC}" "${B1_STAGE3_LOG_SRC}"; do
    if [[ ! -f "${required}" ]]; then
      echo "missing boundary1 artifact after run: ${required}" >&2
      exit 1
    fi
  done

  B1_REPORT_DST="${OUT_DIR}/b1_r${rounds}.report.json"
  B1_STAGE12_REPORT_DST="${OUT_DIR}/b1_r${rounds}.stage12.report.json"
  B1_STAGE3_REPORT_DST="${OUT_DIR}/b1_r${rounds}.stage3.report.json"
  B1_STAGE12_LOG_DST="${OUT_DIR}/b1_r${rounds}.stage12.json.log"
  B1_STAGE3_LOG_DST="${OUT_DIR}/b1_r${rounds}.stage3.json.log"
  cp "${B1_REPORT_SRC}" "${B1_REPORT_DST}"
  cp "${B1_STAGE12_REPORT_SRC}" "${B1_STAGE12_REPORT_DST}"
  cp "${B1_STAGE3_REPORT_SRC}" "${B1_STAGE3_REPORT_DST}"
  cp "${B1_STAGE12_LOG_SRC}" "${B1_STAGE12_LOG_DST}"
  cp "${B1_STAGE3_LOG_SRC}" "${B1_STAGE3_LOG_DST}"

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
  STAGE1_REPORT_SRC="${STAGED_BASE}/mlp_3_layer_staged_stage1.report.json"
  STAGE2_REPORT_SRC="${STAGED_BASE}/mlp_3_layer_staged_stage2.report.json"
  STAGE3_REPORT_SRC="${STAGED_BASE}/mlp_3_layer_staged_stage3.report.json"
  STAGE1_LOG_SRC="${STAGED_BASE}/mlp_3_layer_staged_stage1.json.log"
  STAGE2_LOG_SRC="${STAGED_BASE}/mlp_3_layer_staged_stage2.json.log"
  STAGE3_LOG_SRC="${STAGED_BASE}/mlp_3_layer_staged_stage3.json.log"

  for required in "${STAGED_REPORT_SRC}" "${STAGE1_REPORT_SRC}" "${STAGE2_REPORT_SRC}" "${STAGE3_REPORT_SRC}" "${STAGE1_LOG_SRC}" "${STAGE2_LOG_SRC}" "${STAGE3_LOG_SRC}"; do
    if [[ ! -f "${required}" ]]; then
      echo "missing staged artifact after run: ${required}" >&2
      exit 1
    fi
  done

  STAGED_REPORT_DST="${OUT_DIR}/staged_r${rounds}.report.json"
  STAGE1_REPORT_DST="${OUT_DIR}/staged_r${rounds}.stage1.report.json"
  STAGE2_REPORT_DST="${OUT_DIR}/staged_r${rounds}.stage2.report.json"
  STAGE3_REPORT_DST="${OUT_DIR}/staged_r${rounds}.stage3.report.json"
  STAGE1_LOG_DST="${OUT_DIR}/staged_r${rounds}.stage1.json.log"
  STAGE2_LOG_DST="${OUT_DIR}/staged_r${rounds}.stage2.json.log"
  STAGE3_LOG_DST="${OUT_DIR}/staged_r${rounds}.stage3.json.log"
  cp "${STAGED_REPORT_SRC}" "${STAGED_REPORT_DST}"
  cp "${STAGE1_REPORT_SRC}" "${STAGE1_REPORT_DST}"
  cp "${STAGE2_REPORT_SRC}" "${STAGE2_REPORT_DST}"
  cp "${STAGE3_REPORT_SRC}" "${STAGE3_REPORT_DST}"
  cp "${STAGE1_LOG_SRC}" "${STAGE1_LOG_DST}"
  cp "${STAGE2_LOG_SRC}" "${STAGE2_LOG_DST}"
  cp "${STAGE3_LOG_SRC}" "${STAGE3_LOG_DST}"

  python3 - "${SUMMARY_CSV}" "${rounds}" \
    "${FUSED_REPORT_DST}" "${FUSED_LOG_DST}" \
    "${B1_REPORT_DST}" "${B1_STAGE12_REPORT_DST}" "${B1_STAGE3_REPORT_DST}" "${B1_STAGE12_LOG_DST}" "${B1_STAGE3_LOG_DST}" \
    "${STAGED_REPORT_DST}" "${STAGE1_REPORT_DST}" "${STAGE2_REPORT_DST}" "${STAGE3_REPORT_DST}" "${STAGE1_LOG_DST}" "${STAGE2_LOG_DST}" "${STAGE3_LOG_DST}" << 'PY'
import csv
import json
import sys

(
    summary_csv,
    rounds,
    fused_report,
    fused_log,
    b1_unified,
    b1_stage12,
    b1_stage3,
    b1_stage12_log,
    b1_stage3_log,
    staged_unified,
    staged_stage1,
    staged_stage2,
    staged_stage3,
    staged_stage1_log,
    staged_stage2_log,
    staged_stage3_log,
) = sys.argv[1:]


def load(path):
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def safe_div(a, b):
    if float(b) == 0.0:
        return 0.0
    return float(a) / float(b)


def agg_from_stage_reports(stage_reports):
    total_cycles = sum(int(r.get("totalCycles", 0)) for r in stage_reports)
    active = sum(int(r.get("activeCyclesGlobal", 0)) for r in stage_reports)
    idle = sum(int(r.get("idleCyclesGlobal", 0)) for r in stage_reports)
    inst = sum(int(r.get("instCount", 0)) for r in stage_reports)
    events = sum(int(r.get("totalEvents", 0)) for r in stage_reports)
    sched = sum(int(r.get("scheduleBubbleStallCount", 0)) for r in stage_reports)
    operand = sum(int(r.get("operandWaitStallCount", 0)) for r in stage_reports)
    output = sum(int(r.get("outputBlockedStallCount", 0)) for r in stage_reports)
    bp_count = sum(int(r.get("backpressureCount", 0)) for r in stage_reports)
    bp_cycles = sum(int(r.get("backpressureCycles", 0)) for r in stage_reports)
    wall = sum(float(r.get("wallClockDurationSec", 0.0)) for r in stage_reports)

    tile_count = len(stage_reports[0].get("tiles", [])) if stage_reports else 0
    tile_active = {}
    for report in stage_reports:
        for t in report.get("tiles", []):
            coord = t.get("coord", "")
            tile_active[coord] = tile_active.get(coord, 0) + int(t.get("activeCycles", 0))

    avg_util = 100.0 * safe_div(sum(tile_active.values()), total_cycles * tile_count) if tile_count > 0 else 0.0
    peak_util = max((100.0 * safe_div(v, total_cycles) for v in tile_active.values()), default=0.0)

    return {
        "cycles": total_cycles,
        "active": active,
        "idle": idle,
        "inst": inst,
        "events": events,
        "inst_tp_cycle": safe_div(inst, total_cycles),
        "event_tp_cycle": safe_div(events, total_cycles),
        "inst_tp_sec": safe_div(inst, wall),
        "sched": sched,
        "operand": operand,
        "output": output,
        "bp_count": bp_count,
        "bp_cycles": bp_cycles,
        "wall": wall,
        "avg_util": avg_util,
        "peak_util": peak_util,
    }


fused = load(fused_report)
fused_stats = {
    "cycles": int(fused.get("totalCycles", 0)),
    "active": int(fused.get("activeCyclesGlobal", 0)),
    "idle": int(fused.get("idleCyclesGlobal", 0)),
    "inst": int(fused.get("instCount", 0)),
    "events": int(fused.get("totalEvents", 0)),
    "inst_tp_cycle": float(fused.get("instThroughputPerCycle", 0.0)),
    "event_tp_cycle": float(fused.get("eventThroughputPerCycle", 0.0)),
    "inst_tp_sec": float(fused.get("instThroughputPerSec", 0.0)),
    "sched": int(fused.get("scheduleBubbleStallCount", 0)),
    "operand": int(fused.get("operandWaitStallCount", 0)),
    "output": int(fused.get("outputBlockedStallCount", 0)),
    "bp_count": int(fused.get("backpressureCount", 0)),
    "bp_cycles": int(fused.get("backpressureCycles", 0)),
    "wall": float(fused.get("wallClockDurationSec", 0.0)),
    "avg_util": 100.0 * safe_div(sum(int(t.get("activeCycles", 0)) for t in fused.get("tiles", [])), int(fused.get("totalCycles", 0)) * max(len(fused.get("tiles", [])), 1)),
    "peak_util": max((float(t.get("utilizationPct", 0.0)) for t in fused.get("tiles", [])), default=0.0),
}

b1_stage12_report = load(b1_stage12)
b1_stage3_report = load(b1_stage3)
b1_stats = agg_from_stage_reports([b1_stage12_report, b1_stage3_report])

staged_stage1_report = load(staged_stage1)
staged_stage2_report = load(staged_stage2)
staged_stage3_report = load(staged_stage3)
staged_stats = agg_from_stage_reports([staged_stage1_report, staged_stage2_report, staged_stage3_report])

row = [
    int(rounds),
    fused_stats["cycles"],
    b1_stats["cycles"],
    staged_stats["cycles"],
    b1_stats["cycles"] - fused_stats["cycles"],
    staged_stats["cycles"] - fused_stats["cycles"],
    fused_stats["active"],
    b1_stats["active"],
    staged_stats["active"],
    fused_stats["idle"],
    b1_stats["idle"],
    staged_stats["idle"],
    fused_stats["inst"],
    b1_stats["inst"],
    staged_stats["inst"],
    fused_stats["events"],
    b1_stats["events"],
    staged_stats["events"],
    f"{fused_stats['inst_tp_cycle']:.6f}",
    f"{b1_stats['inst_tp_cycle']:.6f}",
    f"{staged_stats['inst_tp_cycle']:.6f}",
    f"{fused_stats['event_tp_cycle']:.6f}",
    f"{b1_stats['event_tp_cycle']:.6f}",
    f"{staged_stats['event_tp_cycle']:.6f}",
    f"{fused_stats['inst_tp_sec']:.6f}",
    f"{b1_stats['inst_tp_sec']:.6f}",
    f"{staged_stats['inst_tp_sec']:.6f}",
    fused_stats["sched"],
    b1_stats["sched"],
    staged_stats["sched"],
    fused_stats["operand"],
    b1_stats["operand"],
    staged_stats["operand"],
    fused_stats["output"],
    b1_stats["output"],
    staged_stats["output"],
    fused_stats["bp_count"],
    b1_stats["bp_count"],
    staged_stats["bp_count"],
    fused_stats["bp_cycles"],
    b1_stats["bp_cycles"],
    staged_stats["bp_cycles"],
    f"{fused_stats['wall']:.6f}",
    f"{b1_stats['wall']:.6f}",
    f"{staged_stats['wall']:.6f}",
    f"{fused_stats['avg_util']:.6f}",
    f"{b1_stats['avg_util']:.6f}",
    f"{staged_stats['avg_util']:.6f}",
    f"{fused_stats['peak_util']:.6f}",
    f"{b1_stats['peak_util']:.6f}",
    f"{staged_stats['peak_util']:.6f}",
    int(b1_stage12_report.get("totalCycles", 0)),
    int(b1_stage3_report.get("totalCycles", 0)),
    int(staged_stage1_report.get("totalCycles", 0)),
    int(staged_stage2_report.get("totalCycles", 0)),
    int(staged_stage3_report.get("totalCycles", 0)),
    fused_report.split("/")[-1],
    b1_unified.split("/")[-1],
    staged_unified.split("/")[-1],
    fused_log.split("/")[-1],
    b1_stage12.split("/")[-1],
    b1_stage3.split("/")[-1],
    b1_stage12_log.split("/")[-1],
    b1_stage3_log.split("/")[-1],
    staged_stage1.split("/")[-1],
    staged_stage2.split("/")[-1],
    staged_stage3.split("/")[-1],
    staged_stage1_log.split("/")[-1],
    staged_stage2_log.split("/")[-1],
    staged_stage3_log.split("/")[-1],
]

with open(summary_csv, "a", newline="", encoding="utf-8") as f:
    csv.writer(f).writerow(row)
PY
done

echo "micro sweep complete"
echo "output dir: ${OUT_DIR}"
echo "summary: ${SUMMARY_CSV}"

if [[ "${RUN_MICRO_ANALYSIS:-1}" == "1" ]]; then
  ANALYZE_ARGS=(--input-dir "${OUT_DIR}")
  if [[ -n "${BUFFER_STATS_PATH:-}" ]]; then
    ANALYZE_ARGS+=(--buffer-stats "${BUFFER_STATS_PATH}")
  fi
  if [[ -n "${MICRO_DIMS:-}" ]]; then
    ANALYZE_ARGS+=(--dims "${MICRO_DIMS}")
  fi
  if [[ -n "${ACTIVATION_BYTES:-}" ]]; then
    ANALYZE_ARGS+=(--activation-bytes "${ACTIVATION_BYTES}")
  fi
  if [[ -n "${WEIGHT_BYTES:-}" ]]; then
    ANALYZE_ARGS+=(--weight-bytes "${WEIGHT_BYTES}")
  fi
  if [[ -n "${PROLOGUE_CYCLES:-}" ]]; then
    ANALYZE_ARGS+=(--prologue-cycles "${PROLOGUE_CYCLES}")
  fi
  if [[ -n "${STALL_ROUND:-}" ]]; then
    ANALYZE_ARGS+=(--stall-round "${STALL_ROUND}")
  fi

  if [[ -x "${SCRIPT_DIR}/analyze_micro_metrics.py" ]]; then
    "${SCRIPT_DIR}/analyze_micro_metrics.py" "${ANALYZE_ARGS[@]}"
  else
    python3 "${SCRIPT_DIR}/analyze_micro_metrics.py" "${ANALYZE_ARGS[@]}"
  fi
fi
