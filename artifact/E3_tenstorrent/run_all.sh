#!/usr/bin/env bash
# Reproduce Fig. 6 / Table II: Zeonica's TT/Wormhole model vs TT-Sim on the 10
# held-out TT-Metal gtests (paper: MAPE 3.28%, max 9.15% for the 3x3 matmul).
#
#   1. check_freeze.py  frozen config + this repository's sources == freeze-4 hash record;
#                       TT-Sim reference cycles == recorded dump-off gtest runs
#   2. run_zeonica.py   build this repository (git HEAD) and run the 10 kernels
#   3. evaluate.py      APE/MAPE, PASS/FAIL against the paper and against freeze 4
#   4. make_table2.py   results/table2.csv (TT-Sim configuration vs model configuration)
#   5. plot_fig6.py     figures/fig6_tenstorrent.{pdf,png}
#
# TT-Sim is not rerun; its cycles and tensor images are reference data (reference/).
# Usage: ./run_all.sh [--skip-run]     (--skip-run reuses results/zeonica_run)
# Env:   PYTHON (default python3), JOBS (parallel kernel runs, default 8)
set -euo pipefail
cd "$(dirname "$0")"
PY="${PYTHON:-python3}"

status=0
"$PY" scripts/check_freeze.py || status=1
if [[ "${1:-}" != "--skip-run" ]]; then
  "$PY" scripts/run_zeonica.py --jobs "${JOBS:-8}"
fi
"$PY" scripts/evaluate.py || status=1
"$PY" scripts/make_table2.py || status=1
"$PY" scripts/plot_fig6.py
if [[ $status -eq 0 ]]; then echo "run_all: PASS"; else echo "run_all: FAIL"; fi
exit $status
