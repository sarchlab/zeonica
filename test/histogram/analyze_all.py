#!/usr/bin/env python3
"""
Unified log analysis script
Includes:
1. PE utilization analysis
2. Link utilization analysis
3. Backpressure analysis (including port-level)
4. Generate all heatmaps
5. Generate timeline trace visualization
"""

import json
import sys
import re
import numpy as np
import matplotlib.pyplot as plt
import seaborn as sns
from collections import defaultdict

# Set font for plots
plt.rcParams['font.sans-serif'] = ['DejaVu Sans', 'Arial', 'Liberation Sans']
plt.rcParams['axes.unicode_minus'] = False

# CGRA grid size (will be read from log file)
GRID_ROWS = None
GRID_COLS = None

def parse_pe_coord(pe_key):
    """Parse coordinates from PE key, e.g., PE(2,3) -> (2, 3)"""
    match = re.match(r'PE\((\d+),(\d+)\)', pe_key)
    if match:
        return int(match.group(1)), int(match.group(2))
    return None, None

def parse_link_info(link_key):
    """Parse link information, returns source PE, destination PE, and direction"""
    if '->Memory' in link_key:
        match = re.match(r'PE\((\d+),(\d+)\)->Memory', link_key)
        if match:
            return (int(match.group(1)), int(match.group(2))), 'Memory', 'Send'
    elif 'Memory->' in link_key:
        match = re.match(r'Memory->PE\((\d+),(\d+)\)', link_key)
        if match:
            return (int(match.group(1)), int(match.group(2))), 'Memory', 'Recv'
    elif '->' in link_key:
        parts = link_key.split('->')
        if len(parts) == 2:
            src_part = parts[0]
            dst_part = parts[1]
            if src_part.startswith('PE(') and not dst_part.startswith('PE('):
                match = re.match(r'PE\((\d+),(\d+)\)', src_part)
                if match:
                    return (int(match.group(1)), int(match.group(2))), dst_part, 'Send'
            elif dst_part.startswith('PE(') and not src_part.startswith('PE('):
                match = re.match(r'PE\((\d+),(\d+)\)', dst_part)
                if match:
                    return (int(match.group(1)), int(match.group(2))), src_part, 'Recv'
    return None, None, None

def read_grid_config(log_file):
    """Read grid configuration from log file"""
    global GRID_ROWS, GRID_COLS
    rows = None
    cols = None
    
    with open(log_file, 'r') as f:
        for line in f:
            line = line.strip()
            if line.startswith('GRID_ROWS:'):
                try:
                    rows = int(line.split(':', 1)[1].strip())
                except ValueError:
                    pass
            elif line.startswith('GRID_COLS:'):
                try:
                    cols = int(line.split(':', 1)[1].strip())
                except ValueError:
                    pass
            # Stop reading config section when we hit the separator
            if line.startswith('===') and rows is not None and cols is not None:
                break
    
    if rows is None or cols is None:
        print(f"Warning: Could not read grid configuration from log file, using defaults (4x4)")
        rows = 4
        cols = 4
    
    GRID_ROWS = rows
    GRID_COLS = cols
    print(f"Grid configuration: {GRID_ROWS} rows x {GRID_COLS} cols")
    return rows, cols

def analyze_all(log_file):
    """Unified analysis of all metrics"""
    # Read grid configuration from log file
    read_grid_config(log_file)
    
    # PE statistics
    pe_stats = defaultdict(lambda: {
        'exec_events': [],
        'blocked_events': [],
        'idle_events': [],
        'instructions': defaultdict(int)
    })
    
    # Link statistics
    link_stats = defaultdict(lambda: {
        'send_events': [],
        'recv_events': [],
        'send_count': 0,
        'recv_count': 0
    })
    
    # Backpressure statistics
    pe_backpressure = defaultdict(lambda: {
        'total_cycles': set(),
        'by_type': defaultdict(set),
        'by_reason': defaultdict(int)
    })
    
    # Port-level backpressure
    port_backpressure = defaultdict(lambda: {
        'total_cycles': set(),
        'by_type': defaultdict(set),
        'by_direction': defaultdict(set)
    })
    
    max_time = 0
    
    print("=" * 70)
    print("Unified Log Analysis")
    print("=" * 70)
    print(f"Analyzing log file: {log_file}\n")
    
    # Read log file
    with open(log_file, 'r') as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith('=') or line.startswith('GRID_ROWS:') or line.startswith('GRID_COLS:'):
                continue
            
            try:
                entry = json.loads(line)
                msg = entry.get('msg', '')
                time = float(entry.get('Time', 0))
                x = entry.get('X', -1)
                y = entry.get('Y', -1)
                
                if time > max_time:
                    max_time = time
                
                # PE utilization analysis
                if x >= 0 and y >= 0:
                    pe_key = f"PE({x},{y})"
                    pe = pe_stats[pe_key]
                    
                    if 'first_time' not in pe:
                        pe['first_time'] = time
                    pe['last_time'] = max(pe.get('last_time', 0), time)
                    
                    # Execution events
                    if msg in ['Inst_Exec', 'PHI_Exec', 'GPRED_Exec', 'NOT_Exec']:
                        pe['exec_events'].append(time)
                        opcode = entry.get('OpCode', msg)
                        pe['instructions'][opcode] += 1
                    elif msg == 'Memory':
                        behavior = entry.get('Behavior', '')
                        if behavior in ['WriteMemory', 'ReadMemory']:
                            pe['exec_events'].append(time)
                            pe['instructions'][behavior] += 1
                    elif msg == 'DataFlow':
                        behavior = entry.get('Behavior', '')
                        if behavior in ['Send', 'Recv']:
                            pe['exec_events'].append(time)
                            pe['instructions'][f'DataFlow_{behavior}'] += 1
                    elif msg == 'InstGroup_Blocked':
                        pe['blocked_events'].append(time)
                    elif msg == 'InstGroup_NotRun':
                        pe['idle_events'].append(time)
                
                # Link utilization analysis
                if 'Memory' in msg:
                    behavior = entry.get('Behavior', '')
                    if behavior in ['Send', 'WriteMemory']:
                        link_key = f"PE({x},{y})->Memory"
                        link_stats[link_key]['send_events'].append(time)
                        link_stats[link_key]['send_count'] += 1
                    elif behavior in ['Recv', 'ReadMemory']:
                        link_key = f"Memory->PE({x},{y})"
                        link_stats[link_key]['recv_events'].append(time)
                        link_stats[link_key]['recv_count'] += 1
                
                if msg == 'DataFlow':
                    behavior = entry.get('Behavior', '')
                    direction = entry.get('Direction', '')
                    
                    if behavior == 'Send' and x >= 0 and y >= 0:
                        link_key = f"PE({x},{y})->{direction}"
                        link_stats[link_key]['send_events'].append(time)
                        link_stats[link_key]['send_count'] += 1
                    elif behavior == 'Recv' and x >= 0 and y >= 0:
                        link_key = f"{direction}->PE({x},{y})"
                        link_stats[link_key]['recv_events'].append(time)
                        link_stats[link_key]['recv_count'] += 1
                
                # Backpressure analysis
                if msg == 'Backpressure':
                    bp_type = entry.get('Type', 'Unknown')
                    direction = entry.get('Direction', '')
                    color = entry.get('Color', '')
                    
                    if x >= 0 and y >= 0:
                        pe_key = f"PE({x},{y})"
                        cycle = int(time)
                        
                        pe_backpressure[pe_key]['total_cycles'].add(cycle)
                        pe_backpressure[pe_key]['by_type'][bp_type].add(cycle)
                        
                        reason = entry.get('Reason', 'Unknown')
                        pe_backpressure[pe_key]['by_reason'][reason] += 1
                        
                        # Port-level backpressure
                        if direction and direction != 'Unknown':
                            # Port key format: PE(x,y):Direction
                            port_key = f"PE({x},{y}):{direction}"
                            port_backpressure[port_key]['total_cycles'].add(cycle)
                            port_backpressure[port_key]['by_type'][bp_type].add(cycle)
                            port_backpressure[port_key]['by_direction'][direction].add(cycle)
                        
                        # For backpressure without direction, try to infer from other fields
                        elif bp_type in ['SendFailed', 'RecvSkipped', 'SendBufBusy', 'CheckFlagsFailed']:
                            # These types are usually related to specific ports
                            inferred_dir = entry.get('Direction', '')
                            if inferred_dir and inferred_dir != 'Unknown':
                                port_key = f"PE({x},{y}):{inferred_dir}"
                                port_backpressure[port_key]['total_cycles'].add(cycle)
                                port_backpressure[port_key]['by_type'][bp_type].add(cycle)
                                port_backpressure[port_key]['by_direction'][inferred_dir].add(cycle)
                
                # Compatible with old format
                elif msg in ['InstGroup_Blocked', 'InstGroup_NotRun', 'CheckFlags_Failed']:
                    if x >= 0 and y >= 0:
                        pe_key = f"PE({x},{y})"
                        cycle = int(time)
                        
                        pe_backpressure[pe_key]['total_cycles'].add(cycle)
                        if msg == 'InstGroup_Blocked':
                            pe_backpressure[pe_key]['by_type']['InstGroupBlocked'].add(cycle)
                        elif msg == 'InstGroup_NotRun':
                            pe_backpressure[pe_key]['by_type']['InstGroupNotRun'].add(cycle)
                        elif msg == 'CheckFlags_Failed':
                            pe_backpressure[pe_key]['by_type']['CheckFlagsFailed'].add(cycle)
                            
                            # Try to extract port information from Reason
                            reason = entry.get('Reason', '')
                            # Reason format might be: "Port South[R] not ready" or "RecvBufHeadReady[0][2]=false"
                            if 'Port' in reason:
                                # Extract direction: "Port South[R] not ready" -> "South"
                                dir_match = re.search(r'Port\s+(\w+)', reason)
                                if dir_match:
                                    direction = dir_match.group(1)
                                    port_key = f"PE({x},{y}):{direction}"
                                    port_backpressure[port_key]['total_cycles'].add(cycle)
                                    port_backpressure[port_key]['by_type']['CheckFlagsFailed'].add(cycle)
                                    port_backpressure[port_key]['by_direction'][direction].add(cycle)
                        
                        reason = entry.get('Reason', 'Unknown')
                        if reason == 'Unknown':
                            if msg == 'InstGroup_Blocked':
                                reason = 'CheckFlags returned false'
                            elif msg == 'InstGroup_NotRun':
                                reason = 'CheckFlags returned false for all operations'
                        pe_backpressure[pe_key]['by_reason'][reason] += 1
            
            except (json.JSONDecodeError, ValueError):
                continue
    
    # Extract total cycles from log end
    total_cycles = int(max_time) + 1
    with open(log_file, 'r') as f:
        lines = f.readlines()
        for line in reversed(lines):
            if 'Total cycles' in line:
                try:
                    total_cycles = int(line.split('Total cycles:')[1].strip())
                    break
                except:
                    pass
    
    return pe_stats, link_stats, pe_backpressure, port_backpressure, total_cycles

def generate_pe_heatmap(pe_stats, total_cycles, output_file='pe_utilization_heatmap.png'):
    """Generate PE utilization heatmap"""
    utilization_matrix = np.zeros((GRID_ROWS, GRID_COLS))
    
    for pe_key, pe in pe_stats.items():
        x, y = parse_pe_coord(pe_key)
        if x is not None and y is not None:
            active_cycles_set = set(pe['exec_events'])
            active_cycles = len(active_cycles_set)
            utilization = min((active_cycles / total_cycles * 100) if total_cycles > 0 else 0, 100.0)
            
            if 0 <= y < GRID_ROWS and 0 <= x < GRID_COLS:
                utilization_matrix[y, x] = utilization
    
    fig, ax = plt.subplots(figsize=(10, 8))
    sns.heatmap(utilization_matrix,
                annot=True,
                fmt='.2f',
                cmap='YlOrRd',
                cbar_kws={'label': 'Utilization (%)'},
                xticklabels=range(GRID_COLS),
                yticklabels=range(GRID_ROWS),
                ax=ax,
                vmin=0,
                vmax=100)
    
    ax.set_xlabel('X (Column)', fontsize=12)
    ax.set_ylabel('Y (Row)', fontsize=12)
    ax.set_title('PE Utilization Heatmap (%)', fontsize=14, fontweight='bold')
    ax.invert_yaxis()
    
    plt.tight_layout()
    plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"PE utilization heatmap saved to {output_file}")
    plt.close()

def generate_link_heatmap(link_stats, total_cycles, output_file='link_utilization_heatmap.png'):
    """Generate Link utilization heatmap (by direction)"""
    directions = ['North', 'East', 'South', 'West']
    fig, axes = plt.subplots(2, 2, figsize=(16, 14))
    axes = axes.flatten()
    
    for dir_idx, direction in enumerate(directions):
        ax = axes[dir_idx]
        utilization_matrix = np.zeros((GRID_ROWS, GRID_COLS))
        
        for link_key, link in link_stats.items():
            pe_coord, link_dir, link_type = parse_link_info(link_key)
            
            if pe_coord is not None and link_dir == direction:
                x, y = pe_coord
                total_ops = link['send_count'] + link['recv_count']
                active_cycles = total_ops
                utilization = (active_cycles / total_cycles * 100) if total_cycles > 0 else 0
                
                if 0 <= y < GRID_ROWS and 0 <= x < GRID_COLS:
                    utilization_matrix[y, x] += utilization
        
        sns.heatmap(utilization_matrix,
                    annot=True,
                    fmt='.2f',
                    cmap='Blues',
                    cbar_kws={'label': 'Utilization (%)'},
                    xticklabels=range(GRID_COLS),
                    yticklabels=range(GRID_ROWS),
                    ax=ax,
                    vmin=0,
                    vmax=100)
        
        ax.set_xlabel('X (Column)', fontsize=10)
        ax.set_ylabel('Y (Row)', fontsize=10)
        ax.set_title(f'{direction} Link Utilization (%)', fontsize=12, fontweight='bold')
        ax.invert_yaxis()
    
    plt.suptitle('Link Utilization Heatmap by Direction (%)', fontsize=16, fontweight='bold', y=0.995)
    plt.tight_layout()
    plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"Link utilization heatmap saved to {output_file}")
    plt.close()

def generate_backpressure_heatmap(pe_backpressure, total_cycles, output_file='backpressure_heatmap.png'):
    """Generate total backpressure heatmap"""
    utilization_matrix = np.zeros((GRID_ROWS, GRID_COLS))
    
    for pe_key, stats in pe_backpressure.items():
        x, y = parse_pe_coord(pe_key)
        if x is not None and y is not None:
            backpressure_cycles = len(stats['total_cycles'])
            backpressure_rate = (backpressure_cycles / total_cycles * 100) if total_cycles > 0 else 0
            
            if 0 <= y < GRID_ROWS and 0 <= x < GRID_COLS:
                utilization_matrix[y, x] = backpressure_rate
    
    fig, ax = plt.subplots(figsize=(10, 8))
    sns.heatmap(utilization_matrix,
                annot=True,
                fmt='.2f',
                cmap='Reds',
                cbar_kws={'label': 'Backpressure Rate (%)'},
                xticklabels=range(GRID_COLS),
                yticklabels=range(GRID_ROWS),
                ax=ax,
                vmin=0)
    
    ax.set_xlabel('X (Column)', fontsize=12)
    ax.set_ylabel('Y (Row)', fontsize=12)
    ax.set_title('Total Backpressure Heatmap (%)', fontsize=14, fontweight='bold')
    ax.invert_yaxis()
    
    plt.tight_layout()
    plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"Total backpressure heatmap saved to {output_file}")
    plt.close()

def generate_port_backpressure_heatmap(port_backpressure, total_cycles, output_file='port_backpressure_heatmap.png'):
    """Generate port-level backpressure heatmap (by direction)"""
    directions = ['North', 'East', 'South', 'West']
    fig, axes = plt.subplots(2, 2, figsize=(16, 14))
    axes = axes.flatten()
    
    for dir_idx, direction in enumerate(directions):
        ax = axes[dir_idx]
        utilization_matrix = np.zeros((GRID_ROWS, GRID_COLS))
        
        for port_key, stats in port_backpressure.items():
            # Parse port key: PE(x,y):Direction:Color or PE(x,y):Direction
            match = re.match(r'PE\((\d+),(\d+)\):(\w+)', port_key)
            if match:
                x, y = int(match.group(1)), int(match.group(2))
                port_dir = match.group(3)
                
                if port_dir == direction:
                    backpressure_cycles = len(stats['total_cycles'])
                    backpressure_rate = (backpressure_cycles / total_cycles * 100) if total_cycles > 0 else 0
                    
                    if 0 <= y < GRID_ROWS and 0 <= x < GRID_COLS:
                        utilization_matrix[y, x] += backpressure_rate
        
        sns.heatmap(utilization_matrix,
                    annot=True,
                    fmt='.2f',
                    cmap='Reds',
                    cbar_kws={'label': 'Backpressure Rate (%)'},
                    xticklabels=range(GRID_COLS),
                    yticklabels=range(GRID_ROWS),
                    ax=ax,
                    vmin=0)
        
        ax.set_xlabel('X (Column)', fontsize=10)
        ax.set_ylabel('Y (Row)', fontsize=10)
        ax.set_title(f'{direction} Port Backpressure (%)', fontsize=12, fontweight='bold')
        ax.invert_yaxis()
    
    plt.suptitle('Port Backpressure Heatmap by Direction (%)', fontsize=16, fontweight='bold', y=0.995)
    plt.tight_layout()
    plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"Port backpressure heatmap saved to {output_file}")
    plt.close()

def generate_backpressure_by_type(pe_backpressure, total_cycles, output_file='backpressure_by_type_heatmap.png'):
    """Generate backpressure heatmap by type"""
    # Backpressure type descriptions for better understanding
    type_descriptions = {
        'InstGroupBlocked': 'Inst Group Blocked\n(One operation failed CheckFlags)',
        'InstGroupNotRun': 'Inst Group Not Run\n(All operations failed CheckFlags)',
        'CheckFlagsFailed': 'CheckFlags Failed\n(Port not ready or data invalid)',
        'SendFailed': 'Send Failed\n(Port send operation failed)',
        'RecvSkipped': 'Recv Skipped\n(Data reception skipped in SyncOp mode)',
        'DataOverwritten': 'Data Overwritten\n(Old data overwritten by new data in AsyncOp)',
        'SendBufBusy': 'Send Buffer Busy\n(Destination port buffer is busy)',
        'MemoryWriteFailed': 'Memory Write Failed\n(Memory write request failed)',
        'MemoryReadFailed': 'Memory Read Failed\n(Memory read request failed)',
    }
    
    all_types = set()
    for stats in pe_backpressure.values():
        all_types.update(stats['by_type'].keys())
    
    main_types = ['InstGroupBlocked', 'InstGroupNotRun', 'CheckFlagsFailed', 
                  'SendFailed', 'RecvSkipped', 'DataOverwritten', 'SendBufBusy', 
                  'MemoryWriteFailed', 'MemoryReadFailed']
    types_to_show = [t for t in main_types if t in all_types]
    if not types_to_show:
        types_to_show = sorted(all_types)
    
    n_types = len(types_to_show)
    n_cols = 2
    n_rows = (n_types + n_cols - 1) // n_cols
    if n_rows == 0:
        n_rows = 1
    
    fig, axes = plt.subplots(n_rows, n_cols, figsize=(16, 4*n_rows))
    if n_types == 1:
        axes = [axes]
    else:
        axes = axes.flatten()
    
    for idx, bp_type in enumerate(types_to_show):
        ax = axes[idx]
        utilization_matrix = np.zeros((GRID_ROWS, GRID_COLS))
        
        for pe_key, stats in pe_backpressure.items():
            x, y = parse_pe_coord(pe_key)
            if x is not None and y is not None:
                cycles = len(stats['by_type'].get(bp_type, set()))
                rate = (cycles / total_cycles * 100) if total_cycles > 0 else 0
                
                if 0 <= y < GRID_ROWS and 0 <= x < GRID_COLS:
                    utilization_matrix[y, x] = rate
        
        sns.heatmap(utilization_matrix,
                    annot=True,
                    fmt='.2f',
                    cmap='Reds',
                    cbar_kws={'label': 'Rate (%)'},
                    xticklabels=range(GRID_COLS),
                    yticklabels=range(GRID_ROWS),
                    ax=ax,
                    vmin=0)
        
        ax.set_xlabel('X (Column)', fontsize=10)
        ax.set_ylabel('Y (Row)', fontsize=10)
        # Use friendly description if available, otherwise use type name
        title = type_descriptions.get(bp_type, bp_type)
        ax.set_title(f'{title} (%)', fontsize=11, fontweight='bold')
        ax.invert_yaxis()
    
    for idx in range(n_types, len(axes)):
        axes[idx].set_visible(False)
    
    plt.suptitle('Backpressure Heatmap by Type (%)', fontsize=16, fontweight='bold', y=0.995)
    plt.tight_layout()
    plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"Backpressure by type heatmap saved to {output_file}")
    plt.close()

def parse_trace_data(log_file):
    """Parse log file to extract trace information for timeline visualization"""
    # Track instructions by PE and time
    pe_instructions = defaultdict(lambda: defaultdict(list))  # {(x,y): {time: [instructions]}}
    data_flows = []  # List of data flow events
    
    with open(log_file, 'r') as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith('=') or line.startswith('GRID_ROWS:') or line.startswith('GRID_COLS:'):
                continue
            
            try:
                entry = json.loads(line)
                msg = entry.get('msg', '')
                time = float(entry.get('Time', 0))
                x = entry.get('X', -1)
                y = entry.get('Y', -1)
                
                if x >= 0 and y >= 0:
                    pe_key = (x, y)
                    time_int = int(time)
                    
                    # Track instruction execution
                    if msg in ['Inst_Exec', 'PHI_Exec', 'GPRED_Exec', 'NOT_Exec']:
                        opcode = entry.get('OpCode', msg)
                        pe_instructions[pe_key][time_int].append({
                            'opcode': opcode,
                            'time': time,
                            'type': 'exec',
                            'label': opcode
                        })
                    elif msg == 'Memory':
                        behavior = entry.get('Behavior', '')
                        if behavior in ['WriteMemory', 'ReadMemory']:
                            pe_instructions[pe_key][time_int].append({
                                'opcode': behavior,
                                'time': time,
                                'type': 'memory',
                                'label': behavior
                            })
                    elif msg == 'DataFlow':
                        behavior = entry.get('Behavior', '')
                        direction = entry.get('Direction', '')
                        data = entry.get('Data', '')
                        
                        if behavior == 'Send':
                            data_flows.append({
                                'time': time_int,
                                'from': pe_key,
                                'direction': direction,
                                'data': data,
                                'type': 'send'
                            })
                            # Also track as instruction
                            pe_instructions[pe_key][time_int].append({
                                'opcode': f"Send({direction})",
                                'time': time,
                                'type': 'send',
                                'label': f"Send({direction})"
                            })
                        elif behavior == 'Recv':
                            data_flows.append({
                                'time': time_int,
                                'to': pe_key,
                                'direction': direction,
                                'data': data,
                                'type': 'recv'
                            })
                            # Also track as instruction
                            pe_instructions[pe_key][time_int].append({
                                'opcode': f"Recv({direction})",
                                'time': time,
                                'type': 'recv',
                                'label': f"Recv({direction})"
                            })
                    elif msg == 'InstGroup_Blocked':
                        opcode = entry.get('OpCode', 'Blocked')
                        pe_instructions[pe_key][time_int].append({
                            'opcode': f"Blocked({opcode})",
                            'time': time,
                            'type': 'blocked',
                            'label': f"Blocked({opcode})"
                        })
                    elif msg == 'InstGroup_NotRun':
                        pe_instructions[pe_key][time_int].append({
                            'opcode': 'Idle',
                            'time': time,
                            'type': 'idle',
                            'label': 'Idle'
                        })
                
            except (json.JSONDecodeError, ValueError):
                continue
    
    return pe_instructions, data_flows

def generate_timeline(pe_instructions, data_flows, total_cycles, output_file='timeline_visualization.png'):
    """Generate linear timeline visualization showing what each PE does at each timestep"""
    # Get all PEs that have activity
    all_pe_keys = sorted(set(pe_instructions.keys()))
    
    if not all_pe_keys:
        print("No PE activity found for timeline")
        return
    
    # Get time range
    all_times = set()
    for pe_inst in pe_instructions.values():
        all_times.update(pe_inst.keys())
    if data_flows:
        all_times.update(flow['time'] for flow in data_flows)
    
    if not all_times:
        print("No trace data found for timeline")
        return
    
    min_time = min(all_times)
    max_time = max(all_times)
    
    # Create mapping from PE to row index
    n_pes = len(all_pe_keys)
    time_range = max_time - min_time + 1
    
    # Create figure
    fig, ax = plt.subplots(figsize=(max(16, time_range * 0.8), max(8, n_pes * 0.6)))
    
    # Color map for different instruction types
    color_map = {
        'exec': '#4CAF50',      # Green for execution
        'memory': '#2196F3',    # Blue for memory
        'send': '#FF9800',      # Orange for send
        'recv': '#9C27B0',      # Purple for recv
        'blocked': '#F44336',   # Red for blocked
        'idle': '#9E9E9E',      # Gray for idle
        'default': '#607D8B'    # Default blue-gray
    }
    
    # Track what happens at each timestep for each PE
    timeline_data = defaultdict(lambda: defaultdict(list))  # {pe: {time: [events]}}
    
    # Process instruction executions
    for pe_key, time_insts in pe_instructions.items():
        for time, insts in time_insts.items():
            for inst in insts:
                timeline_data[pe_key][time].append(inst)
    
    # Process data flows
    for flow in data_flows:
        time = flow['time']
        if flow['type'] == 'send' and 'from' in flow:
            pe_key = flow['from']
            timeline_data[pe_key][time].append({
                'type': 'send',
                'opcode': 'Send',
                'label': f"Send({flow.get('direction', '')})"
            })
        elif flow['type'] == 'recv' and 'to' in flow:
            pe_key = flow['to']
            timeline_data[pe_key][time].append({
                'type': 'recv',
                'opcode': 'Recv',
                'label': f"Recv({flow.get('direction', '')})"
            })
    
    # Draw timeline bars for each PE
    bar_height = 0.7
    y_positions = {}
    
    for pe_idx, pe_key in enumerate(all_pe_keys):
        y_pos = n_pes - pe_idx - 1
        y_positions[pe_key] = y_pos
        
        # Draw background for this PE row
        ax.axhspan(y_pos - bar_height/2, y_pos + bar_height/2, 
                  xmin=min_time-0.5, xmax=max_time+0.5,
                  facecolor='lightgray', alpha=0.2)
        
        # Draw timeline bars for each timestep
        bar_width = 0.8
        for time in range(min_time, max_time + 1):
            x_start = time - 0.4
            if time in timeline_data[pe_key]:
                events = timeline_data[pe_key][time]
                
                if len(events) == 1:
                    # Single event - draw one bar
                    event = events[0]
                    color = color_map.get(event.get('type', 'default'), color_map['default'])
                    ax.barh(y_pos, bar_width, height=bar_height, left=x_start,
                           color=color, edgecolor='black', linewidth=0.5, alpha=0.8)
                    
                    # Add label
                    label = event.get('label', event.get('opcode', 'Unknown'))
                    if len(label) <= 8:
                        ax.text(time, y_pos, label, ha='center', va='center',
                               fontsize=7, fontweight='bold', color='white')
                    else:
                        ax.text(time, y_pos, label[:6]+'..', ha='center', va='center',
                               fontsize=6, fontweight='bold', color='white')
                else:
                    # Multiple events - draw side-by-side
                    n_events = len(events)
                    sub_width = bar_width / n_events
                    for idx, event in enumerate(events):
                        sub_x_start = x_start + idx * sub_width
                        color = color_map.get(event.get('type', 'default'), color_map['default'])
                        ax.barh(y_pos, sub_width, height=bar_height, left=sub_x_start,
                               color=color, edgecolor='black', linewidth=0.5, alpha=0.8)
                        
                        # Add short label
                        opcode = event.get('opcode', 'Unknown')
                        label = opcode[:4] if len(opcode) > 4 else opcode
                        ax.text(sub_x_start + sub_width/2, y_pos, label, ha='center', va='center',
                               fontsize=5, fontweight='bold', color='white', rotation=0)
            else:
                # No activity - draw empty bar
                ax.barh(y_pos, bar_width, height=bar_height, left=x_start,
                       color='white', edgecolor='lightgray', linewidth=0.5, alpha=0.3)
    
    # Set labels and title
    ax.set_xlabel('Time Step', fontsize=12, fontweight='bold')
    ax.set_ylabel('PE (Processing Element)', fontsize=12, fontweight='bold')
    ax.set_title(f'Kernel Execution Timeline (Time {min_time} - {max_time})', 
                fontsize=14, fontweight='bold', pad=20)
    
    # Set x-axis
    ax.set_xlim(min_time - 0.5, max_time + 0.5)
    ax.set_xticks(range(min_time, max_time + 1))
    ax.set_xticklabels(range(min_time, max_time + 1), fontsize=9)
    
    # Set y-axis with PE labels
    ax.set_ylim(-0.5, n_pes - 0.5)
    pe_labels = [f'PE({x},{y})' for (x, y) in all_pe_keys]
    ax.set_yticks(range(n_pes))
    ax.set_yticklabels(pe_labels, fontsize=9)
    
    # Add grid
    ax.grid(True, axis='x', alpha=0.3, linestyle='--', linewidth=0.5)
    ax.grid(True, axis='y', alpha=0.2, linestyle='-', linewidth=0.5)
    
    # Add legend
    legend_elements = [
        plt.Rectangle((0,0),1,1, facecolor=color_map['exec'], edgecolor='black', label='Execute'),
        plt.Rectangle((0,0),1,1, facecolor=color_map['memory'], edgecolor='black', label='Memory'),
        plt.Rectangle((0,0),1,1, facecolor=color_map['send'], edgecolor='black', label='Send'),
        plt.Rectangle((0,0),1,1, facecolor=color_map['recv'], edgecolor='black', label='Recv'),
        plt.Rectangle((0,0),1,1, facecolor=color_map['blocked'], edgecolor='black', label='Blocked'),
        plt.Rectangle((0,0),1,1, facecolor=color_map['idle'], edgecolor='black', label='Idle'),
    ]
    ax.legend(handles=legend_elements, loc='upper right', fontsize=9, framealpha=0.9)
    
    plt.tight_layout()
    plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"Timeline visualization saved to {output_file}")
    plt.close()

def print_summary(pe_stats, link_stats, pe_backpressure, port_backpressure, total_cycles):
    """Print summary report"""
    print(f"\nTotal Cycles: {total_cycles}")
    
    # PE utilization summary
    print("\n" + "=" * 70)
    print("PE Utilization Summary")
    print("=" * 70)
    
    pe_utilization_list = []
    for pe_key in sorted(pe_stats.keys()):
        pe = pe_stats[pe_key]
        active_cycles_set = set(pe['exec_events'])
        active_cycles = len(active_cycles_set)
        utilization = min((active_cycles / total_cycles * 100) if total_cycles > 0 else 0, 100.0)
        
        pe_utilization_list.append({
            'pe': pe_key,
            'utilization': utilization,
            'active_cycles': active_cycles,
            'exec_count': len(pe['exec_events']),
            'blocked_count': len(pe['blocked_events']),
            'idle_count': len(pe['idle_events'])
        })
    
    avg_pe_utilization = sum(p['utilization'] for p in pe_utilization_list) / len(pe_utilization_list) if pe_utilization_list else 0
    print(f"Average PE Utilization: {avg_pe_utilization:.2f}%")
    print(f"Total PEs: {len(pe_utilization_list)}")
    
    # Link utilization summary
    print("\n" + "=" * 70)
    print("Link Utilization Summary")
    print("=" * 70)
    
    link_utilization_list = []
    for link_key in sorted(link_stats.keys()):
        link = link_stats[link_key]
        total_ops = link['send_count'] + link['recv_count']
        if total_ops > 0:
            active_cycles = total_ops
            utilization = (active_cycles / total_cycles * 100) if total_cycles > 0 else 0
            link_utilization_list.append({
                'link': link_key,
                'utilization': utilization,
                'active_cycles': active_cycles,
                'send_count': link['send_count'],
                'recv_count': link['recv_count']
            })
    
    avg_link_utilization = sum(l['utilization'] for l in link_utilization_list) / len(link_utilization_list) if link_utilization_list else 0
    print(f"Average Link Utilization: {avg_link_utilization:.2f}%")
    print(f"Total Links: {len(link_utilization_list)}")
    
    # Backpressure summary
    print("\n" + "=" * 70)
    print("Backpressure Summary")
    print("=" * 70)
    
    total_bp_cycles = 0
    for pe_key in sorted(pe_backpressure.keys()):
        stats = pe_backpressure[pe_key]
        total_bp_cycles += len(stats['total_cycles'])
    
    avg_bp_rate = (total_bp_cycles / total_cycles * 100) if total_cycles > 0 else 0
    print(f"Total Backpressure Cycles: {total_bp_cycles}")
    print(f"Average Backpressure Rate: {avg_bp_rate:.2f}%")
    print(f"PEs with Backpressure: {len([k for k, v in pe_backpressure.items() if len(v['total_cycles']) > 0])}")
    
    # Port-level backpressure summary
    if port_backpressure:
        print("\nPort-level Backpressure:")
        for port_key in sorted(port_backpressure.keys()):
            stats = port_backpressure[port_key]
            if len(stats['total_cycles']) > 0:
                bp_cycles = len(stats['total_cycles'])
                bp_rate = (bp_cycles / total_cycles * 100) if total_cycles > 0 else 0
                print(f"  {port_key}: {bp_cycles} cycles ({bp_rate:.2f}%)")

def main():
    log_file = sys.argv[1] if len(sys.argv) > 1 else 'histogram_run.log'
    generate_heatmaps = '--no-heatmap' not in sys.argv
    generate_timeline_viz = '--timeline' in sys.argv or '--all' in sys.argv
    
    # Analyze log
    pe_stats, link_stats, pe_backpressure, port_backpressure, total_cycles = analyze_all(log_file)
    
    # Print summary
    print_summary(pe_stats, link_stats, pe_backpressure, port_backpressure, total_cycles)
    
    if generate_heatmaps:
        print("\n" + "=" * 70)
        print("Generating Heatmaps")
        print("=" * 70)
        
        # Generate PE utilization heatmap
        print("\nGenerating PE utilization heatmap...")
        generate_pe_heatmap(pe_stats, total_cycles)
        
        # Generate Link utilization heatmap
        print("Generating link utilization heatmap...")
        generate_link_heatmap(link_stats, total_cycles)
        
        # Generate total backpressure heatmap
        print("Generating total backpressure heatmap...")
        generate_backpressure_heatmap(pe_backpressure, total_cycles)
        
        # Generate port backpressure heatmap
        active_ports = [k for k, v in port_backpressure.items() if len(v['total_cycles']) > 0]
        if active_ports:
            print(f"Generating port backpressure heatmap ({len(active_ports)} active ports)...")
            generate_port_backpressure_heatmap(port_backpressure, total_cycles)
        else:
            print("No port-level backpressure detected, skipping port backpressure heatmap")
        
        # Generate backpressure by type heatmap
        print("Generating backpressure by type heatmap...")
        generate_backpressure_by_type(pe_backpressure, total_cycles)
        
        print("\n" + "=" * 70)
        print("All heatmaps generated successfully!")
        print("=" * 70)
    
    if generate_timeline_viz:
        print("\n" + "=" * 70)
        print("Generating Timeline Visualization")
        print("=" * 70)
        
        # Parse trace data
        print("Parsing trace data from log file...")
        pe_instructions, data_flows = parse_trace_data(log_file)
        
        # Generate timeline visualization
        print("Generating timeline visualization...")
        generate_timeline(pe_instructions, data_flows, total_cycles, 'timeline_visualization.png')
        
        print("\n" + "=" * 70)
        print("Timeline visualization generated successfully!")
        print("=" * 70)

if __name__ == '__main__':
    main()

