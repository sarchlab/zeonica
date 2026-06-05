# Matmul Multicore Trace Summary

This lightweight summary records the logical data-driven lifecycle of each output tile in the TT-Metalium-inspired Zeonica demo. It is not a cycle trace and does not model Wormhole timing.

| Field | Value |
| --- | --- |
| M/K/N | 640/640/640 |
| Tile edge | 32 |
| Mt/Kt/Nt | 20/20/20 |
| Output tiles | 400 |
| Active cores | 16 |
| Tiles per core | 25 |
| Data type | uint32 |
| CPU golden mismatches | 0 |

## Lifecycle Coverage

| Tile records | A ready | B ready | Compute after ready | C emitted | C collected | Complete |
| --- | --- | --- | --- | --- | --- | --- |
| 400 | 400 | 400 | 400 | 400 | 400 | 400 |

## Per-Core Tile Assignment

| Core ID | Coord | Tile start | Tile count |
| --- | --- | --- | --- |
| 0 | (0,0) | 0 | 25 |
| 1 | (1,0) | 25 | 25 |
| 2 | (2,0) | 50 | 25 |
| 3 | (3,0) | 75 | 25 |
| 4 | (0,1) | 100 | 25 |
| 5 | (1,1) | 125 | 25 |
| 6 | (2,1) | 150 | 25 |
| 7 | (3,1) | 175 | 25 |
| 8 | (0,2) | 200 | 25 |
| 9 | (1,2) | 225 | 25 |
| 10 | (2,2) | 250 | 25 |
| 11 | (3,2) | 275 | 25 |
| 12 | (0,3) | 300 | 25 |
| 13 | (1,3) | 325 | 25 |
| 14 | (2,3) | 350 | 25 |
| 15 | (3,3) | 375 | 25 |

## Tile Lifecycle Sample

| Tile ID | Core | Out tile | A ready | B ready | Compute after ready | C emitted | C collected | C lanes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | 0 (0,0) | (0,0) | true | true | true | true | true | 1024 |
| 1 | 0 (0,0) | (0,1) | true | true | true | true | true | 1024 |
| 2 | 0 (0,0) | (0,2) | true | true | true | true | true | 1024 |
| 3 | 0 (0,0) | (0,3) | true | true | true | true | true | 1024 |
| 4 | 0 (0,0) | (0,4) | true | true | true | true | true | 1024 |
| 5 | 0 (0,0) | (0,5) | true | true | true | true | true | 1024 |
| 6 | 0 (0,0) | (0,6) | true | true | true | true | true | 1024 |
| 7 | 0 (0,0) | (0,7) | true | true | true | true | true | 1024 |
| 8 | 0 (0,0) | (0,8) | true | true | true | true | true | 1024 |
| 9 | 0 (0,0) | (0,9) | true | true | true | true | true | 1024 |
| 10 | 0 (0,0) | (0,10) | true | true | true | true | true | 1024 |
| 11 | 0 (0,0) | (0,11) | true | true | true | true | true | 1024 |
| 12 | 0 (0,0) | (0,12) | true | true | true | true | true | 1024 |
| 13 | 0 (0,0) | (0,13) | true | true | true | true | true | 1024 |
| 14 | 0 (0,0) | (0,14) | true | true | true | true | true | 1024 |
| 15 | 0 (0,0) | (0,15) | true | true | true | true | true | 1024 |

## Interpretation

Every output tile has one A panel token and one B panel token before the abstract compute stage. The Zeonica operation fires only when both operand queues provide data, then emits one 32x32 C tile that is collected by the harness. This supports a kernel-level producer-consumer abstraction claim, not a Tenstorrent datapath or timing claim.
