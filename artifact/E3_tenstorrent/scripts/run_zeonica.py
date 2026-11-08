#!/usr/bin/env python3
"""Build the TT/Wormhole Zeonica model from this repository and run the 10
held-out kernels with the frozen freeze-4 profile.

Steps
  1. Snapshot the source: `git archive <rev>` of the repository containing this
     artifact (default rev HEAD, artifact/ excluded), so uncommitted edits are
     never used.
  2. `go build -buildvcs=false -trimpath -o bin/tt_eir ./experiments/tt_eir`.
  3. Run every kernel in config/kernels.json:
       tt_eir -kernel K -config config/tt_eir_freeze4.yaml
              -data-dir reference/ttsim_gtest_dump/K/dump
              -summary-json kernels/K/summary.json -output-u32 kernels/K/zeonica_out.u32
     The binary feeds the TT-Sim input DRAM images (in0/in1) through the model and
     compares its output DRAM image with TT-Sim's readback (out.u32); it exits
     non-zero if the data check fails.
  4. Write results.csv (one row per kernel) and provenance.json.

Only the Python standard library is used.
usage: run_zeonica.py [--rev HEAD] [--out results/zeonica_run] [--jobs 8]
"""
import argparse
import concurrent.futures as cf
import csv
import hashlib
import json
import os
import platform
import shutil
import subprocess
import sys
import tarfile
import io
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REPO = ROOT.parents[1]  # the Zeonica repository containing artifact/E3_tenstorrent
CFG = ROOT / "config" / "tt_eir_freeze4.yaml"
KERNELS = ROOT / "config" / "kernels.json"
DUMPS = ROOT / "reference" / "ttsim_gtest_dump"


def sh(cmd, **kw):
    return subprocess.run(cmd, check=True, capture_output=True, text=True, **kw).stdout.strip()


def sha256(p):
    return hashlib.sha256(Path(p).read_bytes()).hexdigest()


def snapshot(rev, src):
    if src.exists():
        shutil.rmtree(src)
    src.mkdir(parents=True)
    commit = sh(["git", "-C", str(REPO), "rev-parse", rev + "^{commit}"])
    dirty = sh(["git", "-C", str(REPO), "status", "--porcelain", "--untracked-files=no"]) != ""
    data = subprocess.run(["git", "-C", str(REPO), "archive", "--format=tar", commit,
                           "--", ".", ":(exclude)artifact"], check=True, capture_output=True).stdout
    kw = {"filter": "data"} if hasattr(tarfile, "data_filter") else {}
    with tarfile.open(fileobj=io.BytesIO(data)) as t:
        t.extractall(src, **kw)
    return {"mode": "git archive", "rev": rev, "commit": commit, "worktree_dirty_ignored": dirty}


def clean_env():
    # Zeonica reads several ZEONICA_* variables (trace, latency sidecars); the
    # runner sets the ones it needs itself, so start from none of them.
    env = {k: v for k, v in os.environ.items() if not k.startswith("ZEONICA_")}
    return env


def run_kernel(binary, k, outdir, env):
    d = outdir / "kernels" / k
    if d.exists():
        shutil.rmtree(d)
    d.mkdir(parents=True)
    data = DUMPS / k / "dump"
    cmd = [str(binary), "-kernel", k, "-config", str(CFG), "-data-dir", str(data),
           "-summary-json", str(d / "summary.json"), "-output-u32", str(d / "zeonica_out.u32")]
    t0 = time.monotonic()
    q = subprocess.run(cmd, capture_output=True, text=True, env=env)
    wall = time.monotonic() - t0
    (d / "stdout.log").write_text(q.stdout + q.stderr)
    if not (d / "summary.json").exists():
        raise RuntimeError(f"{k}: no summary (exit {q.returncode})\n{q.stdout[-2000:]}{q.stderr[-2000:]}")
    s = json.loads((d / "summary.json").read_text())
    s["data_check"]["source"] = os.path.relpath(data, ROOT)  # keep results path-independent
    (d / "summary.json").write_text(json.dumps(s, indent=1) + "\n")
    return k, s, q.returncode, wall


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--rev", default="HEAD", help="repository revision to build (default: HEAD)")
    ap.add_argument("--out", default=str(ROOT / "results" / "zeonica_run"))
    ap.add_argument("--jobs", type=int, default=8)
    a = ap.parse_args()
    out = Path(a.out).resolve()
    out.mkdir(parents=True, exist_ok=True)
    env = clean_env()

    t_start = time.monotonic()
    src = out / "src"
    prov = {"zeonica": snapshot(a.rev, src)}
    print(f"[run_zeonica] source: {prov['zeonica']}", flush=True)

    bindir = out / "bin"
    bindir.mkdir(exist_ok=True)
    binary = bindir / "tt_eir"
    t0 = time.monotonic()
    subprocess.run(["go", "build", "-buildvcs=false", "-trimpath", "-o", str(binary), "./experiments/tt_eir"],
                   cwd=src, check=True, env=env)
    build_s = time.monotonic() - t0
    print(f"[run_zeonica] built {binary.relative_to(ROOT)} in {build_s:.1f}s", flush=True)

    kernels = json.loads(KERNELS.read_text())["kernels"]
    refs = {r["kernel"]: r for r in json.loads((ROOT / "reference" / "ttsim_reference_cycles.json").read_text())["rows"]}
    t0 = time.monotonic()
    with cf.ThreadPoolExecutor(a.jobs) as ex:
        res = list(ex.map(lambda k: run_kernel(binary, k["name"], out, env), kernels))
    run_s = time.monotonic() - t0

    rows, failed = [], []
    for kspec, (k, s, rc, wall) in zip(kernels, res):
        ref = refs[k]
        assert ref["role"] == "heldout", f"{k} is not a held-out kernel"
        dc = s["data_check"]
        rows.append(dict(kernel=k, label=kspec["label"], role=ref["role"], ttsim_cycles=ref["reference_cycles"],
                         zeonica_cycles=s["zeonica_cycles"], eir_cycles=s["eir_cycles"], fixed_cycles=s["fixed_cycles"],
                         fixed_components=json.dumps(s["fixed_components"], sort_keys=True),
                         pes=s["pes"], eir_instructions=s["eir_instructions"], cb_queues=s["cb_queues"],
                         data_mode=dc["mode"], data_elements=dc["elements"], bit_exact=dc["bit_exact"],
                         bit_exact_fraction=dc["bit_exact_fraction"], max_ulp=dc["max_ulp"],
                         data_passed=dc["passed"], exit_code=rc, model_wall_s=round(s["wall_seconds"], 3),
                         process_wall_s=round(wall, 3)))
        print(f"  {k:44s} zeonica={s['zeonica_cycles']:>9.0f} ttsim={ref['reference_cycles']:>7d} "
              f"data={dc['mode']}:{dc['bit_exact']}/{dc['elements']} passed={dc['passed']}", flush=True)
        if not dc["passed"] or rc != 0:
            failed.append(k)

    with (out / "results.csv").open("w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=list(rows[0]), lineterminator="\n")
        w.writeheader()
        w.writerows(rows)

    go_version = sh(["go", "version"])
    prov.update({
        "config": {"path": os.path.relpath(CFG, ROOT), "sha256": sha256(CFG)},
        "binary_sha256": sha256(binary),
        "go": go_version,
        "build_flags": "-buildvcs=false -trimpath",
        "host": {"node": platform.node(), "machine": platform.machine(), "platform": platform.platform(),
                 "cpu_count": os.cpu_count(), "python": platform.python_version()},
        "jobs": a.jobs,
        "seconds": {"build": round(build_s, 2), "kernels": round(run_s, 2), "total": round(time.monotonic() - t_start, 2)},
        "data_check_failed": failed,
    })
    (out / "provenance.json").write_text(json.dumps(prov, indent=2) + "\n")
    print(f"[run_zeonica] {len(rows)} kernels in {run_s:.1f}s; results -> {(out / 'results.csv').relative_to(ROOT)}")
    if failed:
        print(f"[run_zeonica] FAIL: data check failed for {failed}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
