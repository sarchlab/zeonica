#!/usr/bin/env python3
"""Machine-generate Table II (TT-Sim configuration and cycles of the 10 held-out
kernels) and cross-check the model's configuration against TT-Sim's.

TT-Sim side  reference/ttsim_configuration_cb.csv: shape, tile geometry, output
             tiles, data type, core grid and CB capacities of each gtest (a
             column subset of the evaluation deliverable tt_sim_configuration.csv,
             read from the tt-metal gtest sources; its cycle columns belonged to
             an older model round and are not used).
Model side   the CB capacities, core count and output page count that the frozen
             model actually configures, obtained by building each kernel's E-IR
             program through the runner's own lookup()/build() path
             (scripts/table2_caps.go.txt, compiled into a scratch copy of the
             built snapshot; the repository is not modified).
Cycles       results/fig6_data.csv (written by evaluate.py).

Writes results/table2.csv and results/table2_model_config.json.
usage: make_table2.py [--run results/zeonica_run] [--out results]
"""
import argparse
import csv
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def cbs_compact(d):
    return " ".join(f"{k}={v}" for k, v in sorted(d.items(), key=lambda kv: int(kv[0][1:])))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--run", default=str(ROOT / "results" / "zeonica_run"))
    ap.add_argument("--out", default=str(ROOT / "results"))
    a = ap.parse_args()
    run, out = Path(a.run), Path(a.out)
    cfg = json.loads((ROOT / "config" / "kernels.json").read_text())["kernels"]
    names = [k["name"] for k in cfg]

    # Model-side configuration from the frozen model itself. Without a built
    # snapshot (e.g. `run_all.sh --skip-run` on shipped results) the previously
    # generated results/table2_model_config.json is reused.
    mcfg = out / "table2_model_config.json"
    if (run / "src").exists():
        tsrc = run / "table2_src"
        if tsrc.exists():
            shutil.rmtree(tsrc)
        shutil.copytree(run / "src", tsrc)
        shutil.copy(ROOT / "scripts" / "table2_caps.go.txt", tsrc / "experiments" / "tt_eir" / "table2_caps.go")
        binary = run / "bin" / "tt_eir_table2"
        env = {k: v for k, v in os.environ.items() if not k.startswith("ZEONICA_")}
        subprocess.run(["go", "build", "-buildvcs=false", "-trimpath", "-o", str(binary), "./experiments/tt_eir"],
                       cwd=tsrc, check=True, env=env)
        q = subprocess.run([str(binary), str(ROOT / "config" / "tt_eir_freeze4.yaml"), *names], check=True,
                           capture_output=True, text=True, env={**env, "TT_TABLE2_CAPS": "1"})
        mcfg.write_text(json.dumps(json.loads(q.stdout), indent=1) + "\n")
    else:
        print(f"[make_table2] no snapshot in {run / 'src'}; reusing {mcfg.relative_to(ROOT)}")
    model = {e["kernel"]: e for e in json.loads(mcfg.read_text())}

    tts = {r["kernel"]: r for r in csv.DictReader((ROOT / "reference" / "ttsim_configuration_cb.csv").open())}
    part = {r["kernel"]: r for r in json.loads((ROOT / "reference" / "kernel_partition.json").read_text())}
    fig6 = {r["kernel"]: r for r in csv.DictReader((out / "fig6_data.csv").open())}

    rows, notes = [], []
    for k in cfg:
        n = k["name"]
        t, m, f, p = tts[n], model[n], fig6[n], part[n]
        tcb = {c: v["tiles"] for c, v in json.loads(t["circular_buffer_capacity_tiles_json"]).items()}
        mcb = {}
        for c in m["cbs_core0"]:
            mcb[c["name"]] = c["tiles"]
        common = sorted(set(tcb) & set(mcb), key=lambda c: int(c[1:]))
        mism = [f"{c}: TT-Sim {tcb[c]} vs model {mcb[c]}" for c in common if tcb[c] != mcb[c]]
        not_modeled = [c for c in sorted(tcb, key=lambda c: int(c[1:])) if c not in mcb]
        grid_cores = int(t["core_grid"].split(";")[1].split()[0])
        rows.append({
            "label": k["label"],
            "kernel": n,
            "gtest": p["gtest_filter"],
            "shape": t["tensor_shape"],
            "tiles": t["tile_geometry"],
            "output_tiles": int(t["num_output_tiles"]),
            "data_type": t["data_type"],
            "core_grid": t["core_grid"].split(";")[0].strip(),
            "ttsim_cb_tiles": cbs_compact(tcb),
            "model_cb_tiles": cbs_compact(mcb),
            "cb_common_match": not mism,
            "cb_mismatches": "; ".join(mism),
            "ttsim_cbs_not_modeled": " ".join(not_modeled),
            "model_cores": m["tt_cores"],
            "cores_match": m["tt_cores"] == grid_cores,
            "model_output_pages": m["out_pages"],
            "output_tiles_match": m["out_pages"] == int(t["num_output_tiles"]),
            "ttsim_cycles": int(f["ttsim_cycles"]),
            "zeonica_cycles": int(float(f["zeonica_cycles"])),
            "ape_pct": round(float(f["ape_pct"]), 2),
        })

    with (out / "table2.csv").open("w", newline="") as fh:
        w = csv.DictWriter(fh, fieldnames=list(rows[0]), lineterminator="\n")
        w.writeheader()
        w.writerows(rows)

    print(f"{'kernel':20s} {'grid':5s} {'dtype':10s} {'TT-Sim CBs (tiles)':40s} {'model CBs (tiles)':28s} "
          f"{'TT-Sim':>7s} {'Zeonica':>8s} {'APE':>6s}")
    for r in rows:
        print(f"{r['label']:20s} {r['core_grid']:5s} {r['data_type']:10s} {r['ttsim_cb_tiles']:40s} "
              f"{r['model_cb_tiles']:28s} {r['ttsim_cycles']:>7d} {r['zeonica_cycles']:>8d} {r['ape_pct']:>5.2f}%")
    for r in rows:
        extra = []
        if r["cb_mismatches"]:
            extra.append("CB capacity differs: " + r["cb_mismatches"])
        if r["ttsim_cbs_not_modeled"]:
            extra.append("TT-Sim CBs without a model queue: " + r["ttsim_cbs_not_modeled"])
        if not r["cores_match"] or not r["output_tiles_match"]:
            extra.append(f"cores {r['model_cores']} / output pages {r['model_output_pages']}")
        if extra:
            print(f"  note {r['label']}: " + "; ".join(extra))
    struct_ok = all(r["cores_match"] and r["output_tiles_match"] for r in rows)
    print(f"cores and output tiles agree for all kernels: {struct_ok}; "
          f"CB capacities agree on every shared CB: {all(r['cb_common_match'] for r in rows)}")
    print(f"wrote {(out / 'table2.csv').relative_to(ROOT)}")
    sys.exit(0 if struct_ok else 1)


if __name__ == "__main__":
    main()
