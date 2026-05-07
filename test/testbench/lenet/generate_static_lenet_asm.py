#!/usr/bin/env python3
"""Generate static real-compute LeNet ASM files for packed and sequential modes.

This generator emits only static ASM artifacts. Runtime execution must load the
generated YAML converted by tool/asm_to_yaml.py.
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path


INPUT_H = 28
INPUT_W = 28
CONV1_C = 8
CONV2_C = 16
FC1_OUT = 64
LOGITS_N = 10

FEAT1_H = 14
FEAT1_W = 14
FEAT2_H = 7
FEAT2_W = 7

INPUT_SIZE = INPUT_H * INPUT_W
FEAT1_SIZE = FEAT1_H * FEAT1_W * CONV1_C
FEAT2_SIZE = FEAT2_H * FEAT2_W * CONV2_C
FC1_IN = FEAT2_SIZE
CONV1_W_SIZE = CONV1_C * 3 * 3
CONV2_W_SIZE = CONV2_C * CONV1_C * 3 * 3
FC1_W_SIZE = FC1_OUT * FC1_IN
FC2_W_SIZE = LOGITS_N * FC1_OUT

INPUT_BASE = 0
CONV1_W_BASE = INPUT_BASE + INPUT_SIZE
CONV1_B_BASE = CONV1_W_BASE + CONV1_W_SIZE
FEAT1_BASE = CONV1_B_BASE + CONV1_C
CONV2_W_BASE = FEAT1_BASE + FEAT1_SIZE
CONV2_B_BASE = CONV2_W_BASE + CONV2_W_SIZE
FEAT2_BASE = CONV2_B_BASE + CONV2_C
FC1_W_BASE = FEAT2_BASE + FEAT2_SIZE
FC1_B_BASE = FC1_W_BASE + FC1_W_SIZE
FC1_ACT_BASE = FC1_B_BASE + FC1_OUT
FC2_W_BASE = FC1_ACT_BASE + FC1_OUT
FC2_B_BASE = FC2_W_BASE + FC2_W_SIZE
LOGITS_BASE = FC2_B_BASE + LOGITS_N

CONV2_TMP_BASE = LOGITS_BASE + LOGITS_N
CONV2_TMP_SIZE = FEAT2_H * FEAT2_W * 4

PACKED_K1 = (0, 1)
PACKED_K2 = (0, 5)
PACKED_K3 = (0, 7)
SEQ_TILE = (0, 7)


@dataclass
class PEEmitter:
    file: object
    idx: int = 0

    def op(self, opcode: str, src: list[str], dst: list[str] | None = None) -> None:
        src_txt = ", ".join(f"[{s}]" for s in src)
        if dst:
            dst_txt = ", ".join(f"[{d}]" for d in dst)
            line = f"  {opcode}, {src_txt} -> {dst_txt}"
        else:
            line = f"  {opcode}, {src_txt}"
        self.file.write("{\n")
        self.file.write(line + "\n")
        self.file.write(f"}} (idx_per_ii={self.idx})\n")
        self.idx += 1

    def relu_clamp_0_127(self, val_reg: str = "$0", pred_reg: str = "$2") -> None:
        self.op("LRS", [val_reg, "#31"], [pred_reg])
        self.op("SEL", [pred_reg, "#0", val_reg], [val_reg])
        self.op("ICMP_SGT", [val_reg, "#127"], [pred_reg])
        self.op("SEL", [pred_reg, "#127", val_reg], [val_reg])

    def wait(self) -> None:
        # Terminal blocking op: keep PC on this group and stop kernel replay.
        self.op("DATA_MOV", ["NORTH, RED"], ["$63"])


def begin_file(path: Path, title: str) -> object:
    f = path.open("w", encoding="utf-8")
    f.write("# Compiled II: 1\n")
    f.write(f"# {title}\n\n")
    return f


def begin_pe(f: object, x: int, y: int) -> PEEmitter:
    f.write(f"PE({x},{y}):\n")
    return PEEmitter(f)


def emit_conv1(pe: PEEmitter, send_north: bool) -> None:
    for oc in range(CONV1_C):
        pe.op("LOAD", [f"#{CONV1_B_BASE + oc}"], ["$5"])
        for ky in range(3):
            for kx in range(3):
                w_addr = CONV1_W_BASE + oc * 9 + ky * 3 + kx
                pe.op("LOAD", [f"#{w_addr}"], [f"${10 + ky * 3 + kx}"])

        for oy in range(FEAT1_H):
            for ox in range(FEAT1_W):
                pe.op("MOV", ["#0"], ["$3"])
                for py in range(2):
                    for px in range(2):
                        pe.op("MOV", ["$5"], ["$0"])
                        in_y = oy * 2 + py
                        in_x = ox * 2 + px
                        for ky in range(3):
                            iy = in_y + ky - 1
                            if iy < 0 or iy >= INPUT_H:
                                continue
                            for kx in range(3):
                                ix = in_x + kx - 1
                                if ix < 0 or ix >= INPUT_W:
                                    continue
                                input_addr = INPUT_BASE + iy * INPUT_W + ix
                                pe.op("LOAD", [f"#{input_addr}"], ["$1"])
                                pe.op(
                                    "MUL_ADD",
                                    ["$1", f"${10 + ky * 3 + kx}", "$0"],
                                    ["$0"],
                                )
                        pe.relu_clamp_0_127()
                        pe.op("ADD", ["$3", "$0"], ["$3"])
                pe.op("LRS", ["$3", "#2"], ["$0"])
                pe.relu_clamp_0_127()
                out_addr = FEAT1_BASE + (oy * FEAT1_W + ox) * CONV1_C + oc
                pe.op("STORE", ["$0", f"#{out_addr}"])
    if send_north:
        for i in range(FEAT1_SIZE):
            pe.op("LOAD", [f"#{FEAT1_BASE + i}"], ["$0"])
            pe.op("ADD", ["$0", "#0"], ["NORTH, RED"])


def emit_packed_feat1_receive(pe: PEEmitter) -> None:
    for i in range(FEAT1_SIZE):
        pe.op("STORE", ["SOUTH, RED", f"#{FEAT1_BASE + i}"])


def emit_conv2(pe: PEEmitter, send_north: bool) -> None:
    for oc in range(CONV2_C):
        pe.op("LOAD", [f"#{CONV2_B_BASE + oc}"], ["$5"])
        for t in range(CONV2_TMP_SIZE):
            pe.op("STORE", ["$5", f"#{CONV2_TMP_BASE + t}"])

        for ic in range(CONV1_C):
            for ky in range(3):
                for kx in range(3):
                    w_idx = ((oc * CONV1_C + ic) * 3 + ky) * 3 + kx
                    pe.op("LOAD", [f"#{CONV2_W_BASE + w_idx}"], [f"${10 + ky * 3 + kx}"])

            for oy in range(FEAT2_H):
                for ox in range(FEAT2_W):
                    for py in range(2):
                        for px in range(2):
                            tmp_idx = (oy * FEAT2_W + ox) * 4 + py * 2 + px
                            tmp_addr = CONV2_TMP_BASE + tmp_idx
                            pe.op("LOAD", [f"#{tmp_addr}"], ["$0"])
                            in_y = oy * 2 + py
                            in_x = ox * 2 + px
                            for ky in range(3):
                                iy = in_y + ky - 1
                                if iy < 0 or iy >= FEAT1_H:
                                    continue
                                for kx in range(3):
                                    ix = in_x + kx - 1
                                    if ix < 0 or ix >= FEAT1_W:
                                        continue
                                    inp_addr = FEAT1_BASE + (iy * FEAT1_W + ix) * CONV1_C + ic
                                    pe.op("LOAD", [f"#{inp_addr}"], ["$1"])
                                    pe.op(
                                        "MUL_ADD",
                                        ["$1", f"${10 + ky * 3 + kx}", "$0"],
                                        ["$0"],
                                    )
                            pe.op("STORE", ["$0", f"#{tmp_addr}"])

        for oy in range(FEAT2_H):
            for ox in range(FEAT2_W):
                pe.op("MOV", ["#0"], ["$3"])
                for py in range(2):
                    for px in range(2):
                        tmp_idx = (oy * FEAT2_W + ox) * 4 + py * 2 + px
                        pe.op("LOAD", [f"#{CONV2_TMP_BASE + tmp_idx}"], ["$0"])
                        pe.relu_clamp_0_127()
                        pe.op("ADD", ["$3", "$0"], ["$3"])
                pe.op("LRS", ["$3", "#2"], ["$0"])
                pe.relu_clamp_0_127()
                out_addr = FEAT2_BASE + (oy * FEAT2_W + ox) * CONV2_C + oc
                pe.op("STORE", ["$0", f"#{out_addr}"])
    if send_north:
        for i in range(FEAT2_SIZE):
            pe.op("LOAD", [f"#{FEAT2_BASE + i}"], ["$0"])
            pe.op("ADD", ["$0", "#0"], ["NORTH, RED"])


def emit_packed_feat2_receive(pe: PEEmitter) -> None:
    for i in range(FEAT2_SIZE):
        pe.op("STORE", ["SOUTH, RED", f"#{FEAT2_BASE + i}"])


def emit_fc(pe: PEEmitter) -> None:
    for o in range(FC1_OUT):
        pe.op("LOAD", [f"#{FC1_B_BASE + o}"], ["$0"])
        base = o * FC1_IN
        for i in range(FC1_IN):
            pe.op("LOAD", [f"#{FEAT2_BASE + i}"], ["$1"])
            pe.op("LOAD", [f"#{FC1_W_BASE + base + i}"], ["$4"])
            pe.op("MUL_ADD", ["$1", "$4", "$0"], ["$0"])
        pe.relu_clamp_0_127()
        pe.op("STORE", ["$0", f"#{FC1_ACT_BASE + o}"])

    for o in range(LOGITS_N):
        pe.op("LOAD", [f"#{FC2_B_BASE + o}"], ["$0"])
        base = o * FC1_OUT
        for i in range(FC1_OUT):
            pe.op("LOAD", [f"#{FC1_ACT_BASE + i}"], ["$1"])
            pe.op("LOAD", [f"#{FC2_W_BASE + base + i}"], ["$4"])
            pe.op("MUL_ADD", ["$1", "$4", "$0"], ["$0"])
        pe.op("STORE", ["$0", f"#{LOGITS_BASE + o}"])


def emit_relay(pe: PEEmitter, token_count: int) -> None:
    for _ in range(token_count):
        pe.op("DATA_MOV", ["SOUTH, RED"], ["NORTH, RED"])
    pe.op("DATA_MOV", ["SOUTH, RED"], ["NORTH, RED"])


def write_packed_stage_files(out_dir: Path) -> None:
    conv1 = begin_file(out_dir / "conv1_block_2x8.asm", "Packed K1 (2x8) real compute")
    for y in range(0, 2):
        for x in range(0, 8):
            pe = begin_pe(conv1, x, y)
            if (x, y) == PACKED_K1:
                emit_conv1(pe, send_north=True)
                pe.wait()
            else:
                pe.wait()
            conv1.write("\n")
    conv1.close()

    conv2 = begin_file(out_dir / "conv2_block_4x8.asm", "Packed K2 (4x8) real compute")
    for y in range(2, 6):
        for x in range(0, 8):
            pe = begin_pe(conv2, x, y)
            if (x, y) == PACKED_K2:
                emit_packed_feat1_receive(pe)
                emit_conv2(pe, send_north=True)
                pe.wait()
            elif (x, y) in {(0, 2), (0, 3), (0, 4)}:
                emit_relay(pe, FEAT1_SIZE)
            else:
                pe.wait()
            conv2.write("\n")
    conv2.close()

    fc = begin_file(out_dir / "fc_head_2x8.asm", "Packed K3 (2x8) real compute")
    for y in range(6, 8):
        for x in range(0, 8):
            pe = begin_pe(fc, x, y)
            if (x, y) == PACKED_K3:
                emit_packed_feat2_receive(pe)
                emit_fc(pe)
                pe.wait()
            elif (x, y) == (0, 6):
                emit_relay(pe, FEAT2_SIZE)
            else:
                pe.wait()
            fc.write("\n")
    fc.close()


def write_sequential_stage_files(out_dir: Path) -> None:
    conv1 = begin_file(out_dir / "conv1_block_8x8.asm", "Sequential K1 (8x8 exclusive) real compute")
    for y in range(8):
        for x in range(8):
            pe = begin_pe(conv1, x, y)
            if (x, y) == SEQ_TILE:
                emit_conv1(pe, send_north=False)
                pe.wait()
            else:
                pe.wait()
            conv1.write("\n")
    conv1.close()

    conv2 = begin_file(out_dir / "conv2_block_8x8.asm", "Sequential K2 (8x8 exclusive) real compute")
    for y in range(8):
        for x in range(8):
            pe = begin_pe(conv2, x, y)
            if (x, y) == SEQ_TILE:
                emit_conv2(pe, send_north=False)
                pe.wait()
            else:
                pe.wait()
            conv2.write("\n")
    conv2.close()

    fc = begin_file(out_dir / "fc_head_8x8.asm", "Sequential K3 (8x8 exclusive) real compute")
    for y in range(8):
        for x in range(8):
            pe = begin_pe(fc, x, y)
            if (x, y) == SEQ_TILE:
                emit_fc(pe)
                pe.wait()
            else:
                pe.wait()
            fc.write("\n")
    fc.close()


def write_total_packed(out_dir: Path) -> None:
    f = begin_file(out_dir / "lenet_packed.asm", "Packed full program (real compute)")
    for y in range(8):
        for x in range(8):
            pe = begin_pe(f, x, y)
            if (x, y) == PACKED_K1:
                emit_conv1(pe, send_north=True)
                pe.wait()
            elif (x, y) == PACKED_K2:
                emit_packed_feat1_receive(pe)
                emit_conv2(pe, send_north=True)
                pe.wait()
            elif (x, y) == PACKED_K3:
                emit_packed_feat2_receive(pe)
                emit_fc(pe)
                pe.wait()
            elif (x, y) in {(0, 2), (0, 3), (0, 4)}:
                emit_relay(pe, FEAT1_SIZE)
            elif (x, y) == (0, 6):
                emit_relay(pe, FEAT2_SIZE)
            else:
                pe.wait()
            f.write("\n")
    f.close()


def write_total_sequential(out_dir: Path) -> None:
    f = begin_file(out_dir / "lenet_seq.asm", "Sequential full program (real compute)")
    for y in range(8):
        for x in range(8):
            pe = begin_pe(f, x, y)
            if (x, y) == SEQ_TILE:
                emit_conv1(pe, send_north=False)
                emit_conv2(pe, send_north=False)
                emit_fc(pe)
                pe.wait()
            else:
                pe.wait()
            f.write("\n")
    f.close()


def main() -> None:
    out_dir = Path(__file__).resolve().parent
    write_packed_stage_files(out_dir)
    write_sequential_stage_files(out_dir)
    write_total_packed(out_dir)
    write_total_sequential(out_dir)
    print("Generated static ASM files in", out_dir)


if __name__ == "__main__":
    main()
