# CGRA Log Viewer

This viewer has two synchronized views:

- Timeline replay for JSONL traces (cycle slider + playback)
- **Strict Timing Offset View** (program YAML + trace correlation by op ID)

## Run

From repository root:

```bash
python3 -m http.server 8000
```

Open:

```text
http://localhost:8000/viz/
```

The page tries to auto-load:

- `../gemm.json.log` (trace)
- `../gemm.yaml` (program)

If not found, use file pickers manually.

## Inputs

You need both files to get strict timing comparison:

- **Trace log**: JSONL with `Inst` events (`X`, `Y`, `ID`, `Time`)
- **Program YAML**: includes `array_config.compiled_ii`, per-core operations with `id` and `time_step`

Optional aggregate report input:

- **Report JSON**: generated report (for example `fir.report.json`) with `grid`, global counters, per-tile `utilizationPct`, and `topHotTiles`.
- Report can be loaded independently from trace/yaml for quick utilization review.

## Strict Timing Offset View

Layout behavior:

- Top grid uses hybrid adaptation based on detected grid size:
  - If YAML provides `array_config.columns/rows`, mesh size uses YAML array bounds first.
  - If YAML is unavailable, bounds are inferred from trace events.
  - Prefer fitting into current canvas viewport by scaling tile/gap.
  - If tile would become too small, switch to expanded `viewBox` to keep readability.
- Top mesh supports free zoom/pan (wheel zoom + drag pan) for large arrays.
- If report is loaded, mesh adds a utilization heat overlay from `tiles[].utilizationPct` (missing tiles treated as 0).
- Active tiles render per-cycle summary text in-tile:
  - `OP:` instruction opcode summary
  - `MEM:` direct memory behaviors (e.g. `LoadDirect` / `StoreDirect`)
  - `IN:` / `OUT:` data snippets from `FeedIn` / `Collect`
- Bottom timing view uses a timeline axis (`Y=core`, `X=cycle`) + drilldown.

Report view:

- Report panel shows summary cards: `totalCycles`, `activeCyclesGlobal`, `idleCyclesGlobal`, `passed`, `mismatchCount`, `activeTileCount`, `totalEvents`.
- Hot-tile table shows ranked `coord`, `utilizationPct`, `activeCycles`, `totalEvents`.
- If report grid and current mesh grid differ, viewer shows a warning; overlay is clipped to current mesh bounds.

Timing view layout:

- One lane per core `(x,y)` with:
  - upper sub-row blocks: Expected slots
  - lower sub-row blocks: Actual slots (all samples in full trace)
- Timeline blocks are expanded by **all actual samples across full trace length** (not just first occurrence).
- For each actual sample occurrence, expected block is back-computed and aligned by that sample's delta.
- Fractional `Time` values are rounded with `Math.round` before slot/time comparison and rendering.
- `baseline-view` supports:
  - `strict`: strict baseline only
  - `compensated`: compensated baseline only
  - `split`: strict + compensated side-by-side rows for comparison
- Mismatch blocks/links are drawn as rectangles (not points) for slot-level readability
- Drilldown panel still shows operation-level details for selected `(core, slot)`
- `window-start` + `window-size` let you pan/zoom through full trace cycles

Default view (hybrid as main):

- **Default** is `baseline-view=compensated` and `comp-model=hybrid`. The timeline and anomaly filter use **hybrid** status only, so you focus on "mid-trace" offsets after subtracting expected propagation delay; strict remains in summary and drilldown for reference.
- Use `strict` or `split` only when you want to double-check raw schedule vs trace or debug compiler/schedule issues.

Default interaction:

- `anomaly-only` is disabled by default; when in compensated view it filters by **hybrid** status (not strict).
- `show-phase-explain` is enabled by default to expose per-core phase offsets
- `boundary-only` can focus edge PEs to verify boundary shift patterns quickly
- **Jump to first hybrid mismatch** button moves the time window to the first cycle where any op is a hybrid mismatch.
- `Ctrl + mouse wheel` zooms timeline quickly (X/Y together). Zoom anchor follows mouse position on X-axis to reduce view jump.
- `y-zoom` slider adjusts lane height/readability; `Reset Zoom` restores default zoom and window.
- `comp-model` supports:
  - `distance-heuristic`: infer propagation delay from core-to-ingress distance
  - `trace-fitted-phase`: use per-core fitted phase (`modeDelta`)
  - `hybrid`: prefer fitted when confidence is high, otherwise fall back to distance (default)
- Click a timeline block/link to inspect operation-level details in drilldown
- Drilldown now includes sample source fields so each match can be traced back to `Inst` / `LoadDirect` / `StoreDirect`.
- Core focus supports two synced entry points: click Y-axis core label, or select from `core-focus` dropdown.
- When a core is focused, the main timeline keeps only that core and an inline mini panel shows source distribution plus a compact in-window trace list.
- `Export PNG` downloads the current timeline window
- `max-side` controls export scaling upper bound; oversized windows are proportionally downscaled
- For repeated op executions, timeline labels/tooltips include occurrence tag (e.g. `@2` or `[2/5]`).

Status semantics:

- **Strict baseline (truth reference, unchanged):**
  - `on-time`: `actualSlot == expectedSlot`
  - `early`: `actualSlot < expectedSlot` (signed modular delta)
  - `late`: `actualSlot > expectedSlot`
  - `missing`: operation exists in YAML but no `Inst` with same `(x,y,id)` in trace
- **Compensated baseline (explanation layer):**
  - strict delta is rebased by per-core compensation offset
  - used to reduce global boundary propagation shift false-positives
  - never replaces strict verdict; always shown as secondary comparison

Phase explanation layer (additive, does not change strict status):

- `Δcore`: dominant per-core phase offset inferred from mismatch mode (`modeDelta`)
- `conf`: confidence of that offset from mismatch concentration
- `phase(boundary, inner, gap)`: weighted-median phase summary comparing boundary vs inner cores
- `deltaRebased`: per-op delta after subtracting `Δcore` (for separating global shift from local residual anomalies)

Shift-aware annotations:

- `first-divergence`: first mismatch point or delta-change point in a core
- `propagated`: same-delta continuation after divergence (faded style)

Drilldown fields:

- `opId`, `opcode`
- `expectedSlot`, `actualSlot`
- `deltaStrict`
- `deltaComp(<model>)`
- `statusStrict` / `statusComp`
- `deltaPhaseRebased`
- `firstTime`
- `samples`
- `sourceSummary` (for example `Inst*10,LoadDirect*2`)
- `firstDivergence`
- `samplePreview` with source tags (for example `1:210:Inst,2:213:LoadDirect`)

Recommended read path:

1. Use default **compensated + hybrid** view to see whether there are any mid-trace offsets (hybrid mismatch). Use "Jump to first hybrid mismatch" to focus the window on the first such cycle.
2. In drilldown, read `statusComp` / `deltaComp(hybrid)` first; treat `statusStrict` / `deltaStrict` as reference only for double-check.
3. If you need to verify raw schedule vs trace, switch to `strict` or `split` and compare; strict is the truth reference for pass/fail.

## Supported event families (timeline view)

- `DataFlow` (`FeedIn`, `Send`, `Recv`, `Collect`)
- `Inst` (generic instruction events)
- `Memory` (e.g., `StoreDirect`)
