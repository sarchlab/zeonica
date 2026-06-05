# Claim Boundary

This case study is meant to support a narrow abstraction claim:

> Zeonica's data-driven IR can represent a small TT-Metalium-inspired
> reader/compute/writer producer-consumer tile pipeline for functional and
> coarse dataflow analysis.

## Acceptable Claims

- Zeonica can express a TT-Metalium-inspired data-driven producer-consumer
  kernel subset.
- The selected kernels can be functionally represented and validated in
  Zeonica.
- Zeonica can expose coarse dataflow behavior such as stream movement, queue
  readiness, and abstract pipeline progress for the lowered kernels.
- The experiment is a portability and abstraction case study.
- The full-size matmul demo matches the official example's matrix dimensions
  and output-tile partition idea at an abstract `uint32` dataflow level.
- The trace summary shows complete logical lifecycle coverage for all 400
  output tiles in the abstract demo.

## Claims To Avoid

- Zeonica is a Wormhole simulator.
- Zeonica is TT-Metalium-compatible.
- Zeonica models Tensix microarchitecture.
- Zeonica models Wormhole NoC, torus routing, DRAM bank timing, or hardware
  arbitration.
- Zeonica produces cycle-accurate Tenstorrent performance.
- Zeonica raw cycles are directly comparable to Wormhole hardware cycles.
- The current matmul demo matches TT bf16, tilized layout, untilize, or PCC
  checking.
- The current trace summary is a hardware timing trace.

## If A Real Tenstorrent Run Is Added Later

Use the Tenstorrent run as a source-kernel correctness reference only. Compare
functional output and logical dataflow structure where possible. Do not use it
to claim hardware timing calibration unless a separate validation methodology
is added.
