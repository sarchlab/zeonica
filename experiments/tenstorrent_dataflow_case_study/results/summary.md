# Tenstorrent-Inspired Case Study Summary

| Field | Value |
| --- | --- |
| Kernel | Multicore tiled matmul |
| Matrix size | M=640, K=640, N=640 |
| Tile size | 32x32 |
| Tile grid | Mt=20, Kt=20, Nt=20 |
| Output tiles | 400 |
| Active Zeonica cores | 16 |
| Output tiles per core | 25 |
| Data type | uint32 |
| Layout | Row-major flattened tiles |
| Correctness check | CPU golden output comparison |
| Current result | mismatch=0 |

## Supported Claim

This case study shows that Zeonica can encode a TT-Metalium-style
reader/compute/writer producer-consumer dependency graph for a full-size tiled
matmul workload at an abstract dataflow level.

## Unsupported Claim

This case study does not show Tenstorrent datapath fidelity, TT-Metalium source
compatibility, Wormhole timing, NoC behavior, circular-buffer microarchitecture,
bf16 behavior, tilized layout behavior, or performance comparability.

