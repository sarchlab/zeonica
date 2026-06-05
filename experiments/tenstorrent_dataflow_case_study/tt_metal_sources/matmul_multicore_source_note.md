# TT-Metalium Multicore Matmul Source Note

## Reference

- TT-Metalium documentation: https://docs.tenstorrent.com/tt-metal/latest/tt-metalium/tt_metal/examples/matmul_multi_core.html

This note records the kernel-level dataflow pattern used by the case study. It
does not vendor Tenstorrent source code and does not claim source compatibility.

## Extracted Pattern

The reference multicore matmul pattern decomposes the output matrix into output
tiles. Each core receives a contiguous range described by an output tile start
ID and a number of output tiles. For each assigned output tile, the reader side
derives the output tile row and column, streams the corresponding A and B tiles
over the K dimension into input circular buffers, and the compute side consumes
matching A/B tile pairs. The writer side drains one output tile per assigned
output position.

The case study keeps the following logical structure:

| TT-Metalium pattern | Case-study representation |
| --- | --- |
| Output tile partition by start ID and tile count | Static 16-core partition, 25 output tiles per core |
| Reader reads A tiles for one output row across K | Host constructs one A panel token per output tile |
| Reader reads B tiles for one output column across K | Host constructs one B panel token per output tile |
| Input circular buffers hold ready A/B tiles | Zeonica input queues hold A/B panel tokens |
| Compute waits for A/B CB data and performs tile matmul | `TT_MATMUL_TILE_U32` fires when West/North tokens are ready |
| Writer drains output CB to destination tensor | Host collects one East C tile token per output tile |

## Deliberate Abstractions

- The case study packs all K tiles for an output position into one panel token
  per operand. This compresses the TT inner loop into a single Zeonica macro-op.
- Data is `uint32` row-major tile data, not TT bf16 or tilized memory layout.
- NoC reads, L1 placement, circular-buffer banking, pack/unpack, and Tensix
  matrix-engine timing are out of scope.

