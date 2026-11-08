#!/usr/bin/env python3
"""Compare Zeonica and STONNE runs with the paper and write the plot inputs.

Inputs
  results/zeonica_run/summary.csv     (run_zeonica.py)
  results/stonne_run/summary.csv      (run_stonne.py)
  results/stonne_run/extra.csv        (run_stonne.py, 4x4 num_ms=64 check)
  reference/legacy_data.csv           the paper's data.csv
  config/expected.json                values printed in the paper

Outputs
  results/comparison.csv              per size: cycles (run and paper), residual,
                                      host times on both boundaries, ratios
  results/paper/plot_input.csv        data.csv layout; run cycles + the paper's
                                      host times (reproduces Fig. 5 and Fig. 7d)
  results/measured/plot_input.csv     data.csv layout; run cycles + host times
                                      measured on this machine (Zeonica: report
                                      wallClockDurationSec; STONNE: process time)
  results/summary.json                every check with PASS / FAIL / INFO

Checks marked FAIL make the script exit non-zero. INFO lines are reported but
do not gate (they depend on the host or on the timing boundary; see README).
"""

import csv
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
RES = ROOT / "results"
HEADER = ["Size", "Zeonica(cycle)", "Simulation Time(s)", "Stonne(cycle)",
          "Simulation Time(s)", "Stonne_config"]


def by_size(path: Path, key="size") -> dict:
    with path.open(newline="") as f:
        return {r[key]: r for r in csv.DictReader(f)}


def legacy_rows() -> dict:
    with (ROOT / "reference" / "legacy_data.csv").open(newline="") as f:
        rows = list(csv.reader(f))
    return {r[0]: r for r in rows[1:] if len(r) >= 6}


def label(err: float) -> str:
    # Same formatting as scripts/plot_fig6_hpca.py.
    return f"{'+' if err >= 0 else ''}{err:.1f}%"


def crossing(labels, ratios):
    for i in range(1, len(ratios)):
        if ratios[i - 1] < 1.0 <= ratios[i]:
            return f"between {labels[i - 1]} and {labels[i]}"
    return None


def main():
    cfg = json.loads((ROOT / "config" / "sizes.json").read_text())
    exp = json.loads((ROOT / "config" / "expected.json").read_text())
    zeo = by_size(RES / "zeonica_run" / "summary.csv")
    sto = by_size(RES / "stonne_run" / "summary.csv")
    extra_path = RES / "stonne_run" / "extra.csv"
    extra = by_size(extra_path, key="name") if extra_path.exists() else {}
    leg = legacy_rows()
    labels = [s["label"] for s in cfg["sizes"]]
    missing = [l for l in labels if l not in zeo or l not in sto]
    if missing:
        sys.exit(f"missing runs for {missing}; run run_zeonica.py and run_stonne.py first")

    checks = []

    def check(name, ok, detail, gating=True):
        status = ("PASS" if ok else "FAIL") if gating else "INFO"
        checks.append({"check": name, "status": status, "ok": bool(ok), "detail": detail})

    rows, paper_plot, meas_plot = [], [], []
    for l in labels:
        z, s, g = zeo[l], sto[l], leg[l]
        zc, sc = int(z["total_cycles"]), int(s["cycles"])
        err = (zc - sc) / sc * 100.0
        zt, st = float(z["median_elapsed_s"]), float(s["median_elapsed_s"])
        zspan = float(z["median_report_span_s"])
        pz, ps = float(g[2]), float(g[4])
        rows.append({
            "size": l, "zeonica_cycles": zc, "paper_zeonica_cycles": int(g[1]),
            "stonne_cycles": sc, "paper_stonne_cycles": int(g[3]),
            "residual_pct": f"{err:.4f}", "residual_label": label(err),
            "zeonica_passed": z["passed"], "stonne_passed": s["passed"],
            "zeonica_ext_s": z["median_elapsed_s"], "zeonica_report_span_s": z["median_report_span_s"],
            "stonne_ext_s": s["median_elapsed_s"],
            "ratio_measured": f"{zspan / st:.4f}",
            "ratio_process_both": f"{zt / st:.4f}",
            "paper_zeonica_s": g[2], "paper_stonne_s": g[4], "ratio_paper": f"{pz / ps:.4f}",
            "stonne_stats_match_historical": s["stats_match_historical"],
        })
        paper_plot.append([l, zc, g[2], sc, g[4], g[5]])
        # Fig. 7d boundary: Zeonica's wallClockDurationSec, STONNE's process time.
        meas_plot.append([l, zc, z["median_report_span_s"], sc, s["median_elapsed_s"], g[5]])

    # ---- Fig. 5 (cycles): exact reproduction of the paper -------------------------
    zc = {r["size"]: r["zeonica_cycles"] for r in rows}
    sc = {r["size"]: r["stonne_cycles"] for r in rows}
    check("fig5_zeonica_cycles", zc == exp["fig5"]["zeonica_cycles"],
          " ".join(f"{l}={zc[l]}" for l in labels))
    check("fig5_stonne_cycles", sc == exp["fig5"]["stonne_cycles"],
          " ".join(f"{l}={sc[l]}" for l in labels))
    errs = [abs(float(r["residual_pct"])) for r in rows]
    mape = sum(errs) / len(errs)
    check("fig5_mape", round(mape, 2) == exp["fig5"]["mape_pct_rounded"],
          f"MAPE = {mape:.4f}% (paper {exp['fig5']['mape_pct_rounded']:.2f}%)")
    got_labels = {r["size"]: r["residual_label"] for r in rows}
    check("fig5_residual_labels", got_labels == exp["fig5"]["residual_labels"],
          " ".join(f"{l}={got_labels[l]}" for l in labels))
    check("correctness", all(r["zeonica_passed"] == "True" and r["stonne_passed"] == "True"
                             for r in rows), "Zeonica report passed=true and STONNE 'Test passed correctly' for every size")
    plotted = all([str(a) for a in p[:5]] == leg[p[0]][:5] for p in paper_plot)
    check("paper_plot_input_equals_data_csv", plotted,
          "results/paper/plot_input.csv equals reference/legacy_data.csv on every plotted column")

    # ---- STONNE provenance (informational) ----------------------------------------
    hist = {r["size"]: r["stonne_stats_match_historical"] for r in rows}
    hist_ok = all(v == "True" for k, v in hist.items() if not v.startswith("n/a"))
    x = extra.get("4x4_num_ms64")
    x_detail = (f"; 4x4 with num_ms=64: cycles={x['cycles']}, stats match historical file: "
                f"{x['stats_match_historical']}") if x else ""
    check("stonne_stats_byte_identical_to_historical_sweep",
          hist_ok and (x is None or x["stats_match_historical"] == "True"),
          " ".join(f"{k}={v}" for k, v in hist.items()) + x_detail, gating=False)

    # ---- Fig. 7d (host-time ratio): qualitative trend -----------------------------
    r_meas = [float(r["ratio_measured"]) for r in rows]
    r_proc = [float(r["ratio_process_both"]) for r in rows]
    r_paper = [float(r["ratio_paper"]) for r in rows]
    paper_lbl = {l: f"{v:.2f}" for l, v in zip(labels, r_paper)}
    check("fig7d_paper_ratio_labels",
          all(paper_lbl[k] == v for k, v in exp["fig7d"]["ratio_labels"].items()),
          "paper ratios from data.csv: " + " ".join(f"{l}={paper_lbl[l]}x" for l in labels))
    increasing = all(b > a for a, b in zip(r_meas, r_meas[1:]))
    check("fig7d_ratio_grows_with_N", increasing,
          "measured (Zeonica wallClockDurationSec / STONNE process time): "
          + " ".join(f"{l}={v:.2f}x" for l, v in zip(labels, r_meas)))
    cx = crossing(labels, r_meas)
    check("fig7d_parity_crossing", cx is not None,
          f"measured ratio crosses 1x {cx}; paper: {exp['fig7d']['parity_crossing']}" if cx else
          f"measured ratio does not cross 1x (min {min(r_meas):.2f}x at {labels[r_meas.index(min(r_meas))]}); "
          f"paper: {exp['fig7d']['parity_crossing']}", gating=False)
    check("fig7d_process_time_ratio", True,
          "supplementary, both simulators timed as whole processes: "
          + " ".join(f"{l}={v:.2f}x" for l, v in zip(labels, r_proc)), gating=False)

    # ---- write ---------------------------------------------------------------------
    with (RES / "comparison.csv").open("w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=list(rows[0]))
        w.writeheader()
        w.writerows(rows)
    for variant, data in (("paper", paper_plot), ("measured", meas_plot)):
        (RES / variant).mkdir(parents=True, exist_ok=True)
        with (RES / variant / "plot_input.csv").open("w", newline="") as f:
            w = csv.writer(f)
            w.writerow(HEADER)
            w.writerows(data)
    failed = [c["check"] for c in checks if c["status"] == "FAIL"]
    summary = {"mape_pct": mape, "mape_pct_rounded": round(mape, 2),
               "ratio_measured": dict(zip(labels, r_meas)),
               "ratio_process_both": dict(zip(labels, r_proc)),
               "ratio_paper": dict(zip(labels, r_paper)),
               "checks": checks, "result": "FAIL" if failed else "PASS"}
    (RES / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")

    print(f"\n{'size':8s} {'Zeonica':>8s} {'STONNE':>7s} {'resid':>7s} | {'Z ext s':>9s} {'Z span s':>9s} "
          f"{'S ext s':>8s} {'ratio':>7s} {'paper':>7s}")
    for r in rows:
        print(f"{r['size']:8s} {r['zeonica_cycles']:>8d} {r['stonne_cycles']:>7d} {r['residual_label']:>7s} | "
              f"{float(r['zeonica_ext_s']):>9.3f} {float(r['zeonica_report_span_s']):>9.3f} "
              f"{float(r['stonne_ext_s']):>8.3f} {float(r['ratio_measured']):>6.2f}x "
              f"{float(r['ratio_paper']):>6.2f}x")
    print()
    for c in checks:
        print(f"{c['status']:4s}  {c['check']}: {c['detail']}")
    print(f"\nE2 result: {summary['result']}")
    if failed:
        sys.exit(1)


if __name__ == "__main__":
    main()
