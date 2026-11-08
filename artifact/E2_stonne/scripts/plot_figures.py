#!/usr/bin/env python3
"""Draw Fig. 5 and Fig. 7d with the paper's unmodified plotting scripts.

scripts/plot_fig6_hpca.py (draws paper Fig. 5; its file name is historical)
and scripts/plot_host_time_ratio.py (Fig. 7d) are byte-identical copies of the
scripts used for the paper. Both read a module-level DATA_CSV and write to a
module-level OUT_BASE next to themselves and take no arguments, so this
wrapper loads each one, points those two globals at --input / --output-dir,
and calls its main(). The scripts' sha256 is checked first.

Outputs (under --output-dir)
  comparison_plot_hpca.{pdf,png}            Fig. 5
  zeonica_stonne_host_time_ratio.{pdf,png}  Fig. 7d
"""

import argparse
import hashlib
import importlib.util
from pathlib import Path

HERE = Path(__file__).resolve().parent
SCRIPTS = {
    "plot_fig6_hpca.py": ("1882d6f10117b1eea25ec7fd30e7f3f6a4ff5922059b3fb6b56849551d4dc931",
                          "comparison_plot_hpca"),
    "plot_host_time_ratio.py": ("9a7ba7305c8d190c34ce2f7d807f5c0e72273d3fef10a2bd148d6af23c5bc6f2",
                                "zeonica_stonne_host_time_ratio"),
}


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--input", type=Path, required=True, help="CSV in the paper's data.csv layout")
    ap.add_argument("--output-dir", type=Path, required=True)
    args = ap.parse_args()
    args.output_dir.mkdir(parents=True, exist_ok=True)

    for name, (digest, out_name) in SCRIPTS.items():
        path = HERE / name
        got = hashlib.sha256(path.read_bytes()).hexdigest()
        if got != digest:
            raise SystemExit(f"{path}: sha256 {got} differs from the paper script ({digest})")
        spec = importlib.util.spec_from_file_location(path.stem, path)
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        mod.DATA_CSV = args.input.resolve()
        mod.OUT_BASE = args.output_dir.resolve() / out_name
        mod.main()
        print(f"[plot] {name} -> {mod.OUT_BASE}.{{pdf,png}}")


if __name__ == "__main__":
    main()
