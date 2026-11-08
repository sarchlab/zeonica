# E3: TT-Metalium/Wormhole model vs. TT-Sim (Fig. 6, Table II)

This experiment reproduces Fig. 6 and the cycle columns of Table II. Zeonica's
Tenstorrent-style model runs 10 held-out TT-Metal gtests, and its cycle counts
are compared with TT-Sim device-side cycles.

| | Paper | This artifact |
|---|---|---|
| MAPE over the 10 held-out kernels | 3.28% | **3.2802%** |
| Max APE | 9.15% (3x3 matmul) | **9.15%** (3x3 matmul) |

The Zeonica side is rebuilt from this repository (branch
`hpca27-ae/e3-tenstorrent`) and rerun with the frozen, calibrated profile
(`config/tt_eir_freeze4.yaml`).

**TT-Sim is not rerun.** Its cycle counts, and the tensor images the model
consumes, are shipped as reference data in `reference/`. Their provenance is
described in [TT-Sim reference data](#tt-sim-reference-data).

## Requirements

- Go >= 1.24, tested with go1.25.3 linux/amd64. The first build downloads
  the Go modules in the repository's `go.sum`, for example
  `github.com/sarchlab/akita/v4`. This needs network access or a populated
  module cache.
- `git`, to snapshot this repository at HEAD. Run from a git clone of the
  `hpca27-ae/e3-tenstorrent` branch, not from an exported tarball.
- Python 3, tested with 3.13.
  - Only `scripts/plot_fig6.py` needs third-party packages: `matplotlib` and
    `numpy`.
  - Every other script uses only the standard library. PyYAML is not needed,
    because the Go binary reads the YAML profile itself.

## Run

```bash
./run_all.sh              # build + run + evaluate + Table II + Fig. 6
./run_all.sh --skip-run   # reuse results/zeonica_run; re-evaluate and redraw only
```

Optional environment variables: `PYTHON` (default `python3`) and `JOBS`
(parallel kernel runs, default 8).

Measured runtimes on a 128-core x86-64 host:

| Command | Wall time | Breakdown |
|---|---:|---|
| `./run_all.sh` | 39 s | ~35 s Go build with an empty `GOCACHE` (module cache already populated); 2.0 s for the 10 model runs |
| `./run_all.sh --skip-run` | 1.2 s | |

`run_all.sh` prints `run_all: PASS` and exits 0 only if every check below
passes.

## What happens

1. **`scripts/check_freeze.py`** checks that the inputs are the frozen ones
   (`results/freeze_check.json`). See [Calibration, freeze and held-out discipline](#calibration-freeze-and-held-out-discipline).
   - `config/tt_eir_freeze4.yaml` and the 59 source files of the frozen
     tree, read from this repository at git HEAD, must match the freeze-4 hash
     record `config/pre_validation_freeze4.json`. That record was written by
     the original pipeline before the held-out runs.
   - The record's 60th entry, `UPSTREAM_COMMIT`, names the upstream commit
     the frozen tree was cut from (`5820166`). It must be an ancestor of HEAD.
   - No Go file may exist in the recorded package directories that the
     record does not list, since it would change the build.
   - Every TT-Sim reference cycle count must equal the recorded dump-off gtest
     run, both its `result.json` and the TT-Sim exit line in `stdout.log`.
2. **`scripts/run_zeonica.py`** builds and runs the model.
   - It runs `git archive HEAD` of this repository, without `artifact/`, into
     `results/zeonica_run/src`, so uncommitted edits are never used.
   - It builds with `go build -buildvcs=false -trimpath ./experiments/tt_eir`.
   - It runs the 10 kernels of `config/kernels.json` with all `ZEONICA_*`
     environment variables removed:

     ```
     tt_eir -kernel K -config config/tt_eir_freeze4.yaml \
            -data-dir reference/ttsim_gtest_dump/K/dump \
            -summary-json ... -output-u32 ...
     ```

   - Each run feeds TT-Sim's input DRAM images (`in0`, `in1`) through the model.
     It then compares the output DRAM image with TT-Sim's readback (`out.u32`):
     - matmul, bcast and eltwise must match bit for bit;
     - SFPU exp, sigmoid and tanh use the gtest's own tolerance. About 50% of
       their elements are bit-exact; the rest are within 1 ULP.

     A failed data check fails the run.
   - Outputs go to `results/zeonica_run/{results.csv,provenance.json,kernels/<k>/}`.
3. **`scripts/evaluate.py`** computes APE = |Zeonica − TT-Sim| / TT-Sim and the
   MAPE. It writes `results/fig6_data.csv` and `results/summary.json`, with
   these PASS/FAIL checks:
   - `mape`: the MAPE rounds to 3.28%, and to 4 decimals it equals the
     freeze-4 value 3.2802%.
   - `per_kernel_ape`: each kernel's APE, rounded to 2 decimals, equals the
     value printed in the paper: 0.05, 7.29, 0.93, 9.15, 3.61, 8.85, 0.03, 0.45,
     0.61 and 1.83%.
   - `max_ape`: the largest APE is 9.15%, for the 3x3 matmul.
   - `data`: all 10 output-data checks passed.
   - `freeze4`: the rebuilt model reproduces the original freeze-4 run exactly
     for every kernel. This covers:
     - the Zeonica cycle count (`reference/freeze4/kernels.csv`);
     - every summary field: E-IR cycles, fixed cycles and their components,
       PEs, instructions, CB queues and the data check
       (`reference/freeze4/summaries/`);
     - the sha256 of the output DRAM image (`reference/freeze4/zeonica_out.sha256`).
4. **`scripts/make_table2.py`** writes `results/table2.csv`. See [Table II](#table-ii).
5. **`scripts/plot_fig6.py`** draws `figures/fig6_tenstorrent.{pdf,png}`.
   - Horizontal grouped bars on a log x axis, with the per-kernel APE at the
     end of each bar and `MAPE = 3.28%` in the corner.
   - Colors are Zeonica #1D9E75 and TT-Sim #BA7517.
   - The rcParams (serif fonts) are taken from the paper's
     `plot_rtl_comparison.py`, the Fig. 4 plotting script.

## Results

| Kernel (Fig. 6) | gtest key | TT-Sim | Zeonica | APE |
|---|---|---:|---:|---:|
| BFP8 matmul | `matmul_single_tile_bfp8b` | 4,413 | 4,411 | 0.05% |
| single-core matmul | `matmul_single_core_small` | 5,568 | 5,974 | 7.29% |
| HiFi4 matmul | `matmul_large_block_hifi4_rowmajor_rowmajor` | 14,889 | 15,027 | 0.93% |
| 3x3 matmul (9 cores) | `matmul_multicore_multi_dram_small_3x3` | 35,177 | 38,397 | 9.15% |
| bcast W-mul | `bcast_w_mul` | 5,403 | 5,208 | 3.61% |
| eltwise mul | `eltwise_binary_mul` | 393,369 | 428,183 | 8.85% |
| SFPU exp-1 | `sfpu_exponential_1tile` | 12,525 | 12,521 | 0.03% |
| SFPU sigmoid-1 | `sfpu_sigmoid_1tile` | 12,465 | 12,521 | 0.45% |
| SFPU sigmoid-4 | `sfpu_sigmoid_4tile` | 12,531 | 12,608 | 0.61% |
| SFPU tanh-4 | `sfpu_tanh_4tile` | 12,843 | 12,608 | 1.83% |
| **MAPE** | | | | **3.28%** |

Each Zeonica cycle count is the sum of two parts, listed separately in
`results/fig6_data.csv`:

- **E-IR cycles:** the simulated time until the last output page leaves the
  writer PE.
- **Fixed cycles:** calibrated constants for the TT-Sim time outside the
  kernel pipeline:
  - `dispatch` = 4,237 cycles;
  - `sfpu_dispatch` = 12,483 cycles;
  - `large_block_hifi4` = 8,486 cycles;
  - a multi-core term for the 3x3 grid.

For the SFPU kernels the fixed part dominates: E-IR cycles are only 38 to 125
of about 12,500. The TT-Sim reference is a whole-process clock (see below), so
it includes this fixed part too.

## Table II

`results/table2.csv` is generated by `make_table2.py` from three sources. The
TT-Sim columns are reference data; the model columns come from the model
itself.

- **TT-Sim configuration:** shape, tile geometry, output tiles, data type,
  core grid and circular-buffer (CB) capacities in tiles, from
  `reference/ttsim_configuration_cb.csv`.
  - This file is a column subset of the evaluation deliverable
    `tt_sim_configuration.csv`, which was read from the tt-metal gtest sources.
  - That deliverable's cycle columns belong to an older model round and are
    dropped.
- **Model configuration:** the CB capacities, core count and output pages
  that the frozen model actually configures.
  - `scripts/table2_caps.go.txt` is compiled into a scratch copy of the built
    snapshot. The repository itself is not modified.
  - It builds each kernel's E-IR program through the runner's own
    `lookup()`/`build()` path and prints the receive-queue capacities that
    carry a CB name (`results/table2_model_config.json`).
- **Cycles and APE:** from `results/fig6_data.csv`.

The core count and the number of output tiles agree for all 10 kernels. CB
capacities agree on every CB both sides define, with one exception.
`make_table2.py` reports it, and it is a property of the model, not of this
packaging:

- **HiFi4 matmul** (row-major in and out, M_t=4, N_t=2):
  - The gtest allocates c25 (partials) = 8 and c27 (reblock) = 2. The model has
    no queue for either.
  - It allocates c26 (final partials) = 8. The model sizes c26 at
    SubM x N_t = 4 tiles.
- The model also has a single-tile c24 queue for the BFP8 single-tile matmul.
  The gtest does not allocate this CB.

The paper's Table II itself was not available to this packaging; the paper
PDF in the workspace is an older draft. The generated CSV was therefore
checked against the deliverable CSV and the model source, not against the
typeset table.

## What was changed in Zeonica

This branch starts from upstream Zeonica `5820166` ("Skip FFT test when
submodule artifact is missing"), the commit the TT work was built on, and
adds two commits before the artifact:

| Commit | Content |
|---|---|
| `2be7fa3` Add REM opcode | The REM opcode in `core/emu.go` and `core/opcodes.go`. It was uncommitted in the working tree the model was frozen on (2026-10-05); the same opcode later landed as `67c6090` on the OpenCGRA-comparison line. |
| `70eb547` Add TT-Metalium/Wormhole model on E-IR | The TT/Wormhole changes only. |

```bash
git diff --stat 2be7fa3 70eb547   # 16 files changed, 2569 insertions(+), 6 deletions(-)
git diff 2be7fa3 70eb547          # the full TT change
```

- **Hooks in existing upstream files** (+65/−6 lines):
  - `core/emu.go`, `core/core.go`, `core/builder.go`:
    - per-(color, direction) receive-queue capacities, so a CB is a port's FIFO;
    - indexed queue reads (`West@i`, read tile i without popping);
    - CB issue readiness;
    - progress-credit ports;
    - a shared `ttRunState`. Core evaluates readiness on `cloneForEval()`
      copies, so this state must live behind a pointer.
  - `core/opcodes.go` registers the TT tile opcodes.
  - `config/platform.go` adds a `CoreOf` helper.
  - `api/driver.go` fixes data collection:
    - the driver polls a host-drained queue, because a core filling it does
      not wake the driver;
    - the driver wakes the core after draining;
    - an optional `ZEONICA_DATA_COLLECT_TIMEOUT_CYCLES` stall guard.
- **New model code:**
  - `core/tt_cb.go`: `CB_WAIT`, `CB_POP`, credit-based `CB_RESERVE`, and the
    progress-work/credit operations.
  - `core/tt_tile_ops.go`: DRAM read/write, unpack/pack (bf16/bfp8), reload,
    zero, matmul, tilize/untilize, eltwise/bcast and SFPU.
  - `core/tt_numerics.go`, `core/tt_kernel_data.go`, `core/tt_bcast.go`:
    Wormhole numerics ported from tt-sim `src/tensix.cpp`, bit-exact for the
    FPU paths.
- **New experiment program** `experiments/tt_eir/`:
  - `gen.go`, `kernels.go`, `specs.go`: kernel to E-IR generator, with the
    gtest shapes, CB sizes and DRAM page orders.
  - `main.go`: the runner. The E-IR YAML goes through
    `LoadProgramFileFromYAML`, then `Driver.MapProgram`, then Core/Akita;
    opcode timing comes from the operation-latency sidecar.
  - `state.go`: the internal-state logger, not used here.

**Identity with the frozen tree.** The model was frozen and evaluated on a
pruned copy of the upstream tree (`zeonica_tt_eir_20261004/pruned/`): the
packages `api`, `cgra`, `config`, `core`, `report`, `runtimecfg`,
`experiments/tt_eir`, `go.mod` and `go.sum`, plus an `UPSTREAM_COMMIT` file.
Every one of those files is byte-identical to the same path at `70eb547`.
The branch keeps the rest of the upstream tree (`test/`, `cmd/`, `docs/`, ...);
none of it is built into `experiments/tt_eir`. This is rechecked on every run
against the freeze-4 hash record. The original binary's hash (`2a2e5421…`) is
not reproduced, because a rebuild embeds different paths and toolchain data.
Cycles, every summary field and the output images are reproduced exactly; see
the `freeze4` check above.

## Calibration, freeze and held-out discipline

**Partition.** The 28 TT-Metal gtests were split into 18 calibration and 10
held-out kernels in `reference/kernel_partition.json`. That file is dated
2026-10-04 10:41 UTC, before the first freeze of this model. Case B (a
TT-style resource sweep, not part of this artifact) adds 7 calibration and 22
validation conditions.

**Calibration inputs.** Only calibration data from TT-Sim was used:

- the 18 calibration gtest end points;
- for the 7 Case B calibration conditions, the core-0 CB block events and the
  probe window deltas.

**Pipeline** (original `scripts/pipeline.sh`):

1. `fit_caseb.py` (evolution strategy plus coordinate refinement);
2. `calibrate_rest.py`;
3. `freeze.py`, which hashes the profile, binary and sources;
4. only then, the held-out and validation runs.

The resulting freeze-4 profile is `config/tt_eir_freeze4.yaml`. Its hash
record, `config/pre_validation_freeze4.json`, was created at
2026-10-05T03:35:22Z. This artifact checks against that record on every run.

**Not included.** The calibration step itself is not part of this artifact. It
needs the Case B TT-Sim observer data (CB event traces from a separately
patched TT-Sim build), the calibration-kernel tensor images (about 35 MB more),
and a stochastic search. The frozen result and its hash record are shipped
instead.

**Freeze history.** The original report, `zeonica_tt_eir_20261004/REPORT.zh.md`
§4, lists five freezes:

| Freeze | Change | Held-out MAPE | Case B validation passed |
|---|---|---:|---:|
| 1 | Fixed latencies only | 6.13% | 12/22 |
| 2 | Adds credit CBs and progress credits | 6.06% | 14/22 |
| 3 | Adds per-subblock structure taken from the TT kernel sources | 3.28% | 21/22 |
| **4** | Adds zero-init folded into the unpack group | **3.28%** | **21/22** |
| 5 | Adds a steady-state subblock-interval objective | 3.34% | 20/22 |

**Disclosure** (from that report):

- The structural changes after freeze 1 were made after seeing the previous
  freeze's validation and internal-state results.
- Freeze 4 was chosen over freeze 5 after seeing both.
- Held-out kernels were never used as a fitting target or for a structural
  decision. For example, the single-core improvement came from a per-subblock
  structure derived from the residuals of two calibration kernels,
  `matmul_large_block` and `matmul_single_core`.
- Held-out results were computed for every freeze, so the 10 kernels are
  "held out from fitting", not blind across freezes.

For context, the 18 calibration kernels score 0.98% MAPE (max 9.83%) under the
same profile (`reference/freeze4/evaluation.json`). This artifact does not
rerun them.

## TT-Sim reference data

**What the reference is.** Each reference is TT-Sim's process-global clock
(`g_clock`) at simulator exit. TT-Sim prints it as `[<cycles>] <s> seconds (<f> KHz)`
(`src/sim.cpp`).

- It counts every simulated device cycle of the gtest process: device setup,
  program dispatch, kernel execution and readback.
- It is not a kernel-only window. The model's fixed terms cover the parts
  outside the kernel pipeline.

**Software versions.** The TT-Sim runs used:

- **TT-Sim v1.8.4**, commit `d351ea790333c20536680418b231b6a8284067ce`
  ("Public source drop for v1.8.4 release"), `wh-cycle` build (library sha256
  `9a667f9b…`). Local patch `reference/provenance/ttsim_diff.txt` adds a
  read-only `libttsim_get_clock()`, which does not advance simulation time. The
  reference values here do not depend on it, since they come from TT-Sim's own
  exit line.
- **tt-metal** commit `5922d48d50e29fffb37ee2f687fb764400314cdb`, with the local
  diff `reference/provenance/tt-metal_diff.txt`:
  - new gtest variants (`tt-metal_diff_before.txt`): the HiFi4 large-block
    variants and the sized multi-core "small" grids, including 3x3;
  - an environment-guarded tensor dump hook (`tt-metal_dump_hook.patch.diff`
    plus the untracked `zeonica_dump.hpp`). It is a no-op unless
    `ZEONICA_TT_DUMP_DIR` is set, and dumps after readback.
  - `tt-metal_status.txt` lists the tree state; `provenance.json` records the
    gtest binary hashes.

**How the gtests were run.** `reference/provenance/run_gtest_reference.py` is
the original runner, shipped as a record and not run here.

- Environment: `TT_METAL_SIMULATOR=<wh-cycle libttsim.so>`,
  `TT_METAL_DISABLE_SFPLOADMACRO=1`, and `TT_METAL_SLOW_DISPATCH_MODE=1` for the
  slow-dispatch tests.
- The gtest names, source files and binaries are listed in `provenance.json`
  (`reference_cycles.gtests`) and `reference/kernel_partition.json`.

**Dump-off cycles, dump-on data.**

- Cycle references come from dump-off runs: `reference/ttsim_gtest_nodump/<k>/`,
  plus all repeat observations in `reference/ttsim_reference_cycles.json`.
  The held-out kernels have 1 to 9 observations each, all identical.
- Tensor images come from dump-on runs: `reference/ttsim_gtest_dump/<k>/dump/`.
  The model reads `in0.u32`, `in1.u32` and `out.u32`. The gtest's host
  `golden.*` files are not used and are omitted.
- In fast-dispatch mode a gtest's end point includes host completion-queue
  polling. As a result, the dump-on `sfpu_sigmoid_4tile` run ended at 12,613
  cycles, against 12,531 in all 9 dump-off runs. `check_freeze.py` reports this
  as informational.

## Layout and sizes

```
run_all.sh  README.md  provenance.json  .gitignore
config/      kernels.json (kernels, labels, paper APEs), tt_eir_freeze4.yaml, pre_validation_freeze4.json
reference/   ttsim_reference_cycles.json, kernel_partition.json, ttsim_configuration_cb.csv,
             ttsim_gtest_nodump/<k>/, ttsim_gtest_dump/<k>/{...,dump/},       (15 MB, 14 MB of it tensor images)
             freeze4/ (original freeze-4 kernels.csv, evaluation.json, summaries, output hashes),
             provenance/ (TT-Sim / tt-metal commits, patches, status, original runner)
scripts/     check_freeze.py run_zeonica.py evaluate.py make_table2.py table2_caps.go.txt plot_fig6.py
results/     freeze_check.json, zeonica_run/ (results.csv, provenance.json, kernels/<k>/),
             fig6_data.csv, summary.json, table2.csv, table2_model_config.json   (5 MB, mostly output images)
figures/     fig6_tenstorrent.{pdf,png}
```

The total is about 20 MB without build products. `results/zeonica_run/{src,bin,table2_src}`
are regenerated on every run and are git-ignored; with them, about 45 MB.

## Out of scope

- Table IV (internal state).
- LOC counts.
- Case study B figures and the Case B validation.
- The later NoC/DRAM/dispatch round (freeze 6), which was not used in the paper.
