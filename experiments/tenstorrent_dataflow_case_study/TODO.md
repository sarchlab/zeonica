# TODO

1. Done: select the TT-Metalium multicore matmul documentation pattern as the
   source structure for the main case study.
2. Done: write a kernel-level dataflow extraction note for multicore matmul.
3. Done: define manual lowering rules from reader/compute/writer and circular
   buffers into Zeonica data-driven IR.
4. Done: lower `eltwise_add` into Zeonica YAML.
5. Done: lower the full-size multicore matmul into Zeonica YAML using
   `TT_MATMUL_TILE_U32`.
6. Done: add CPU golden-output checks for both executable demos.
7. Done: add optional matmul trace-summary generation.
8. Remaining: optionally add a real TT-Metalium run as a correctness-only
   reference if hardware access becomes available.
9. Remaining: turn `CASE_STUDY_TEXT.md` into the final paper section and align
   terminology with the surrounding manuscript.
10. Done: generate figure sources and visual tables from the extracted dataflow
   graph and per-core partition table.
