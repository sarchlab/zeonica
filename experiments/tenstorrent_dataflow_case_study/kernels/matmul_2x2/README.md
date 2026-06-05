# Full-Size Multicore Matmul Kernel

This kernel directory now hosts the first full-size TT-Metalium-inspired
multicore matmul demo artifact.

## Zeonica Program

- `matmul_multicore.yaml`
- Device shape: `4x4`
- Active PEs: 16
- Operation per PE: `TT_MATMUL_TILE_U32 West/R, North/R -> East/R`

## Matrix Shape

- `M = 640`
- `K = 640`
- `N = 640`
- Tile edge: `32`
- `Mt = 20`, `Kt = 20`, `Nt = 20`
- Output tiles: `400`
- Fixed work partition: 25 output tiles per PE

## Data Format

The demo uses deterministic `uint32` input values. It intentionally does not
model TT bf16, tilized layout, untilize, NoC routing, circular-buffer
implementation, or Tensix hardware timing.

## Reproduce

From the repository root:

```bash
go run ./experiments/tenstorrent_dataflow_case_study/matmul_multicore_demo
```

The run passes when it prints `mismatch=0`.
