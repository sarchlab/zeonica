# Tenstorrent Data-Driven Case Study

This directory is a planning and artifact home for a TT-Metalium-inspired
data-driven producer-consumer case study in Zeonica.

The study asks whether Zeonica's data-driven IR can represent a small,
representative subset of the TT-Metalium execution pattern: reader kernels
produce tiles into circular buffers, compute kernels consume ready tiles, and
writer kernels drain output tiles. The first implementation targets functional
representation and coarse dataflow behavior, not Wormhole hardware fidelity.

## Scope

In scope:

- Manual extraction of a small TT-Metalium-style reader/compute/writer dataflow.
- Manual or semi-manual lowering into Zeonica per-PE programs.
- Functional validation against a CPU golden result.
- Zeonica trace/report collection for dataflow-level behavior.

Out of scope:

- Wormhole NoC or torus routing fidelity.
- Tensix microarchitecture, packer/unpacker, or matrix engine timing fidelity.
- Exact circular-buffer implementation.
- TT-Metalium C++ compatibility.
- Cycle-accurate Tenstorrent or Wormhole evaluation.

## Initial Kernels

- `eltwise_add`: a minimal producer-consumer kernel to validate reader,
  compute, writer, and buffer abstractions.
- `matmul_2x2`: now hosts the full-size `640x640x640` multicore tiled matmul
  demo at an abstract `uint32` dataflow level.

## Directory Layout

- `tt_metal_sources/`: source-kernel notes, links, and short summaries.
- `lowering_notes/`: manual lowering rules and per-kernel mapping notes.
- `kernels/eltwise_add/`: future Zeonica artifacts for the eltwise case.
- `kernels/matmul_2x2/`: future Zeonica artifacts for the matmul case.
- `configs/`: future architecture and runtime configuration files.
- `results/`: future reports, traces, and summary tables.
- `figures/`: future paper figures.

## Current Status

This directory now contains:

- a minimal `eltwise_add` demo;
- a full-size TT-Metalium-inspired multicore matmul demo;
- source extraction, lowering, result, and claim-boundary documents.

The matmul demo matches the official-style matrix dimensions and output-tile
partition pattern at the dataflow abstraction level. It is not a Wormhole
hardware simulator and does not provide Tenstorrent timing or datapath fidelity.

## Reproducing The Main Demo

```bash
go run ./experiments/tenstorrent_dataflow_case_study/matmul_multicore_demo \
  --trace-summary experiments/tenstorrent_dataflow_case_study/results/matmul_multicore_trace_summary.md
```

Expected summary fields:

- `M=640 K=640 N=640`
- `output_tiles=400`
- `active_cores=16`
- `mismatch=0`
