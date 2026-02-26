# CGRA Log Viewer

This viewer visualizes JSONL traces like `gemm.json.log` with a cycle slider and playback.

## Run

From repository root:

```bash
python3 -m http.server 8000
```

Open:

```text
http://localhost:8000/viz/
```

It will try to load `../gemm.json.log` automatically. You can also load any other trace from the file picker.

## Supported events

- `DataFlow` (`FeedIn`, `Send`, `Recv`, `Collect`)
- `Inst` (`DATA_MOV`, `MUL_ADD`, `STORE`)
- `Memory` (`StoreDirect`)
