# Design

## Goal

Demonstrate that Zeonica can host a small TT-Metalium-inspired data-driven
producer-consumer kernel pattern as a case study. The design is an abstraction
mapping, not a Wormhole simulator.

## Abstraction Mapping

| TT-Metalium concept | Zeonica abstraction |
| --- | --- |
| Reader kernel | Host feed, memory preload, or data movement operations |
| Compute kernel | Per-PE data-driven instruction groups |
| Writer kernel | Collect or store operations |
| Circular buffer | Bounded FIFO or stream-port approximation |
| Tile availability | Operand readiness and queue readiness |
| NoC transfer | Fixed Zeonica mesh route or abstract data movement |
| DRAM/L1 placement | Local or shared memory setup in Zeonica |
| TT `matmul_tiles` | Abstract `TT_MATMUL_TILE_U32` macro-op |
| Kernel lifecycle evidence | Lightweight tile lifecycle trace summary |

## Execution Interpretation

The case study treats TT-Metalium reader/compute/writer stages as a logical
pipeline. A stage can make progress when its input data is ready and its output
buffer or stream has capacity. Zeonica represents the same class of behavior
through operand readiness, stream ports, local memory, and bounded queues.

## Fidelity Boundary

The mapping preserves functional data dependencies and the high-level
producer-consumer structure. It does not preserve Wormhole-specific timing,
NoC arbitration, torus routing, Tensix internal pipelines, exact circular-buffer
state, or hardware-level resource contention.

## Initial Case Studies

1. `eltwise_add`
   - Purpose: validate reader/compute/writer and circular-buffer-style flow.
   - Expected validation: output equals CPU golden result.

2. `matmul_2x2`
   - Purpose: validate full-size `640x640x640` multicore tiled matmul at
     Zeonica's abstract data-driven level.
   - Expected validation: output matrix equals CPU golden result.
   - Current demo uses `uint32` flattened `32x32` tile tokens and fixed
     16-core output-tile partitioning.
   - Trace evidence records A panel readiness, B panel readiness, abstract
     compute firing, C emission, and C collection for each output tile.

## Evidence Chain

The strengthened case study should be read through four artifacts:

1. `tt_metal_sources/matmul_multicore_source_note.md` records the source
   kernel-level pattern.
2. `lowering_notes/matmul_multicore_dataflow_mapping.md` defines the lowering
   from TT-Metalium-style stages into Zeonica tokens and operations.
3. `results/summary.md` summarizes the functional result and supported claim.
4. `results/matmul_multicore_trace_summary.md`, when generated, records the
   per-output-tile producer-consumer lifecycle.

## Artifact Policy

Do not vendor large external Tenstorrent source trees into this directory.
Keep source references as links, short summaries, or manually extracted
dataflow descriptions. Generated Zeonica artifacts should be added only after
the lowering rules are documented.
