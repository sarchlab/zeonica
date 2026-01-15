#!/usr/bin/env python3
import argparse
import os
import sys
import yaml
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.patches as patches
from collections import defaultdict
from typing import Dict, List, Tuple, DefaultDict, Optional, Any

# Constants
PALETTE = [
    "#1f77b4",  # blue
    "#ff7f0e",  # orange
    "#2ca02c",  # green
    "#d62728",  # red
    "#9467bd",  # purple
    "#8c564b",  # brown
    "#e377c2",  # pink
    "#7f7f7f",  # gray
    "#bcbd22",  # olive
    "#17becf",  # cyan
]

PECoord = Tuple[int, int]

class ArrowInfo:
    """Information about a single boundary arrow."""
    def __init__(self, pe_coord: PECoord, side: str, direction: str, inst: 'Instruction', color: str):
        self.pe_coord = pe_coord
        self.side = side  # 'NORTH', 'SOUTH', 'EAST', 'WEST'
        self.direction = direction  # 'in' or 'out'
        self.inst = inst
        self.color = color

class BoundaryArrows:
    """
    Data structure to manage boundary arrows for each PE boundary.
    Key: (pe_coord, side) -> List[ArrowInfo]
    """
    def __init__(self):
        # Changed key to remove direction, grouping all arrows for a boundary together
        self.arrows: Dict[Tuple[PECoord, str], List[ArrowInfo]] = defaultdict(list)
    
    def add_arrow(self, pe_coord: PECoord, side: str, direction: str, inst: 'Instruction', color: str):
        """
        Add an arrow to the specified boundary.
        
        Args:
            pe_coord: PE coordinates (column, row)
            side: Boundary side ('NORTH', 'SOUTH', 'EAST', 'WEST')
            direction: Arrow direction ('in' or 'out')
            inst: Instruction this arrow belongs to (for color determination)
            color: Color of the arrow
        """
        arrow_info = ArrowInfo(pe_coord, side, direction, inst, color)
        # Store by (pe_coord, side) only, mixing 'in' and 'out'
        self.arrows[(pe_coord, side)].append(arrow_info)
    
    def get_arrows(self, pe_coord: PECoord, side: str) -> List[ArrowInfo]:
        """Get all arrows for a specific boundary."""
        return self.arrows.get((pe_coord, side), [])
    
    def get_all_arrows_by_boundary(self) -> Dict[Tuple[PECoord, str], List[ArrowInfo]]:
        """Get all arrows organized by boundary."""
        return dict(self.arrows)

class Operand:
    def __init__(self, operand_str: str, color: str = None):
        self.raw = operand_str
        self.kind = self._determine_kind(operand_str)
        self.value = operand_str
        self.side = None
        if self.kind == 'port':
            self.side = self._get_side(operand_str)

    def _determine_kind(self, s: str) -> str:
        if s.startswith('$'): return 'reg'
        if s.startswith('#'): return 'const'
        if any(d in s for d in ['NORTH', 'SOUTH', 'EAST', 'WEST']): return 'port'
        return 'other'

    def _get_side(self, s: str) -> str:
        for d in ['NORTH', 'SOUTH', 'EAST', 'WEST']:
            if d in s:
                return d
        return None

class Instruction:
    def __init__(self, opcode: str, src_operands: List[Dict], dst_operands: List[Dict], timestep: int, is_wrapped: bool = False):
        self.opcode = opcode
        self.srcs = [Operand(o.get('operand', '')) for o in src_operands]
        self.dsts = [Operand(o.get('operand', '')) for o in dst_operands]
        self.timestep = timestep
        self.is_wrapped = is_wrapped

    def has_src_reg(self) -> bool:
        return any(op.kind == 'reg' for op in self.srcs)

    def has_dst_reg(self) -> bool:
        return any(op.kind == 'reg' for op in self.dsts)

def parse_yaml_file(yaml_path: str) -> Tuple[Dict[int, Dict[PECoord, List[Instruction]]], int, int, int]:
    with open(yaml_path, 'r') as f:
        data = yaml.safe_load(f)

    config = data.get('array_config', {})
    cols = config.get('columns', 0)
    rows = config.get('rows', 0)
    compiled_ii = config.get('compiled_ii', 1)

    instructions_by_t = defaultdict(lambda: defaultdict(list))

    for core in config.get('cores', []):
        c = core.get('column')
        r = core.get('row')
        pe_coord = (c, r)
        
        for entry in core.get('entries', []):
            for inst_group in entry.get('instructions', []):
                ts = inst_group.get('index_per_ii')
                
                # Skip if timestep is missing
                if ts is None:
                    continue
                
                # Determine if wrapped
                is_wrapped = False
                effective_ts = ts
                if compiled_ii > 0:
                    if ts >= compiled_ii:
                        is_wrapped = True
                        effective_ts = ts % compiled_ii
                
                for op in inst_group.get('operations', []):
                    opcode = op.get('opcode')
                    srcs = op.get('src_operands', [])
                    dsts = op.get('dst_operands', [])
                    
                    instr = Instruction(opcode, srcs, dsts, ts, is_wrapped)
                    instructions_by_t[effective_ts][pe_coord].append(instr)

    return instructions_by_t, cols, rows, compiled_ii

def lighten_color(color, amount=0.5):
    """
    Lightens the given color by multiplying (1-luminosity) by the given amount.
    Input can be matplotlib color string, hex string, or RGB tuple.
    
    Actually, a simpler way to 'fade' is to blend with white or reduce alpha.
    Matplotlib colors can be converted to RGBA.
    """
    try:
        c = matplotlib.colors.to_rgb(color)
        # Blend with white: new = old * (1 - amount) + white * amount
        # Wait, 'amount' logic: 0.5 means 50% white.
        white = (1.0, 1.0, 1.0)
        new_c = tuple(c[i] * (1 - amount) + white[i] * amount for i in range(3))
        return new_c
    except ValueError:
        return color

def prepare_draw_data(
    pe_to_insts: Dict[PECoord, List[Instruction]]
) -> Tuple[List[Dict[str, Any]], BoundaryArrows]:
    """
    Prepare data for drawing: calculates instruction text positions and collects boundary arrows.
    
    Returns:
        instruction_texts: List of dicts with keys 'x', 'y', 'text', 'color', 'fontweight'
        boundary_arrows: BoundaryArrows object containing all arrows to be drawn
    """
    instruction_texts = []
    boundary_arrows = BoundaryArrows()
    
    for (c, r), insts in pe_to_insts.items():
        x = c
        y = r 
        
        # Layout for text
        num_insts = len(insts)
        if num_insts == 0: continue
        
        # Calculate spacing
        if num_insts == 1:
            start_y = y + 0.5
            step_y = 0.0
        else:
            top_y = y + 0.7
            bottom_y = y + 0.3
            start_y = top_y
            step_y = (top_y - bottom_y) / (num_insts - 1)
        
        for i, inst in enumerate(insts):
            # Determine Color
            base_color = PALETTE[i % len(PALETTE)]
            color = base_color
            if inst.is_wrapped:
                color = lighten_color(base_color, 0.6)
            
            # Apply abbreviations to opcode
            opcode_map = {
                'GRANT_ONCE': 'GONCE',
                'GRANT_PREDICATE': 'GPRED',
                'DATA_MOV': 'MOV',
                'CTRL_MOV': 'CMOV'
            }
            display_opcode = opcode_map.get(inst.opcode, inst.opcode)

            # Build Text: Opcode + Arrows
            text = display_opcode
            if inst.has_src_reg():
                text += "↑"
            if inst.has_dst_reg():
                text += "↓"
                
            # Position Text
            text_y = start_y - (i * step_y)
            
            instruction_texts.append({
                'x': x + 0.5,
                'y': text_y,
                'text': text,
                'color': color,
                'fontweight': 'bold'
            })
            
            # Collect arrow information
            pe_coord = (c, r)
            
            # SRC -> Incoming Arrow
            for src in inst.srcs:
                if src.kind == 'port' and src.side:
                    boundary_arrows.add_arrow(pe_coord, src.side, 'in', inst, color)
            
            # DST -> Outgoing Arrow
            for dst in inst.dsts:
                if dst.kind == 'port' and dst.side:
                    boundary_arrows.add_arrow(pe_coord, dst.side, 'out', inst, color)
                    
    return instruction_texts, boundary_arrows

def draw_grid(
    t: int,
    cols: int,
    rows: int
) -> Tuple[Any, Any]:
    """
    Draw the grid background, coordinate labels, and title.
    Includes a ring of driver cells around the main grid.
    Returns (fig, ax).
    """
    # Calculate figure size (add 2 for driver ring on both sides)
    figsize_per_cell = 2.0
    width = max(3, (cols + 2) * figsize_per_cell)
    height = max(3, (rows + 2) * figsize_per_cell)
    
    fig, ax = plt.subplots(figsize=(width, height))
    # Adjust limits to include driver ring: from -1 to cols+1 and rows+1
    ax.set_xlim(-1, cols + 1)
    ax.set_ylim(-1, rows + 1)
    ax.set_aspect("equal")
    ax.axis("off")

    # Draw Grid (from -1 to cols and -1 to rows to include driver ring)
    for y in range(-1, rows + 1):
        for x in range(-1, cols + 1):
            # Determine if this is a driver cell (outside the main grid)
            is_driver = (x < 0 or x >= cols or y < 0 or y >= rows)
            
            if is_driver:
                # Driver cell style
                facecolor = "#e0e0e0" # Light grey
                edgecolor = "#666666" # Darker grey
                linestyle = "--"      # Dashed line
            else:
                # Normal cell style
                facecolor = "#f9f9f9"
                edgecolor = "black"
                linestyle = "-"

            # Rectangle
            rect = patches.Rectangle(
                (x, y), 1, 1, 
                linewidth=1.0, 
                edgecolor=edgecolor, 
                facecolor=facecolor,
                linestyle=linestyle
            )
            ax.add_patch(rect)
            
            # Coord label
            ax.text(x + 0.05, y + 0.95, f"{x},{y}", ha="left", va="top", fontsize=8, color="#888")

    ax.set_title(f"Timestep: {t}", fontsize=14)

    return fig, ax

def draw_instructions(
    ax: Any,
    instruction_texts: List[Dict[str, Any]],
    base_font_size: int = 10
):
    """
    Draw instruction text using pre-calculated positions.
    """
    for item in instruction_texts:
        ax.text(item['x'], item['y'], item['text'], 
                ha="center", va="center", 
                fontsize=base_font_size, 
                color=item['color'], 
                fontweight=item['fontweight'])

def draw_arrows(
    ax: Any,
    boundary_arrows: BoundaryArrows
):
    """
    Draw boundary arrows evenly distributed along each boundary.
    """
    # Constants for arrow distribution
    MARGIN = 0.1  # Margin from cell edges
    
    for (pe_coord, side), arrow_list in boundary_arrows.get_all_arrows_by_boundary().items():
        if len(arrow_list) == 0:
            continue
        
        x, y = pe_coord
        num_arrows = len(arrow_list)
        
        # Calculate positions for arrows evenly distributed along the boundary
        if side in ['NORTH', 'SOUTH']:
            # Horizontal distribution for North/South boundaries
            if num_arrows == 1:
                positions = [x + 0.5]
            else:
                start = x + MARGIN
                end = x + 1 - MARGIN
                positions = [start + (end - start) * i / (num_arrows - 1) for i in range(num_arrows)]
            
            # Draw each arrow at its position
            for i, arrow_info in enumerate(arrow_list):
                arrow_x_center = positions[i]
                # text_y parameter is ignored for North/South, but we need to provide it
                dummy_text_y = y + 0.5
                
                draw_boundary_arrow(ax, x, y, side, arrow_info.direction, arrow_info.color, 
                                   dummy_text_y, arrow_x_center)
        
        elif side in ['EAST', 'WEST']:
            # Vertical distribution for East/West boundaries
            if num_arrows == 1:
                positions = [y + 0.5]
            else:
                start = y + MARGIN
                end = y + 1 - MARGIN
                positions = [start + (end - start) * i / (num_arrows - 1) for i in range(num_arrows)]
            
            # Draw each arrow at its position
            for i, arrow_info in enumerate(arrow_list):
                text_y = positions[i]
                # arrow_x_center parameter is ignored for East/West, but we need to provide it
                dummy_arrow_x_center = x + 0.5
                
                draw_boundary_arrow(ax, x, y, side, arrow_info.direction, arrow_info.color, 
                                   text_y, dummy_arrow_x_center)

def draw_grid_for_timestep(
    t: int,
    cols: int,
    rows: int,
    pe_to_insts: Dict[PECoord, List[Instruction]],
    out_path: str,
    base_font_size: int = 10
):
    # Prepare data
    instruction_texts, boundary_arrows = prepare_draw_data(pe_to_insts)

    # Draw grid background, labels, and title
    fig, ax = draw_grid(t, cols, rows)
    
    # Draw instructions
    draw_instructions(ax, instruction_texts, base_font_size)
    
    # Draw arrows
    draw_arrows(ax, boundary_arrows)

    plt.tight_layout()
    plt.savefig(out_path, dpi=150)
    plt.close(fig)

def draw_boundary_arrow(ax, x, y, side, direction, color, text_y, arrow_x_center):
    # direction: 'in' (into core) or 'out' (out of core)
    # side: boundary location
    # text_y: y-coordinate for West/East arrows (aligned with instruction text)
    # arrow_x_center: x-coordinate center for North/South arrows
    
    # Constants for arrow geometry
    MARGIN = 0.05
    LENGTH = 0.15 # Length of arrow
    
    # Determine start and end points based on side and direction
    # Must be COMPLETELY INSIDE the cell [x, x+1] x [y, y+1]
    
    if side == 'WEST':
        # Left edge
        arrow_y = text_y
        if direction == 'in':
            # From West -> Pointing East (Right)
            # Start near left edge, End towards center
            start = (x + MARGIN, arrow_y)
            end = (x + MARGIN + LENGTH, arrow_y)
        else:
            # To West -> Pointing West (Left)
            # Start towards center, End near left edge
            start = (x + MARGIN + LENGTH, arrow_y)
            end = (x + MARGIN, arrow_y)
            
    elif side == 'EAST':
        # Right edge
        arrow_y = text_y
        if direction == 'in':
            # From East -> Pointing West (Left)
            # Start near right edge, End towards center
            start = (x + 1 - MARGIN, arrow_y)
            end = (x + 1 - MARGIN - LENGTH, arrow_y)
        else:
            # To East -> Pointing East (Right)
            # Start towards center, End near right edge
            start = (x + 1 - MARGIN - LENGTH, arrow_y)
            end = (x + 1 - MARGIN, arrow_y)
            
    elif side == 'NORTH':
        # Top edge
        arrow_x = arrow_x_center
        if direction == 'in':
            # From North -> Pointing South (Down)
            # Start near top edge, End towards center
            start = (arrow_x, y + 1 - MARGIN)
            end = (arrow_x, y + 1 - MARGIN - LENGTH)
        else:
            # To North -> Pointing North (Up)
            # Start towards center, End near top edge
            start = (arrow_x, y + 1 - MARGIN - LENGTH)
            end = (arrow_x, y + 1 - MARGIN)
            
    elif side == 'SOUTH':
        # Bottom edge
        arrow_x = arrow_x_center
        if direction == 'in':
            # From South -> Pointing North (Up)
            # Start near bottom edge, End towards center
            start = (arrow_x, y + MARGIN)
            end = (arrow_x, y + MARGIN + LENGTH)
        else:
            # To South -> Pointing South (Down)
            # Start towards center, End near bottom edge
            start = (arrow_x, y + MARGIN + LENGTH)
            end = (arrow_x, y + MARGIN)
            
    else:
        return

    ax.annotate(
        "",
        xy=end,
        xytext=start,
        arrowprops=dict(
            arrowstyle="-|>",
            lw=2,
            color=color,
            shrinkA=0,
            shrinkB=0
        )
    )


def draw_yaml(yaml_file: str, out_dir: str) -> str:
    """
    Programmatically draw YAML schedule visualization.
    
    Args:
        yaml_file: Path to YAML file
        out_dir: Base output directory (a subdirectory will be created based on filename)
    
    Returns:
        Final output directory path (including the filename stem subdirectory)
    """
    if not os.path.exists(yaml_file):
        raise FileNotFoundError(f"File {yaml_file} not found")

    insts_by_t, cols, rows, ii = parse_yaml_file(yaml_file)
    
    # Determine output directory
    stem = os.path.splitext(os.path.basename(yaml_file))[0]
    final_out_dir = os.path.join(out_dir, stem)
    os.makedirs(final_out_dir, exist_ok=True)
    
    print(f"Generating images for {stem} (cols={cols}, rows={rows}, II={ii})...")
    
    # Generate images for 0 to II-1
    # (Or max timestep if II is not set? Config says compiled_ii usually exists)
    if ii is None or ii == 0:
        max_t = max(insts_by_t.keys()) if insts_by_t else 0
        rng = range(max_t + 1)
    else:
        rng = range(ii)
        
    for t in rng:
        out_path = os.path.join(final_out_dir, f"timestep_{t:02d}.png")
        draw_grid_for_timestep(t, cols, rows, insts_by_t[t], out_path)
        
    print(f"Done. Images saved to {final_out_dir}")
    return final_out_dir

def main():
    parser = argparse.ArgumentParser(description="Visualize YAML schedule.")
    parser.add_argument("yaml_file", help="Path to YAML file")
    parser.add_argument("--out-dir", default="output/yaml_drawer", help="Output directory")
    
    args = parser.parse_args()
    
    draw_yaml(args.yaml_file, args.out_dir)

if __name__ == "__main__":
    main()
