# Eltwise Add Kernel

This is the smallest TT-Metalium-inspired data-driven producer-consumer demo in
this case study.

## Zeonica Program

- `eltwise_add.yaml`
- Device shape: `1x1`
- Active PE: `(0,0)`
- Operation: `ADD West/R, North/R -> East/R`

## TT-Metalium Analogy

- `West` feed: reader kernel producing tiles into an input circular buffer.
- `North` feed: second reader kernel producing tiles into another input
  circular buffer.
- `ADD`: compute stage that can fire only after both input streams are ready.
- `East` collect: writer stage draining the output stream.

## Demo Inputs

- `a = [1, 2, 3]`
- `b = [10, 20, 30]`
- expected output: `[11, 22, 33]`

## Reproduce

From the repository root:

```bash
go run ./experiments/tenstorrent_dataflow_case_study/eltwise_add_demo
```

The run passes when it prints `mismatch=0`.

## Limitation

This demo validates functional data readiness and a producer-consumer stream
shape. It does not model exact TT circular buffers, Wormhole NoC or torus
routing, Tensix microarchitecture, or hardware timing.
