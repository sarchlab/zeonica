#!/usr/bin/env python3
"""Check that the artifact's inputs are the frozen ones.

1. Freeze record. config/pre_validation_freeze4.json is the hash list written by
   the original pipeline's freeze step (2026-10-05T03:35Z) before the held-out
   kernels and validation conditions were run. It covers the profile
   (configs/tt_eir.yaml), the model binary (build/tt_eir_pruned) and the 60 source
   files of the pruned Zeonica tree. This script hashes
     - config/tt_eir_freeze4.yaml            against configs/tt_eir.yaml
     - every recorded file of the pruned tree against the same path in this
       repository at git HEAD (--rev). The pruned tree's UPSTREAM_COMMIT file
       is not a source file: it names the upstream commit the tree was cut
       from, and the check instead requires that commit to be an ancestor of
       HEAD. Files of HEAD that the pruned tree omitted (test/, cmd/, docs/,
       ...) are allowed, except Go files inside the recorded package
       directories, which would change the build.
   and requires an exact match. The binary hash is reported but not required:
   a rebuild embeds different paths and toolchain data (the cycle and output
   identity with freeze 4 is checked by evaluate.py instead).
2. TT-Sim references. Each held-out reference cycle count in
   reference/ttsim_reference_cycles.json must equal the dump-off gtest run in
   reference/ttsim_gtest_nodump/<k>/ (result.json and the TT-Sim exit line
   "[<cycles>] <s> seconds (<f> KHz)" in stdout.log), and every dump-off
   observation recorded in the JSON. The dump-on run (which produced the tensor
   images) is reported, not required: in fast-dispatch mode the gtest end point
   includes host completion-queue polling, so a dump-on run can differ (it does
   for sfpu_sigmoid_4tile: 12613 vs 12531 in all 9 dump-off runs). Cycle
   references therefore come from dump-off runs and data from dump-on runs.

usage: check_freeze.py [--out results/freeze_check.json]
"""
import argparse
import hashlib
import json
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REPO = ROOT.parents[1]  # the Zeonica repository containing artifact/E3_tenstorrent


def git(*args, binary=False):
    out = subprocess.run(["git", "-C", str(REPO), *args], check=True, capture_output=True).stdout
    return out if binary else out.decode().strip()


def files_at(rev):
    names = git("ls-tree", "-r", "--name-only", rev).split()
    return names, (lambda n: git("show", f"{rev}:{n}", binary=True))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default=str(ROOT / "results" / "freeze_check.json"))
    ap.add_argument("--rev", default="HEAD", help="repository revision to check (default: HEAD)")
    a = ap.parse_args()

    pv = json.loads((ROOT / "config" / "pre_validation_freeze4.json").read_text())
    want = pv["sha256"]
    names, show = files_at(a.rev)
    commit = git("rev-parse", a.rev + "^{commit}")
    how = f"{a.rev} = {commit}"
    h = lambda b: hashlib.sha256(b).hexdigest()

    pruned_keys = {k[len("pruned/"):] for k in want if k.startswith("pruned/")}
    src_match, src_mismatch = [], []
    for rel in sorted(pruned_keys - {"UPSTREAM_COMMIT"}):
        (src_match if rel in names and h(show(rel)) == want["pruned/" + rel] else src_mismatch).append(rel)
    # UPSTREAM_COMMIT is not stored in the repository; its recorded hash is the
    # hash of "<upstream sha>\n". Look for that sha among the ancestors of rev.
    upstream = next((sha for sha in git("rev-list", a.rev).split()
                     if h((sha + "\n").encode()) == want["pruned/UPSTREAM_COMMIT"]), None)
    pkg_dirs = {str(Path(r).parent) for r in pruned_keys if "/" in r}
    extra = sorted(n for n in names if n.endswith(".go") and str(Path(n).parent) in pkg_dirs
                   and n not in pruned_keys)
    cfg_ok = h((ROOT / "config" / "tt_eir_freeze4.yaml").read_bytes()) == want["configs/tt_eir.yaml"]

    kernels = [k["name"] for k in json.loads((ROOT / "config" / "kernels.json").read_text())["kernels"]]
    refs = {r["kernel"]: r for r in json.loads((ROOT / "reference" / "ttsim_reference_cycles.json").read_text())["rows"]}
    ref_rows = []
    for k in kernels:
        nd = ROOT / "reference" / "ttsim_gtest_nodump" / k
        res = json.loads((nd / "result.json").read_text())
        m = re.search(r"^\[(\d+)\]\s+[0-9.]+\s+seconds\s+\([0-9.]+\s+KHz\)\s*$", (nd / "stdout.log").read_text(), re.M)
        dump = json.loads((ROOT / "reference" / "ttsim_gtest_dump" / k / "result.json").read_text())
        r = refs[k]
        obs = r["dump_off_observations"]
        ok = (r["role"] == "heldout" and res["passed"] and m is not None
              and r["reference_cycles"] == res["cycles"] == int(m.group(1))
              and all(o == r["reference_cycles"] for o in obs))
        ref_rows.append(dict(kernel=k, reference_cycles=r["reference_cycles"], nodump_result=res["cycles"],
                             nodump_stdout=int(m.group(1)) if m else None, dump_off_observations=obs,
                             dump_on=dump["cycles"], dump_on_equal=dump["cycles"] == r["reference_cycles"], ok=ok))

    checks = {
        "config_hash": cfg_ok,
        "source_hashes": not src_mismatch and len(src_match) == len(pruned_keys) - 1 and not extra,
        "upstream_commit_is_ancestor": upstream is not None,
        "ttsim_references": all(r["ok"] for r in ref_rows),
    }
    out = {
        "freeze_record": {"created_utc": pv["created_utc"], "files": pv["files"]},
        "zeonica_source": how,
        "upstream_commit": upstream,
        "source_files_matched": len(src_match),
        "source_files_expected": len(pruned_keys) - 1,
        "source_mismatches": src_mismatch,
        "go_files_not_in_freeze_record": extra,
        "binary": "build/tt_eir_pruned sha256 %s (not compared: rebuilt locally)" % want["build/tt_eir_pruned"],
        "ttsim_references": ref_rows,
        "checks": {k: "PASS" if v else "FAIL" for k, v in checks.items()},
        "check": "PASS" if all(checks.values()) else "FAIL",
    }
    Path(a.out).parent.mkdir(parents=True, exist_ok=True)
    Path(a.out).write_text(json.dumps(out, indent=2) + "\n")
    print(f"freeze record {pv['created_utc']}: config {'match' if cfg_ok else 'MISMATCH'}; "
          f"Zeonica ({how}) {len(src_match)}/{len(pruned_keys) - 1} source files match; "
          f"UPSTREAM_COMMIT {upstream[:10] + ' is an ancestor' if upstream else 'is NOT an ancestor'}"
          + (f"; mismatches {src_mismatch}" if src_mismatch else "")
          + (f"; unrecorded Go files {extra}" if extra else ""))
    for r in ref_rows:
        print(f"  TT-Sim {r['kernel']:44s} ref={r['reference_cycles']:>7d} dump-off={r['nodump_result']} "
              f"stdout={r['nodump_stdout']} n_obs={len(r['dump_off_observations'])} {'ok' if r['ok'] else 'MISMATCH'}"
              + ("" if r["dump_on_equal"] else f"  (dump-on run: {r['dump_on']}, informational)"))
    for k, v in checks.items():
        print(f"  check {k:27s} {'PASS' if v else 'FAIL'}")
    print(out["check"])
    sys.exit(0 if out["check"] == "PASS" else 1)


if __name__ == "__main__":
    main()
