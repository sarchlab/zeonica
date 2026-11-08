#!/usr/bin/env python3
"""Score a run_zeonica.py run against the TT-Sim reference cycles.

APE_k = |zeonica_k - ttsim_k| / ttsim_k * 100, MAPE = mean over the 10 held-out kernels.

Checks (each PASS/FAIL, all must pass):
  mape            MAPE rounded to 2 decimals equals the paper value (3.28%) and to
                  4 decimals the freeze-4 value (3.2802%)
  per_kernel_ape  every kernel's APE rounded to 2 decimals equals the paper value
  max_ape         the largest APE rounds to 9.15% and belongs to the 3x3 matmul
  data            every kernel passed the output-data check against TT-Sim readback
  freeze4         every kernel's Zeonica cycle count equals the original freeze-4
                  run (reference/freeze4/kernels.csv), its summary fields (E-IR
                  cycles, fixed components, PEs, instructions, CB queues, data
                  check) equal reference/freeze4/summaries/<k>.json, and its
                  output DRAM image hashes to reference/freeze4/zeonica_out.sha256

Writes <out>/fig6_data.csv (plot input) and <out>/summary.json.
usage: evaluate.py --run results/zeonica_run --out results
"""
import argparse
import csv
import hashlib
import json
import statistics
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--run", default=str(ROOT / "results" / "zeonica_run"))
    ap.add_argument("--out", default=str(ROOT / "results"))
    a = ap.parse_args()
    run, out = Path(a.run), Path(a.out)

    cfg = json.loads((ROOT / "config" / "kernels.json").read_text())
    exp = cfg["expected"]
    got = {r["kernel"]: r for r in csv.DictReader((run / "results.csv").open())}
    f4dir = ROOT / "reference" / "freeze4"
    f4 = {r["kernel"]: r for r in csv.DictReader((f4dir / "kernels.csv").open())}
    f4hash = dict(reversed(l.split()) for l in (f4dir / "zeonica_out.sha256").read_text().splitlines())

    def same_as_freeze4(k):
        new = json.loads((run / "kernels" / k / "summary.json").read_text())
        old = json.loads((f4dir / "summaries" / f"{k}.json").read_text())
        fields = ["zeonica_cycles", "eir_cycles", "fixed_cycles", "fixed_components", "pes", "eir_instructions", "cb_queues"]
        dn, do = dict(new["data_check"]), dict(old["data_check"])
        dn.pop("source"), do.pop("source")
        out = hashlib.sha256((run / "kernels" / k / "zeonica_out.u32").read_bytes()).hexdigest()
        return all(new[f] == old[f] for f in fields) and dn == do, out == f4hash[k]

    rows = []
    for k in cfg["kernels"]:
        r = got[k["name"]]
        ref, z = int(r["ttsim_cycles"]), float(r["zeonica_cycles"])
        ape = 100 * abs(z - ref) / ref
        f4z = float(f4[k["name"]]["eir_zeonica"])
        summ_same, out_same = same_as_freeze4(k["name"])
        rows.append(dict(kernel=k["name"], label=k["label"], ttsim_cycles=ref, zeonica_cycles=z,
                         ape_pct=ape, ape_pct_2dp=round(ape, 2), paper_ape_pct=k["paper_ape_pct"],
                         ape_matches_paper=round(ape, 2) == k["paper_ape_pct"],
                         freeze4_zeonica_cycles=f4z, cycles_equal_freeze4=z == f4z,
                         summary_equal_freeze4=summ_same, output_equal_freeze4=out_same,
                         eir_cycles=float(r["eir_cycles"]), fixed_cycles=float(r["fixed_cycles"]),
                         data_mode=r["data_mode"], bit_exact_fraction=float(r["bit_exact_fraction"]),
                         data_passed=r["data_passed"] == "True"))

    mape = statistics.mean(r["ape_pct"] for r in rows)
    mx = max(rows, key=lambda r: r["ape_pct"])
    checks = {
        "mape": round(mape, 2) == exp["mape_pct"] and round(mape, 4) == exp["mape_pct_unrounded"],
        "per_kernel_ape": all(r["ape_matches_paper"] for r in rows),
        "max_ape": round(mx["ape_pct"], 2) == exp["max_ape_pct"] and mx["kernel"] == exp["max_ape_kernel"],
        "data": all(r["data_passed"] for r in rows),
        "freeze4": all(r["cycles_equal_freeze4"] and r["summary_equal_freeze4"] and r["output_equal_freeze4"] for r in rows),
    }
    ok = all(checks.values())

    out.mkdir(parents=True, exist_ok=True)
    with (out / "fig6_data.csv").open("w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=list(rows[0]), lineterminator="\n")
        w.writeheader()
        w.writerows(rows)
    summary = {
        "experiment": "Fig. 6 / Table II: Zeonica TT/Wormhole model vs TT-Sim, 10 held-out kernels",
        "kernels": len(rows),
        "mape_pct": mape,
        "mape_pct_rounded": round(mape, 2),
        "expected_mape_pct": exp["mape_pct"],
        "max_ape_pct": mx["ape_pct"],
        "max_ape_kernel": mx["kernel"],
        "per_kernel_ape_pct": {r["kernel"]: round(r["ape_pct"], 4) for r in rows},
        "checks": {k: "PASS" if v else "FAIL" for k, v in checks.items()},
        "check": "PASS" if ok else "FAIL",
    }
    (out / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")

    for r in rows:
        print(f"  {r['label']:20s} TT-Sim {r['ttsim_cycles']:>7d}  Zeonica {r['zeonica_cycles']:>9.0f}  "
              f"APE {r['ape_pct']:6.2f}% (paper {r['paper_ape_pct']:.2f}%)  "
              f"{'ok' if r['ape_matches_paper'] else 'MISMATCH'}  freeze4={'same' if r['cycles_equal_freeze4'] and r['summary_equal_freeze4'] and r['output_equal_freeze4'] else 'DIFF'}")
    print(f"MAPE = {mape:.4f}% (rounded {mape:.2f}%, paper {exp['mape_pct']:.2f}%); "
          f"max {mx['ape_pct']:.2f}% ({mx['label']})")
    for k, v in checks.items():
        print(f"  check {k:15s} {'PASS' if v else 'FAIL'}")
    print("PASS" if ok else "FAIL")
    sys.exit(0 if ok else 1)


if __name__ == "__main__":
    main()
