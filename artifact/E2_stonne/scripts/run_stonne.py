#!/usr/bin/env python3
"""Build STONNE from a clean snapshot and run the six Fig. 5 GEMM configurations.

The STONNE checkout is never modified: `git archive HEAD stonne/` is unpacked
into --out/src and built there with the unmodified upstream Makefile.

Each size runs the exact command printed in the Stonne_config column of
reference/legacy_data.csv (checked against config/sizes.json). STONNE writes
its statistics files into $OUTPUT_DIR; they contain no timestamp, so their
sha256 is compared with the files left by the historical sweep
(reference/stonne_historical_stats.json).

Timing boundary: external monotonic elapsed time (time.monotonic) around the
prebuilt STONNE process, the same boundary run_zeonica.py uses. STONNE has no
tracing; its default statistics output (print_stats) stays on, as in the
historical runs.

Outputs (under --out):
  sizes/<label>/stdout.log             stdout of the last run
  sizes/<label>/output_stats_*         statistics files of the last run
  runs.csv, summary.csv, provenance.json
"""

import argparse
import csv
import hashlib
import json
import os
import platform
import re
import shlex
import shutil
import statistics
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent


def sh(cmd, cwd=None):
    return subprocess.run(cmd, cwd=cwd, check=True, text=True, stdout=subprocess.PIPE,
                          stderr=subprocess.STDOUT).stdout


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()


def legacy_commands() -> dict:
    with (ROOT / "reference" / "legacy_data.csv").open(newline="") as f:
        rows = list(csv.reader(f))
    return {r[0]: shlex.split(r[5]) for r in rows[1:] if len(r) > 5 and r[5] != "N/A"}


def build(stonne: Path, src: Path, jobs: int) -> dict:
    if src.exists():
        shutil.rmtree(src)
    src.mkdir(parents=True)
    prov = {
        "path": str(stonne),
        "commit": sh(["git", "-C", str(stonne), "rev-parse", "HEAD"]).strip(),
        "dirty_tracked": bool(sh(["git", "-C", str(stonne), "status", "--porcelain",
                                  "--untracked-files=no"]).strip()),
    }
    if prov["dirty_tracked"]:
        print(f"WARNING: STONNE checkout has uncommitted changes; the snapshot uses its "
              f"HEAD ({prov['commit'][:10]}) only.")
    subprocess.run(f"git -C '{stonne}' archive HEAD stonne | tar -x -C '{src}'",
                   shell=True, check=True)
    bdir = src / "stonne"
    log = sh(["make", f"-j{jobs}", "all"], cwd=bdir)
    (src.parent / "build.log").write_text(log)
    binary = bdir / "stonne"
    if not binary.exists():
        sys.exit("STONNE build failed; see build.log")
    prov["compiler"] = sh(["g++", "--version"]).splitlines()[0]
    prov["makefile_sha256"] = sha256(bdir / "Makefile")
    prov["binary"] = str(binary)
    prov["binary_sha256"] = sha256(binary)
    return prov


def run_once(binary: Path, args: list, outdir: Path, label: str):
    for f in outdir.glob("output_stats_*"):
        f.unlink()
    env = dict(os.environ, OUTPUT_DIR=str(outdir))
    t0 = time.monotonic()
    proc = subprocess.run([str(binary), *args], cwd=binary.parent, env=env, text=True,
                          stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    elapsed = time.monotonic() - t0
    (outdir / "stdout.log").write_text(proc.stdout)
    cyc = re.findall(r"Number of cycles running:\s*(\d+)", proc.stdout)
    if proc.returncode != 0 or not cyc:
        sys.exit(f"{label}: STONNE run failed (exit {proc.returncode}); see {outdir / 'stdout.log'}")
    txt = sorted(outdir.glob("output_stats_*.txt"))
    cnt = sorted(outdir.glob("output_stats_*.counters"))
    return elapsed, {
        "cycles": int(cyc[-1]),
        "passed": "Test passed correctly" in proc.stdout,
        "stats_txt_sha256": sha256(txt[0]) if len(txt) == 1 else "",
        "stats_counters_sha256": sha256(cnt[0]) if len(cnt) == 1 else "",
    }


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--stonne", type=Path, required=True, help="STONNE git checkout (repo root)")
    ap.add_argument("--out", type=Path, default=ROOT / "results" / "stonne_run")
    ap.add_argument("--warmup", type=int, default=1)
    ap.add_argument("--reps", type=int, default=3)
    ap.add_argument("--sizes", nargs="*", type=int, help="subset of N values")
    ap.add_argument("--jobs", type=int, default=min(16, os.cpu_count() or 1))
    args = ap.parse_args()

    cfg = json.loads((ROOT / "config" / "sizes.json").read_text())
    hist = json.loads((ROOT / "reference" / "stonne_historical_stats.json").read_text())["files"]
    legacy = legacy_commands()
    sizes = cfg["sizes"]
    if args.sizes:
        sizes = [s for s in sizes if s["n"] in set(args.sizes)]
    for s in sizes:
        if legacy.get(s["label"]) != s["stonne_args"]:
            sys.exit(f"{s['label']}: config stonne_args differ from data.csv Stonne_config:\n"
                     f"  config: {s['stonne_args']}\n  csv:    {legacy.get(s['label'])}")

    out = args.out.resolve()
    sdir = out / "sizes"
    if sdir.exists():
        shutil.rmtree(sdir)
    sdir.mkdir(parents=True)
    prov = {"stonne": build(args.stonne.resolve(), out / "src", args.jobs)}
    binary = Path(prov["stonne"]["binary"])
    prov["host"] = {"node": platform.node(), "machine": platform.machine(),
                    "platform": platform.platform(), "cpu_count": os.cpu_count()}
    prov["timing_boundary"] = ("external monotonic elapsed time around the prebuilt STONNE "
                               "process (median of measured runs)")
    prov["tracing"] = "not applicable; default statistics output on (as in the historical runs)"
    prov["warmup"], prov["reps"] = args.warmup, args.reps
    prov["commands"] = {s["label"]: s["stonne_args"] for s in sizes}

    runs, summary = [], []
    for s in sizes:
        label = s["label"]
        outdir = sdir / label
        outdir.mkdir()
        times = []
        for i in range(args.warmup + args.reps):
            measured = i >= args.warmup
            elapsed, rep = run_once(binary, s["stonne_args"], outdir, label)
            runs.append({"size": label, "phase": "measured" if measured else "warmup", "run": i,
                         "elapsed_s": f"{elapsed:.6f}", **rep})
            if measured:
                times.append(elapsed)
            print(f"[run] STONNE {label} #{i} {'measured' if measured else 'warm-up'} "
                  f"cycles={rep['cycles']} passed={rep['passed']} {elapsed:.3f}s", flush=True)
        h = hist[label]
        num_ms = int(next(a for a in s["stonne_args"] if a.startswith("-num_ms="))[len("-num_ms="):])
        if num_ms == h["num_ms"]:
            same = (rep["stats_txt_sha256"] == h["txt"]
                    and rep["stats_counters_sha256"] == h["counters"])
        else:
            same = f"n/a (historical file used num_ms={h['num_ms']})"
        summary.append({"size": label, "n": s["n"], "cycles": rep["cycles"],
                        "passed": rep["passed"], "stats_match_historical": same,
                        "median_elapsed_s": f"{statistics.median(times):.6f}",
                        "min_elapsed_s": f"{min(times):.6f}",
                        "max_elapsed_s": f"{max(times):.6f}", "reps": len(times)})

    extra = []
    for x in cfg.get("stonne_extra", []):
        outdir = sdir / x["name"]
        outdir.mkdir()
        elapsed, rep = run_once(binary, x["stonne_args"], outdir, x["name"])
        h = hist[x["historical_key"]]
        extra.append({"name": x["name"], "cycles": rep["cycles"], "passed": rep["passed"],
                      "stats_match_historical": rep["stats_txt_sha256"] == h["txt"]
                      and rep["stats_counters_sha256"] == h["counters"],
                      "elapsed_s": f"{elapsed:.6f}", "args": " ".join(x["stonne_args"])})
        print(f"[run] STONNE {x['name']} (untimed check) cycles={rep['cycles']} "
              f"passed={rep['passed']} stats_match_historical={extra[-1]['stats_match_historical']}",
              flush=True)

    for fname, rows in (("runs.csv", runs), ("summary.csv", summary), ("extra.csv", extra)):
        if rows:
            with open(out / fname, "w", newline="") as f:
                w = csv.DictWriter(f, fieldnames=list(rows[0]))
                w.writeheader()
                w.writerows(rows)
    (out / "provenance.json").write_text(json.dumps(prov, indent=2) + "\n")
    print(f"wrote {out}/summary.csv")
    bad = [r["size"] for r in summary if not r["passed"]] + [r["name"] for r in extra if not r["passed"]]
    if bad:
        sys.exit(f"STONNE correctness check failed for: {', '.join(bad)}")


if __name__ == "__main__":
    main()
