# Case Study Text

We use a TT-Metalium-inspired multicore tiled matrix multiplication as an
abstraction case study. The workload follows the kernel-level structure of the
TT-Metalium multicore matmul example: output tiles are partitioned across cores,
reader-like stages provide the A and B data required for each output tile,
compute fires when both operands are available, and writer-like stages drain the
resulting C tiles. In Zeonica, each output tile is represented by one A panel
token and one B panel token, both covering the full K-tile dimension. A single
abstract `TT_MATMUL_TILE_U32` operation consumes the two ready panel tokens and
produces one flattened 32x32 output tile.

The experiment uses M=640, K=640, and N=640 with 32x32 tiles, yielding a
20x20 output-tile grid. The 400 output tiles are statically distributed over a
4x4 Zeonica PE grid, with each PE responsible for 25 contiguous output tiles.
The demo validates functional correctness by reconstructing the full output
matrix and comparing it against a CPU golden result.

This case study is intentionally not a Wormhole hardware simulation. It does
not model Tenstorrent NoC routing, L1 placement, circular-buffer banking,
packer/unpacker behavior, Tensix matrix-engine timing, bf16 arithmetic, tilized
layout, or untilize. Its purpose is narrower: to demonstrate that Zeonica's
data-driven IR can express the producer-consumer dependency structure of a
TT-Metalium-style tiled workload and provide traceable evidence that compute
occurs only after the required data tokens are available.

