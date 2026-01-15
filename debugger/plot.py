#!/usr/bin/env python3
import os
import sys
import json
import argparse
import re
from collections import defaultdict
import importlib.util
from typing import Dict, List, Tuple, Any, Optional
import matplotlib.pyplot as plt

# Add current directory to sys.path to ensure local imports work
sys.path.append(os.getcwd())

# Import debugger/dot.py
try:
    import debugger.dot as dot_drawer
except ImportError:
    # Try importing assuming we are in the root
    spec = importlib.util.spec_from_file_location("dot_drawer", "debugger/dot.py")
    if spec and spec.loader:
        dot_drawer = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(dot_drawer)
    else:
        raise ImportError("Could not import debugger/dot.py")

# Import debugger/yaml-drawer.py (handling hyphen)
spec = importlib.util.spec_from_file_location("yaml_drawer", "debugger/yaml-drawer.py")
if spec and spec.loader:
    yaml_drawer = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(yaml_drawer)
else:
    raise ImportError("Could not import debugger/yaml-drawer.py")

PECoord = Tuple[int, int]

# Red Palette for instructions/arrows
RED_PALETTE = [
    "#ff0000", # Red
    "#cc0000", # Darker Red
    "#990000", # Even Darker
    "#ff4444", # Lighter Red
    "#880000", # Darkest
]

class LogParser:
    def __init__(self):
        self.events_by_time = defaultdict(lambda: {'insts': [], 'flows': []})
        self.max_x = 0
        self.max_y = 0

    def parse_coord_str(self, s: str) -> Tuple[Optional[str], Optional[int], Optional[int]]:
        """
        Parse string like 'Device.Tile[1][0].Core.South'
        Returns (type, x, y) or (type, index, None) for drivers
        Types: 'Tile', 'South', 'North', 'West', 'East' (Driver types)
        """
        # Tile check: Device.Tile[y][x]
        m = re.search(r'Device\.Tile\[(\d+)\]\[(\d+)\]', s)
        if m:
            y, x = int(m.group(1)), int(m.group(2))
            self.max_x = max(self.max_x, x)
            self.max_y = max(self.max_y, y)
            return 'Tile', x, y
        
        # Driver checks
        m = re.search(r'Driver\.Device(South|North|West|East)\[(\d+)\]', s)
        if m:
            side = m.group(1)
            idx = int(m.group(2))
            return side, idx, None
            
        return None, None, None

    def get_coord(self, type_str: str, x: int, y: Optional[int], rows: int, cols: int) -> PECoord:
        if type_str == 'Tile':
            return (x, y)
        elif type_str == 'South':
            return (x, -1)
        elif type_str == 'North':
            return (x, rows)
        elif type_str == 'West':
            return (-1, x) # Index is row
        elif type_str == 'East':
            return (cols, x) # Index is row
        return (0, 0) # Fallback

    def parse_file(self, filepath: str):
        with open(filepath, 'r') as f:
            for line in f:
                line = line.strip()
                if not line: continue
                try:
                    data = json.loads(line)
                except json.JSONDecodeError:
                    continue

                msg = data.get('msg')
                ts = data.get('Time')
                if ts is None: continue
                
                # Treat float timestamps as int for visualization steps
                ts = int(float(ts))

                if msg == 'Inst':
                    self.events_by_time[ts]['insts'].append(data)
                    # Update dimensions from Inst too
                    if 'X' in data and 'Y' in data:
                        self.max_x = max(self.max_x, data['X'])
                        self.max_y = max(self.max_y, data['Y'])
                        
                elif msg == 'DataFlow':
                    self.events_by_time[ts]['flows'].append(data)
                    # Update dimensions from Src/Dst/From/To
                    for key in ['Src', 'Dst', 'From', 'To']:
                        if key in data and data[key] != 'None':
                            self.parse_coord_str(data[key])

    def get_dims(self) -> Tuple[int, int]:
        # Rows and Cols are max_index + 1
        return self.max_x + 1, self.max_y + 1

def get_arrow_side(src: PECoord, dst: PECoord) -> Optional[str]:
    xs, ys = src
    xd, yd = dst
    if xd > xs: return 'EAST'
    if xd < xs: return 'WEST'
    if yd > ys: return 'NORTH'
    if yd < ys: return 'SOUTH'
    return None

def process_log_and_draw(log_path: str, dfg_path: str, out_dir: str = "output/viz"):
    # Check inputs
    if not os.path.exists(log_path):
        print(f"Error: Log file {log_path} not found")
        return
    if not os.path.exists(dfg_path):
        print(f"Error: DFG file {dfg_path} not found")
        return

    # 1. Parse Log
    lp = LogParser()
    lp.parse_file(log_path)
    cols, rows = lp.get_dims()
    print(f"Parsed log. Grid dimensions: {cols}x{rows}. Max timestep: {max(lp.events_by_time.keys()) if lp.events_by_time else 0}")
    
    # Create output dirs
    os.makedirs(os.path.join(out_dir, "dfg"), exist_ok=True)
    os.makedirs(os.path.join(out_dir, "mesh"), exist_ok=True)
    
    # Initialize Instruction Counts
    instruction_counts = defaultdict(int)
    
    # Get all timesteps sorted
    timesteps = sorted(lp.events_by_time.keys())
    
    opcode_map = {
        'GRANT_ONCE': 'GONCE',
        'GRANT_PREDICATE': 'GPRED',
        'DATA_MOV': 'MOV',
        'CTRL_MOV': 'CMOV'
    }

    for t in timesteps:
        events = lp.events_by_time[t]
        insts = events['insts']
        flows = events['flows']
        
        # --- DFG Visualization ---
        current_inst_ids = []
        for inst in insts:
            iid = inst.get('ID')
            if iid is not None:
                instruction_counts[iid] += 1
                current_inst_ids.append(iid)
        
        dfg_out_path = os.path.join(out_dir, "dfg", f"dfg_{t:04d}")
        dot_drawer.draw_dfg_with_counts(
            instruction_counts, 
            yaml_path=dfg_path, 
            output_path=dfg_out_path, 
            highlight=current_inst_ids
        )
        
        # --- Mesh Visualization ---
        boundary_arrows = yaml_drawer.BoundaryArrows()
        
        # Process Insts for text
        insts_by_pe = defaultdict(list)
        for inst in insts:
            x = inst.get('X')
            y = inst.get('Y')
            if x is not None and y is not None:
                insts_by_pe[(x, y)].append(inst)
        
        instruction_texts = []
        for (x, y), pe_insts in insts_by_pe.items():
            num_insts = len(pe_insts)
            if num_insts == 0: continue
            
            # Layout logic
            if num_insts == 1:
                start_y = y + 0.5
                step_y = 0.0
            else:
                top_y = y + 0.7
                bottom_y = y + 0.3
                start_y = top_y
                step_y = (top_y - bottom_y) / (num_insts - 1)
            
            for i, inst in enumerate(pe_insts):
                opcode = inst.get('OpCode', 'UNKNOWN')
                display_opcode = opcode_map.get(opcode, opcode)
                
                # Determine color
                color = RED_PALETTE[i % len(RED_PALETTE)]
                
                # Position
                text_y = start_y - (i * step_y)
                
                instruction_texts.append({
                    'x': x + 0.5,
                    'y': text_y,
                    'text': display_opcode,
                    'color': color,
                    'fontweight': 'bold'
                })

        # Process Flows for Arrows
        for flow in flows:
            behavior = flow.get('Behavior')
            # Normalize to Src -> Dst
            src_str = flow.get('Src') or flow.get('From')
            dst_str = flow.get('Dst') or flow.get('To')
            
            # Special case for Collect with "None" destination (it means Driver collected it)
            if behavior == 'Collect' and dst_str == 'None':
                # Src is the driver component that collected it
                # We want to visualize it arriving AT the driver
                # So we treat Src as the Dst coordinate
                dst_str = src_str
                # Where did it come from? 
                # If Driver is East, it came from West neighbor.
                # We need to construct a fake Src coordinate or just use side logic.
                # Let's rely on standard parsing.
                src_str = None # We will infer src side from dst location
            
            if not dst_str: continue

            # Parse Dst
            dtype, dx, dy = lp.parse_coord_str(dst_str)
            dst_coord = lp.get_coord(dtype, dx, dy, rows, cols)
            
            src_coord = None
            if src_str:
                stype, sx, sy = lp.parse_coord_str(src_str)
                src_coord = lp.get_coord(stype, sx, sy, rows, cols)

            # Determine Arrow placement
            # We want to place arrows on boundaries.
            
            # Default color
            color = RED_PALETTE[0]

            if behavior == 'Send' and src_coord and dst_coord:
                # Arrow leaving Src
                side = get_arrow_side(src_coord, dst_coord)
                if side:
                    boundary_arrows.add_arrow(src_coord, side, 'out', None, color)
            
            elif behavior == 'FeedIn' and src_coord and dst_coord:
                # FeedIn (Driver -> Tile) visualized as Arrow leaving Driver (Src)
                side = get_arrow_side(src_coord, dst_coord)
                if side:
                    boundary_arrows.add_arrow(src_coord, side, 'out', None, color)

            elif behavior == 'Recv' and src_coord and dst_coord:
                # Arrow entering Dst
                # Determine side of Dst facing Src
                # Direction from Dst to Src gives the side of Dst
                # e.g. Src is North of Dst. Arrow enters from North. Side=NORTH.
                side = get_arrow_side(dst_coord, src_coord)
                if side:
                    boundary_arrows.add_arrow(dst_coord, side, 'in', None, color)

            elif behavior == 'Collect':
                # Arriving at Driver (dst_coord)
                # Infer side based on Driver location
                # DriverSouth (y=-1) -> Comes from North (Side=NORTH)
                # DriverNorth (y=rows) -> Comes from South (Side=SOUTH)
                # DriverWest (x=-1) -> Comes from East (Side=EAST)
                # DriverEast (x=cols) -> Comes from West (Side=WEST)
                if dtype == 'South': side = 'NORTH'
                elif dtype == 'North': side = 'SOUTH'
                elif dtype == 'West': side = 'EAST'
                elif dtype == 'East': side = 'WEST'
                else: side = None
                
                if side:
                    boundary_arrows.add_arrow(dst_coord, side, 'in', None, color)

        # Draw Mesh
        mesh_out_path = os.path.join(out_dir, "mesh", f"mesh_{t:04d}.png")
        fig, ax = yaml_drawer.draw_grid(t, cols, rows)
        yaml_drawer.draw_instructions(ax, instruction_texts)
        yaml_drawer.draw_arrows(ax, boundary_arrows)
        
        plt.tight_layout()
        plt.savefig(mesh_out_path, dpi=150)
        plt.close(fig)

    print(f"Visualization complete. Output in {out_dir}")

def main():
    parser = argparse.ArgumentParser(description="Visualize execution trace.")
    parser.add_argument("--log", required=True, help="Path to JSON log file")
    parser.add_argument("--dfg", required=True, help="Path to DFG YAML file")
    parser.add_argument("--out-dir", default="output/viz", help="Output directory")
    
    args = parser.parse_args()
    process_log_and_draw(args.log, args.dfg, args.out_dir)

if __name__ == "__main__":
    main()
