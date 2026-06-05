# Matmul Multicore Dataflow Mapping

## Purpose

This document defines the manual lowering boundary for the full-size
TT-Metalium-inspired matmul demo. The goal is to show that Zeonica can encode
the kernel-level data-driven producer-consumer graph, not the Tenstorrent
hardware datapath.

## Lowered Dataflow Graph

```text
A tensor source
  -> reader-like A panel construction
  -> bounded Zeonica West input token queue
  -> TT_MATMUL_TILE_U32
  -> Zeonica East output token queue
  -> writer-like host collect

B tensor source
  -> reader-like B panel construction
  -> bounded Zeonica North input token queue
  -> TT_MATMUL_TILE_U32
```

## Mapping Rules

| Source concept | Zeonica lowering | Evidence artifact |
| --- | --- | --- |
| Reader kernel | Harness constructs A/B panel tokens and feeds a target PE | Demo feed calls and trace summary |
| Circular buffer input | Bounded PE input queue and operand readiness | `A ready`, `B ready`, and compute lifecycle fields |
| `matmul_tiles` K loop | `TT_MATMUL_TILE_U32` macro-op over full K panel | Core unit tests and CPU golden comparison |
| Writer kernel | Harness drains East output token | Trace summary `C collected` field |
| Output tile assignment | `coreID * tilesPerCore` contiguous ranges | Per-core tile assignment table |

## Non-Mapped Hardware Behavior

The lowering does not model NoC routes, torus behavior, L1 address placement,
CB banking, CB credit details, unpacker, math engine, packer, bf16 precision,
tilized layout, untilize, PCC checking, or Wormhole cycle timing.

## Claim Supported

The experiment supports this claim:

> Zeonica can represent the data-driven producer-consumer dependency structure
> of a TT-Metalium-style tiled multicore matmul at the kernel abstraction level.

It does not support a Tenstorrent datapath-fidelity or performance claim.

