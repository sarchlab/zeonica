# E2: Systolic GEMM, Zeonica vs. STONNE (Fig. 5 and Fig. 7d)

This experiment reproduces two figures:

- **Fig. 5:** cycles of an output-stationary systolic GEMM with M = N = K = N on an
  N x N array, for N in {4, 8, 16, 32, 64, 128}, on Zeonica and on STONNE.
- **Fig. 7d:** the ratio of Zeonica host time to STONNE host time for each size.

Both simulators are built from clean snapshots and rerun from scratch.

| Figure | What is checked | Result on the reference machine |
|---|---|---|
| Fig. 5 | Exact cycles, per-size residual labels, MAPE | identical to the paper: Zeonica 15/31/63/127/255/511, STONNE 15/35/67/131/259/515, MAPE **3.80%** |
| Fig. 7d | Qualitative trend: the ratio grows with N | grows with N (0.53x at 4x4 up to 17.97x at 128x128) |

Host times depend on the machine, so both simulators are re-timed on your
machine. The paper's ratios are printed next to yours for comparison.

**Fig. 7d as published is kept unchanged.** `figures/paper/` regenerates it
from the paper's historical host times. `figures/measured/` regenerates it
from this machine's host times on the same boundary as the paper: Zeonica's
`wallClockDurationSec` and STONNE's process time (see
[Fig. 7d timing boundary](#fig-7d-timing-boundary)). The claim the artifact
supports on any machine is the trend: the Zeonica/STONNE host-time ratio grows
with N. Where the ratio crosses 1x depends on the host: between 8x8 and 16x16
in the paper, between 4x4 and 8x8 on the reference machine.

## Requirements

- Go 1.25 (tested with go1.25.3, linux/amd64)
- g++ with C++17 and GNU make (tested with g++ 11.4.0)
- Python 3 with `matplotlib` and `numpy`
- A git clone of this repository on branch `hpca27-ae/e2-stonne`, with the
  `test/Zeonica_Testbench` submodule initialized (`git submodule update --init`;
  tested at submodule commit `6ed5ac6`)
- A STONNE checkout (https://github.com/stonne-simulator/stonne) at commit
  `22634b8b`, unmodified

## Run

```bash
cd artifact/E2_stonne
STONNE_DIR=/path/to/stonne ./run_all.sh
```

This takes about 12 minutes (697 s for a full run with the defaults) on the reference machine. Most of it is Zeonica
128x128, at about 2.3 minutes per run.

`./run_all.sh --skip-run` reuses the existing runs and only redoes the
comparison and the figures. Optional environment variables:

- `WARMUP` (default 1) and `REPS` (default 3): unmeasured and measured runs per
  size, for both simulators. Each Zeonica 128x128 run takes about 2.3 minutes, so
  every extra run adds that much. For example, `WARMUP=0 REPS=1` takes about 4 minutes (an estimate; this setting was not timed).
- `SOURCE` (default `head`): which Zeonica source to snapshot. `head` uses the
  checkout's git HEAD. `worktree` uses HEAD plus uncommitted edits to tracked
  files and is meant only for development.

All checks run automatically. `scripts/compare.py` prints one line per check,
and `run_all.sh` exits non-zero if any check prints `FAIL`.

## What happens

1. **`scripts/run_stonne.py`** builds and runs STONNE.
   - It unpacks `git archive HEAD stonne/` from your STONNE checkout into
     `results/stonne_run/src/` and builds it there with STONNE's unmodified
     Makefile. Your checkout is not touched.
   - For each size, it runs the exact command in the `Stonne_config` column of
     the paper's `data.csv`. Before running, it checks that this command matches
     `config/sizes.json`.
   - It checks that every run prints `Test passed correctly`.
   - It hashes the statistics files that STONNE writes and compares them with
     the files left by the historical sweep.
2. **`scripts/run_zeonica.py`** builds and runs Zeonica.
   - It builds a clean source snapshot from this repository's git HEAD
     (without `artifact/`) and the submodule HEAD. Your working tree is never
     modified. `ZEONICA_DIR` selects another Zeonica checkout; it must contain
     the `test/testbench/stonneGEMM*` testbenches.
   - It builds one binary per size and runs it with a fixed seed
     (`ZEONICA_RAND_SEED=12345`).
   - It fails if a run's output does not match the CPU reference GEMM.
3. **`scripts/compare.py`** compares the runs with the paper and runs the checks.
   - It writes `results/comparison.csv`.
   - It writes `results/paper/plot_input.csv` and `results/measured/plot_input.csv`
     in the layout of the paper's `data.csv`.
   - It writes `results/summary.json` with every check.
4. **`scripts/plot_figures.py`** draws both figures into `figures/paper/` and
   `figures/measured/`:
   - `comparison_plot_hpca.{pdf,png}` is Fig. 5.
   - `zeonica_stonne_host_time_ratio.{pdf,png}` is Fig. 7d.

   It uses the paper's plotting scripts, unmodified:
   `scripts/plot_fig6_hpca.py` (sha256 `1882d6f1…`; despite its name it draws
   Fig. 5) and `scripts/plot_host_time_ratio.py` (sha256 `9a7ba730…`). Both scripts
   hard-code `data.csv` and an output path next to themselves. The wrapper
   loads each one, points those two module variables at the given CSV and
   output directory, and calls its `main()`. It checks both hashes first.

The two figure directories differ only in Fig. 7d. Fig. 5 uses the cycles
from this run in both. In `paper/`, Fig. 7d uses the host times printed in the
paper, so the figure reproduces the published one. In `measured/`, it uses this
machine's host times.

## Checks

| Check | Gating | Meaning |
|---|---|---|
| `fig5_zeonica_cycles`, `fig5_stonne_cycles` | yes | cycles equal the paper for every size |
| `fig5_mape` | yes | MAPE rounds to 3.80% (exact value 3.7955%) |
| `fig5_residual_labels` | yes | the per-size labels are +0.0, -11.4, -6.0, -3.1, -1.5, -0.8 % |
| `correctness` | yes | both simulators compute the correct GEMM at every size |
| `paper_plot_input_equals_data_csv` | yes | `results/paper/plot_input.csv` equals the paper's `data.csv` on every plotted field |
| `fig7d_paper_ratio_labels` | yes | the paper's `data.csv` gives the labeled ratios 0.27x, 2.60x, 28.39x |
| `fig7d_ratio_grows_with_N` | yes | the measured ratio strictly increases with N |
| `stonne_stats_byte_identical_to_historical_sweep` | no (INFO) | STONNE's statistics files are byte-identical to the historical sweep |
| `fig7d_parity_crossing` | no (INFO) | where the measured ratio crosses 1x |
| `fig7d_process_time_ratio` | no (INFO) | supplementary ratio with both simulators timed as whole processes |

The parity check does not gate because where the ratio crosses 1x depends on
the host.

## Results on the reference machine

Host: 128-core x86_64, Linux 5.15, go1.25.3, g++ 11.4.0. Zeonica `ad19fef`
(this branch without `artifact/`; submodule `6ed5ac6`), STONNE `22634b8b`, `WARMUP=1 REPS=3`,
medians of 3 measured runs. Source: `results/comparison.csv`.

| N | Zeonica cycles | STONNE cycles | Residual | Zeonica `wallClockDurationSec` (s) | STONNE process (s) | Measured ratio | Paper ratio |
|---|---|---|---|---|---|---|---|
| 4 | 15 | 15 | +0.0% | 0.004 | 0.007 | 0.53x | 0.27x |
| 8 | 31 | 35 | -11.4% | 0.034 | 0.018 | 1.87x | 0.88x |
| 16 | 63 | 67 | -6.0% | 0.286 | 0.062 | 4.63x | 2.60x |
| 32 | 127 | 131 | -3.1% | 2.254 | 0.245 | 9.20x | 6.31x |
| 64 | 255 | 259 | -1.5% | 17.394 | 1.130 | 15.40x | 13.12x |
| 128 | 511 | 515 | -0.8% | 135.171 | 7.523 | 17.97x | 28.39x |

`results/comparison.csv` also lists Zeonica's whole-process time
(`zeonica_ext_s`) and the ratio with both simulators timed as processes
(`ratio_process_both`: 2.61x to 18.39x). That ratio is supplementary and not
plotted.

- Fig. 5 is reproduced exactly: same cycles, labels and MAPE. The PNGs in
  `figures/paper/` are pixel-identical to the published PNGs.
- Every gating check printed `PASS`.
- STONNE's statistics files are byte-identical to the historical sweep for
  8x8 to 128x128, and for 4x4 when it is run with `num_ms=64`
  (see [STONNE configuration](#stonne-configuration)).
- In `figures/measured/`, the paper script places the last label at a fixed
  offset to the left of the last point. With this machine's values that label
  overlaps the 64x64 point. This is cosmetic and comes from the unmodified script.

## Fig. 7d timing boundary

Fig. 7d uses the same boundary as the paper:

- **Zeonica:** the `wallClockDurationSec` field of the Zeonica report. It is
  the wall-time span from the first to the last simulation event inside
  `driver.Run`. The paper's Zeonica times are this field of the retained
  reports; for example, 64x64 is 17.511 s and 128x128 is 161.712 s, the same
  values as in `data.csv`.
- **STONNE:** the external monotonic elapsed time around the prebuilt STONNE
  process (`time.monotonic()` in Python around `subprocess.run`), because
  STONNE has no internal timer. Its default statistics output stays on, as in
  the historical runs.

Each value is the median of `REPS` runs after `WARMUP` runs. Neither process
is pinned to a CPU, and build time is excluded.

**Tracing.** Zeonica tracing is **off** for every timed run. The committed arch
specs disagree: 4x4 (which uses `test/arch_spec/arch_spec.yaml`), 8x8 and
32x32 set `enableTrace: true`, while 16x16, 64x64 and 128x128 set it to
false. `run_zeonica.py` therefore writes `arch_spec.timing.yaml` next to each
testbench in the snapshot. This file is the size's spec with only
`enableTrace` forced to `false`, and the timed runs select it through
`ZEONICA_ARCH_SPEC`. Each timed run is checked to leave an empty trace file.
For the three sizes whose committed spec enables tracing, one extra untimed
run with the committed spec confirms that the cycle count is the same
(`committed_spec_cycles` in `results/zeonica_run/summary.csv`).

**Comparison with the paper.** The trend reproduces: the ratio grows steadily
with N (gating check `fig7d_ratio_grows_with_N`). It crosses 1x between 4x4
and 8x8 here, versus between 8x8 and 16x16 in the paper. The absolute ratio at
128x128 is lower here than in the paper: 18.0x vs. 28.4x. Zeonica is faster on
this machine (135.2 s vs. 161.7 s) and STONNE is slower (7.5 s vs. 5.7 s).

The 32x32 Zeonica time in `data.csv` (1.906 s) does not match the retained
32x32 report (4.80 s, with tracing on). It may come from a run whose report
was not kept.

## Zeonica testbenches

- The six testbenches are `test/testbench/stonneGEMM{4x4,8x8,16x16,32x32,64x64,128X128}/`
  (committed on this branch as `ad19fef`). Every testbench feeds A from the west edge and B from the north
  edge. Each PE runs `MUL_ADD` and forwards its operands east and south (II = 2),
  as in `gemm.asm`. The host then reads C from each tile's memory word 0 and
  compares it with a CPU GEMM.
- 4x4 has no `arch_spec.yaml` of its own. It resolves
  `test/arch_spec/arch_spec.yaml` from the Zeonica snapshot, as it did
  historically. That spec has changed since the historical runs: for example,
  `49c03db` added `gep_stride`. The 4x4 cycle count is still 15. The
  three-operand GEP and `REM` changes do not affect this kernel, which uses no
  GEP.
- The testbenches seed their random inputs from the clock unless
  `ZEONICA_RAND_SEED` is set. This experiment fixes it to 12345
  (`config/sizes.json`). The cycle count does not depend on the input values.
- The HEAD snapshot reproduces 15/31/63/127/255/511 exactly, with no change to
  Zeonica.

## STONNE configuration

- `config/sizes.json` holds the commands, which are the `Stonne_config` column of
  `data.csv` tokenized with `shlex`. Every size uses
  `-DenseGEMM -M=N -N=N -K=N -ms_rows=N -ms_cols=N -num_ms=N*N
  -T_M=N -T_N=N -T_K=1 -accumulation_buffer=1 -rn_type=TEMPORALRN
  -mn_type=OS_MESH -mem_ctrl=TPU_OS_DENSE`. `dn_bw` and `rn_bw` are 8 for N=4 and 8,
  and N otherwise.
- **4x4 `num_ms`.** `data.csv` prints `-num_ms=16` for 4x4. The surviving
  historical 4x4 statistics file was produced with `num_ms=64`: its
  `MSNetwork.ms_size` is 64, and the file is
  `output_stats_layer_TestLayer_architecture_MSes_64_dnbw_8_rn_bw_8timestamp_1772765188.txt`.
  Both settings give **15 cycles**. The experiment runs and times the `data.csv`
  command (`num_ms=16`). It also runs `num_ms=64` once, untimed
  (`results/stonne_run/extra.csv`), and confirms that this run reproduces the
  historical file byte for byte. The discrepancy does not affect any plotted value.
- For 8x8 to 128x128, the `data.csv` command reproduces the historical
  statistics files byte for byte.

## Reference values

`reference/legacy_data.csv` is the paper's original data file
(sha256 `37f1a24a…a16ce`), from the historical `stonne_compare` sweep
directory. Its last row (100x100, Zeonica only) is
not plotted by the paper's scripts.

`reference/stonne_historical_stats.json` lists the sha256 of the STONNE
statistics files left by the historical sweep. The files themselves are not
shipped because they are up to 47 MB each.

`config/expected.json` holds the values printed in the paper.

## Notes

- `test/testbench/stonneGEMM128X128/gemm.yaml` is 28 MB, and the six
  testbenches total 37 MB.
- Fig. 7d: the published figure is kept. The measured figure uses the same
  boundary (Zeonica `wallClockDurationSec`, STONNE process time).
