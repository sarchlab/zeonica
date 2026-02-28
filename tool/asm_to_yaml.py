#!/usr/bin/env python3
"""
Convert Zeonica asm files to YAML compatible with core.LoadProgramFileFromYAML.

Assumptions:
- ASM uses PE(x,y) blocks and { ... } (idx_per_ii=N) instruction groups.
- Operation lines contain "(t=..., inv_iters=...)" metadata.
"""

from __future__ import annotations

import argparse
import os
import re
from dataclasses import dataclass, field
from typing import Dict, Iterable, List, Optional, Tuple


PE_HEADER_RE = re.compile(r"^PE\((\d+),\s*(\d+)\):")
GROUP_END_RE = re.compile(r"^\}\s*\(idx_per_ii=(\d+)\)")
OP_META_RE = re.compile(r"\(t=(\d+),\s*inv_iters=(\d+)\)")
OPERAND_RE = re.compile(r"\[([^\]]+)\]")
COMPILED_II_RE = re.compile(r"#\s*Compiled II:\s*(\d+)")


@dataclass
class Operation:
    opcode: str
    time_step: int
    invalid_iterations: int
    src_operands: List[Dict[str, str]] = field(default_factory=list)
    dst_operands: List[Dict[str, str]] = field(default_factory=list)
    op_id: int = 0


@dataclass
class InstructionGroup:
    index_per_ii: int
    operations: List[Operation] = field(default_factory=list)


@dataclass
class CoreProgram:
    column: int
    row: int
    core_id: str
    instruction_groups: List[InstructionGroup] = field(default_factory=list)


def parse_operand(token: str) -> Dict[str, str]:
    parts = [p.strip() for p in token.split(",") if p.strip()]
    if not parts:
        return {"operand": "", "color": "RED"}
    if len(parts) == 1:
        return {"operand": parts[0], "color": "RED"}
    return {"operand": parts[0], "color": parts[1]}


def parse_operands(segment: str) -> List[Dict[str, str]]:
    return [parse_operand(match.group(1)) for match in OPERAND_RE.finditer(segment)]


def parse_operation_line(line: str) -> Optional[Tuple[str, int, int, List[Dict[str, str]], List[Dict[str, str]]]]:
    meta_match = OP_META_RE.search(line)
    if not meta_match:
        return None
    time_step = int(meta_match.group(1))
    invalid_iters = int(meta_match.group(2))

    op_part = line[: meta_match.start()].strip()
    if not op_part:
        return None

    if "->" in op_part:
        left, right = op_part.split("->", 1)
    else:
        left, right = op_part, ""

    left = left.strip().rstrip(",")
    if "," in left:
        opcode, src_segment = left.split(",", 1)
    else:
        opcode, src_segment = left, ""

    opcode = opcode.strip()
    src_operands = parse_operands(src_segment)
    dst_operands = parse_operands(right) if right.strip() else []

    return opcode, time_step, invalid_iters, src_operands, dst_operands


def parse_asm(lines: Iterable[str]) -> Tuple[List[CoreProgram], int, int, int]:
    compiled_ii: Optional[int] = None
    cores: Dict[Tuple[int, int], CoreProgram] = {}
    core_order: List[Tuple[int, int]] = []
    max_x = -1
    max_y = -1
    op_id = 0

    current_coord: Optional[Tuple[int, int]] = None
    in_group = False
    group_lines: List[str] = []

    for raw_line in lines:
        line = raw_line.strip()
        if not line:
            continue
        if line.startswith("#"):
            compiled_match = COMPILED_II_RE.match(line)
            if compiled_match:
                compiled_ii = int(compiled_match.group(1))
            continue

        pe_match = PE_HEADER_RE.match(line)
        if pe_match:
            x = int(pe_match.group(1))
            y = int(pe_match.group(2))
            current_coord = (x, y)
            if current_coord not in cores:
                core_id = str(len(cores))
                cores[current_coord] = CoreProgram(column=x, row=y, core_id=core_id)
                core_order.append(current_coord)
            max_x = max(max_x, x)
            max_y = max(max_y, y)
            continue

        if line == "{":
            if current_coord is None:
                raise ValueError("Found instruction group without a PE header.")
            in_group = True
            group_lines = []
            continue

        if line.startswith("}"):
            if not in_group:
                continue
            end_match = GROUP_END_RE.match(line)
            if not end_match:
                raise ValueError(f"Malformed instruction group end: {line}")
            index_per_ii = int(end_match.group(1))
            ops: List[Operation] = []
            for op_line in group_lines:
                if not op_line or op_line.startswith("#"):
                    continue
                parsed = parse_operation_line(op_line)
                if not parsed:
                    continue
                opcode, time_step, invalid_iters, src_ops, dst_ops = parsed
                ops.append(
                    Operation(
                        opcode=opcode,
                        time_step=time_step,
                        invalid_iterations=invalid_iters,
                        src_operands=src_ops,
                        dst_operands=dst_ops,
                        op_id=op_id,
                    )
                )
                op_id += 1
            cores[current_coord].instruction_groups.append(
                InstructionGroup(index_per_ii=index_per_ii, operations=ops)
            )
            in_group = False
            group_lines = []
            continue

        if in_group:
            group_lines.append(line)

    if max_x < 0 or max_y < 0:
        raise ValueError("No PE blocks found in asm.")

    columns = max_x + 1
    rows = max_y + 1
    if compiled_ii is None:
        max_idx = 0
        for core in cores.values():
            for group in core.instruction_groups:
                if group.index_per_ii > max_idx:
                    max_idx = group.index_per_ii
        compiled_ii = max_idx + 1

    ordered_cores = [cores[coord] for coord in core_order]
    return ordered_cores, columns, rows, compiled_ii


def quote(value: str) -> str:
    return '"' + value.replace('"', '\\"') + '"'


def emit_yaml(
    cores: List[CoreProgram], columns: int, rows: int, compiled_ii: int
) -> str:
    lines: List[str] = []
    lines.append("array_config:")
    lines.append(f"  columns: {columns}")
    lines.append(f"  rows: {rows}")
    lines.append(f"  compiled_ii: {compiled_ii}")
    lines.append("  cores:")

    for core in cores:
        lines.append(f"    - column: {core.column}")
        lines.append(f"      row: {core.row}")
        lines.append(f"      core_id: {quote(core.core_id)}")
        lines.append("      entries:")
        lines.append(f"        - entry_id: {quote('entry0')}")
        lines.append("          instructions:")
        for group in core.instruction_groups:
            lines.append(f"            - index_per_ii: {group.index_per_ii}")
            lines.append("              operations:")
            for op in group.operations:
                lines.append(f"                - opcode: {quote(op.opcode)}")
                lines.append(f"                  id: {op.op_id}")
                lines.append(f"                  time_step: {op.time_step}")
                lines.append(f"                  invalid_iterations: {op.invalid_iterations}")
                if op.src_operands:
                    lines.append("                  src_operands:")
                    for operand in op.src_operands:
                        lines.append(f"                    - operand: {quote(operand['operand'])}")
                        lines.append(f"                      color: {quote(operand['color'])}")
                if op.dst_operands:
                    lines.append("                  dst_operands:")
                    for operand in op.dst_operands:
                        lines.append(f"                    - operand: {quote(operand['operand'])}")
                        lines.append(f"                      color: {quote(operand['color'])}")

    return "\n".join(lines) + "\n"


def default_output_path(input_path: str) -> str:
    base, _ = os.path.splitext(input_path)
    return base + ".yaml"


def main() -> None:
    parser = argparse.ArgumentParser(description="Convert Zeonica asm to YAML.")
    parser.add_argument("asm_file", help="Path to asm file")
    parser.add_argument(
        "-o", "--output", default="", help="Output YAML path (default: input.yaml)"
    )
    args = parser.parse_args()

    with open(args.asm_file, "r", encoding="utf-8") as f:
        cores, columns, rows, compiled_ii = parse_asm(f)

    output_path = args.output or default_output_path(args.asm_file)
    yaml_text = emit_yaml(cores, columns, rows, compiled_ii)

    with open(output_path, "w", encoding="utf-8") as f:
        f.write(yaml_text)

    print(f"Wrote YAML: {output_path}")


if __name__ == "__main__":
    main()
