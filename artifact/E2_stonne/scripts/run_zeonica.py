#!/usr/bin/env python3
"""Build and run the six Fig. 5 systolic-GEMM testbenches on Zeonica.

The run happens in a clean source snapshot: git HEAD of the Zeonica checkout
(without the artifact/ directory) and its Zeonica_Testbench submodule HEAD.
The user's working tree is never modified and uncommitted edits never leak
into the results.

Tracing. The as-committed arch specs disagree (4x4, 8x8 and 32x32 enable the
JSONL trace, 16x16, 64x64 and 128x128 disable it). Every timed run therefore
uses a generated trace-off copy of the size's arch spec,
<testbench>/arch_spec.timing.yaml, which differs from the original only in
`enableTrace: false`. For each size whose committed spec enables tracing, one
extra untimed run with the committed spec checks that the cycle count does
not depend on tracing.

Timing boundary: external monotonic elapsed time (time.monotonic) around the
prebuilt testbench binary process. The report's wallClockDurationSec (event
wall-time span inside driver.Run, the boundary of the paper's Zeonica column)
is recorded next to it.

Outputs (under --out):
  sizes/<label>/<report>.json   Zeonica report of the last measured run
  sizes/<label>/stdout.log      stdout of the last run
  runs.csv                      one row per run (trace check, warm-up, measured)
  summary.csv                   per-size cycles, correctness, median times
  provenance.json               commits, toolchain, host, settings
"""

import argparse
import csv
import json
import os
import platform
import re
import shutil
import statistics
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent
TRACE_RE = re.compile(r"^(\s*enableTrace:\s*)(true|false)\s*$", re.MULTILINE)


def sh(cmd, cwd=None, env=None, capture=True):
    return subprocess.run(
        cmd, cwd=cwd, env=env, check=True, text=True,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.STDOUT if capture else None,
    ).stdout


def git_info(repo: Path) -> dict:
    return {
        "path": str(repo),
        "commit": sh(["git", "-C", str(repo), "rev-parse", "HEAD"]).strip(),
        # artifact/ holds this experiment's own results, which a rerun rewrites;
        # it is not part of the snapshot, so it does not count as dirty.
        "dirty": bool(sh(["git", "-C", str(repo), "status", "--porcelain",
                          "--untracked-files=no", "--", ".", ":(exclude)artifact"]).strip()),
    }


def make_snapshot(zeonica: Path, src: Path, source: str) -> dict:
    if src.exists():
        shutil.rmtree(src)
    src.mkdir(parents=True)
    prov = {"zeonica": git_info(zeonica)}
    subprocess.run(f"git -C '{zeonica}' archive HEAD -- . ':(exclude)artifact' | tar -x -C '{src}'",
                   shell=True, check=True)
    prov["source"] = source
    if source == "worktree":
        # HEAD plus uncommitted edits to tracked files (untracked files ignored).
        diff = sh(["git", "-C", str(zeonica), "diff", "--binary", "HEAD"])
        if diff.strip():
            subprocess.run(["git", "apply", "--whitespace=nowarn", "-"], input=diff,
                           text=True, cwd=src, check=True)
        prov["worktree_diff_files"] = sh(["git", "-C", str(zeonica), "diff", "--name-only",
                                          "HEAD"]).split()
    sub = zeonica / "test" / "Zeonica_Testbench"
    if (sub / ".git").exists():
        prov["zeonica_testbench"] = git_info(sub)
        dst = src / "test" / "Zeonica_Testbench"
        dst.mkdir(parents=True, exist_ok=True)
        subprocess.run(f"git -C '{sub}' archive HEAD | tar -x -C '{dst}'",
                       shell=True, check=True)
    else:
        sys.exit(f"missing submodule checkout: {sub} (run git submodule update --init)")
    for name, info in prov.items():
        if source == "head" and isinstance(info, dict) and info.get("dirty"):
            print(f"WARNING: {name} checkout has uncommitted changes; "
                  f"the snapshot uses its HEAD ({info['commit'][:10]}) only.")
    return prov


def write_timing_spec(src: Path, size: dict) -> tuple:
    """Write <testbench>/arch_spec.timing.yaml (tracing off); return (path, committed_trace)."""
    base = src / size["arch_spec"]
    text = base.read_text()
    matches = TRACE_RE.findall(text)
    if len(matches) != 1:
        sys.exit(f"{base}: expected exactly one 'enableTrace:' line, found {len(matches)}")
    committed_trace = matches[0][1] == "true"
    out = src / size["dir"] / "arch_spec.timing.yaml"
    out.write_text(TRACE_RE.sub(r"\1false", text))
    return out, committed_trace


def read_report(path: Path) -> dict:
    d = json.loads(path.read_text())
    return {
        "total_cycles": d["totalCycles"],
        "passed": d["passed"],
        "mismatch_count": d.get("mismatchCount", ""),
        "report_span_s": d.get("wallClockDurationSec", ""),
    }


def run_once(binary: Path, cwd: Path, env: dict, report: Path, outdir: Path, label: str):
    for f in list(cwd.glob("*.report.json")) + list(cwd.glob("*.json.log")):
        f.unlink()
    t0 = time.monotonic()
    proc = subprocess.run([str(binary)], cwd=cwd, env=env, text=True,
                          stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    elapsed = time.monotonic() - t0
    (outdir / "stdout.log").write_text(proc.stdout)
    if proc.returncode != 0 or not report.exists():
        sys.exit(f"{label}: run failed (exit {proc.returncode}); see {outdir / 'stdout.log'}")
    rep = read_report(report)
    logs = list(cwd.glob("*.json.log"))
    rep["trace_bytes"] = logs[0].stat().st_size if logs else 0
    return elapsed, rep


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--zeonica", type=Path, default=ROOT.parent.parent,
                    help="Zeonica git checkout (default: the repository containing this artifact)")
    ap.add_argument("--out", type=Path, default=ROOT / "results" / "zeonica_run")
    ap.add_argument("--warmup", type=int, default=1, help="unmeasured runs per size")
    ap.add_argument("--reps", type=int, default=3, help="measured runs per size")
    ap.add_argument("--sizes", nargs="*", type=int, help="subset of N values")
    ap.add_argument("--source", choices=["head", "worktree"], default="head",
                    help="snapshot the checkout's HEAD (default) or HEAD plus uncommitted "
                         "edits to tracked files")
    args = ap.parse_args()

    cfg = json.loads((ROOT / "config" / "sizes.json").read_text())
    sizes = cfg["sizes"]
    if args.sizes:
        sizes = [s for s in sizes if s["n"] in set(args.sizes)]

    out = args.out.resolve()
    src, binp, sdir = out / "src", out / "bin", out / "sizes"
    for d in (binp, sdir):
        if d.exists():
            shutil.rmtree(d)
        d.mkdir(parents=True)

    prov = make_snapshot(args.zeonica.resolve(), src, args.source)
    prov["go"] = sh(["go", "version"]).strip().splitlines()[-1]
    prov["host"] = {"node": platform.node(), "machine": platform.machine(),
                    "platform": platform.platform(), "cpu_count": os.cpu_count()}
    prov["timing_boundary"] = ("external monotonic elapsed time around the prebuilt "
                               "testbench binary process (median of measured runs); "
                               "report wallClockDurationSec recorded alongside")
    prov["tracing"] = ("off for every timed run (generated arch_spec.timing.yaml); one "
                       "extra untimed run with the committed spec for sizes whose spec "
                       "enables tracing, to check that cycles do not change")
    prov["seed"] = cfg["seed"]
    prov["warmup"], prov["reps"] = args.warmup, args.reps

    for s in sizes:
        print(f"[build] {s['label']}", flush=True)
        # The snapshot is not a git checkout; commits are recorded in provenance.json.
        sh(["go", "build", "-buildvcs=false", "-o", str(binp / s["label"]), "./" + s["dir"]],
           cwd=src)

    runs, summary = [], []
    for s in sizes:
        label, cwd = s["label"], src / s["dir"]
        outdir = sdir / label
        outdir.mkdir()
        report = cwd / s["report"]
        timing_spec, committed_trace = write_timing_spec(src, s)
        base_env = dict(os.environ, ZEONICA_RAND_SEED=str(cfg["seed"]))
        base_env.pop("ZEONICA_ARCH_SPEC", None)
        base_env.pop("ZEONICA_PROGRAM_YAML", None)

        committed_cycles = ""
        if committed_trace:
            # Committed spec as the testbench resolves it by default (tracing on).
            elapsed, rep = run_once(binp / label, cwd, base_env, report, outdir, label)
            committed_cycles = rep["total_cycles"]
            runs.append({"size": label, "phase": "committed_spec_trace_on", "run": 0,
                         "elapsed_s": f"{elapsed:.6f}", **rep})
            print(f"[run] {label} committed spec (trace on) cycles={rep['total_cycles']} "
                  f"passed={rep['passed']} trace={rep['trace_bytes']}B {elapsed:.3f}s", flush=True)

        env = dict(base_env, ZEONICA_ARCH_SPEC=str(timing_spec))
        times, spans = [], []
        for i in range(args.warmup + args.reps):
            measured = i >= args.warmup
            elapsed, rep = run_once(binp / label, cwd, env, report, outdir, label)
            if rep["trace_bytes"]:
                sys.exit(f"{label}: timing run produced a non-empty trace ({rep['trace_bytes']} B)")
            runs.append({"size": label, "phase": "measured" if measured else "warmup", "run": i,
                         "elapsed_s": f"{elapsed:.6f}", **rep})
            if measured:
                times.append(elapsed)
                spans.append(float(rep["report_span_s"]))
            print(f"[run] {label} #{i} {'measured' if measured else 'warm-up'} "
                  f"cycles={rep['total_cycles']} passed={rep['passed']} {elapsed:.3f}s", flush=True)
        shutil.copy2(report, outdir / s["report"])
        summary.append({
            "size": label, "n": s["n"], "total_cycles": rep["total_cycles"],
            "passed": rep["passed"], "mismatch_count": rep["mismatch_count"],
            "committed_spec_trace_on": committed_trace,
            "committed_spec_cycles": committed_cycles,
            "median_elapsed_s": f"{statistics.median(times):.6f}",
            "min_elapsed_s": f"{min(times):.6f}", "max_elapsed_s": f"{max(times):.6f}",
            "median_report_span_s": f"{statistics.median(spans):.6f}",
            "reps": len(times),
        })

    for fname, rows in (("runs.csv", runs), ("summary.csv", summary)):
        with open(out / fname, "w", newline="") as f:
            w = csv.DictWriter(f, fieldnames=list(rows[0]))
            w.writeheader()
            w.writerows(rows)
    (out / "provenance.json").write_text(json.dumps(prov, indent=2) + "\n")

    print(f"wrote {out}/summary.csv")
    bad = [r["size"] for r in summary if not r["passed"]]
    drift = [r["size"] for r in summary
             if r["committed_spec_cycles"] != "" and r["committed_spec_cycles"] != r["total_cycles"]]
    if bad:
        sys.exit(f"correctness check failed for: {', '.join(bad)}")
    if drift:
        sys.exit(f"cycle count depends on tracing for: {', '.join(drift)}")


if __name__ == "__main__":
    main()
