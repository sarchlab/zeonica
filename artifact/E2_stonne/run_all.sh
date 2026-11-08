#!/usr/bin/env bash
# Reproduce Fig. 5 (Zeonica vs. STONNE systolic GEMM cycles) and Fig. 7d
# (Zeonica-to-STONNE host-time ratio) from scratch:
#
#   figures/paper/     run cycles + the host times printed in the paper
#   figures/measured/  run cycles + host times measured on this machine
#
# Usage: STONNE_DIR=/path/to/stonne [ZEONICA_DIR=../..] \
#        [WARMUP=1] [REPS=3] [SOURCE=head|worktree] ./run_all.sh [--skip-run]
set -euo pipefail
cd "$(dirname "$0")"

ZEONICA_DIR="${ZEONICA_DIR:-../..}"  # this repository
STONNE_DIR="${STONNE_DIR:-../../stonne}"
PY="${PYTHON:-python3}"

if [[ "${1:-}" != "--skip-run" ]]; then
  "$PY" scripts/run_stonne.py --stonne "$STONNE_DIR" --out results/stonne_run \
        --warmup "${WARMUP:-1}" --reps "${REPS:-3}"
  "$PY" scripts/run_zeonica.py --zeonica "$ZEONICA_DIR" --out results/zeonica_run \
        --warmup "${WARMUP:-1}" --reps "${REPS:-3}" --source "${SOURCE:-head}"
fi

status=0
"$PY" scripts/compare.py || status=1

for variant in paper measured; do
  "$PY" scripts/plot_figures.py --input "results/$variant/plot_input.csv" \
        --output-dir "figures/$variant"
done
exit $status
