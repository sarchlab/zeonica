#!/usr/bin/env python3
"""
CGRA Debug Report Generator
Creates readable text reports from CSV data
"""

import csv
import sys
from collections import defaultdict

def print_core_timeline(csv_file, cores_to_show=None, cycles_range=None):
    """Print detailed timeline for selected cores"""
    
    if cores_to_show is None:
        cores_to_show = [(0,2), (1,2), (2,1), (2,2), (3,1), (3,2), (3,0)]
    
    with open(csv_file, 'r') as f:
        reader = csv.DictReader(f)
        rows = list(reader)
    
    if cycles_range is None:
        cycles_range = (0, len(rows) - 1)
    
    start_cycle, end_cycle = cycles_range
    
    for x, y in cores_to_show:
        print(f"\n{'='*140}")
        print(f"PE({x},{y}) Timeline")
        print(f"{'='*140}")
        print(f"{'Cycle':<8} {'Status':<10} {'Opcode':<15} {'→Dir':<6} {'→Data':<8} {'←Dir':<6} {'←Data':<8} {'BackP':<8} {'Notes':<40}")
        print(f"{'-'*140}")
        
        for row in rows[start_cycle:end_cycle+1]:
            cycle = row['Cycle']
            
            status = row.get(f'PE({x},{y})-Status', '-')
            opcode = row.get(f'PE({x},{y})-Opcode', '-')
            send_dir = row.get(f'PE({x},{y})-SendDir', '-')
            send_data = row.get(f'PE({x},{y})-SendData', '-')
            recv_dir = row.get(f'PE({x},{y})-RecvDir', '-')
            recv_data = row.get(f'PE({x},{y})-RecvData', '-')
            backp = row.get(f'PE({x},{y})-BackP', '-')
            
            notes = ""
            if status == 'BLOCKED':
                notes = "⚠️  STALLED"
            elif status == 'EXEC':
                notes = f"Executing {opcode}"
            
            print(f"{cycle:<8} {status:<10} {opcode:<15} {send_dir:<6} {send_data:<8} {recv_dir:<6} {recv_data:<8} {backp:<8} {notes:<40}")

def print_dataflow_sequence(csv_file, cycles_range=None):
    """Print data flow sequence"""
    
    with open(csv_file, 'r') as f:
        reader = csv.DictReader(f)
        rows = list(reader)
    
    if cycles_range is None:
        cycles_range = (0, min(20, len(rows) - 1))
    
    start_cycle, end_cycle = cycles_range
    
    print(f"\n{'='*140}")
    print(f"DATA FLOW SEQUENCE (Cycles {start_cycle} to {end_cycle})")
    print(f"{'='*140}\n")
    
    for row in rows[start_cycle:end_cycle+1]:
        cycle = row['Cycle']
        
        # Find all active data flows for this cycle
        flows = []
        for col in row:
            if col.startswith('PE(') and col.endswith(')-SendData'):
                core = col.split('-')[0]
                send_data = row[col]
                send_dir = row[col.replace('-SendData', '-SendDir')]
                
                if send_data != '-' and send_dir != '-':
                    flows.append(f"{core} →{send_dir} [{send_data}]")
        
        if flows:
            print(f"Cycle {cycle:>3}: {' | '.join(flows)}")

def print_backpressure_summary(csv_file):
    """Print backpressure analysis"""
    
    with open(csv_file, 'r') as f:
        reader = csv.DictReader(f)
        rows = list(reader)
    
    print(f"\n{'='*140}")
    print(f"BACKPRESSURE ANALYSIS")
    print(f"{'='*140}\n")
    
    # Count backpressure by core
    backpressure_by_core = defaultdict(int)
    
    for row in rows:
        for col in row:
            if col.endswith('-BackP') and row[col] == 'YES':
                core = col.split('-')[0]
                backpressure_by_core[core] += 1
    
    for core in sorted(backpressure_by_core.keys()):
        count = backpressure_by_core[core]
        print(f"{core}: {count} backpressure cycles ({100*count/len(rows):.1f}%)")
    
    # Timeline of backpressure events
    print(f"\n{'-'*140}")
    print("Backpressure Timeline:")
    print(f"{'-'*140}")
    
    for row in rows[:25]:  # First 25 cycles
        cycle = row['Cycle']
        blocked = []
        for col in row:
            if col.endswith('-BackP') and row[col] == 'YES':
                core = col.split('-')[0]
                blocked.append(core)
        
        if blocked:
            print(f"Cycle {cycle:>3}: {', '.join(blocked)} BLOCKED")

if __name__ == '__main__':
    if len(sys.argv) < 2:
        print("Usage: python3 generate_debug_report.py <core_activity.csv> [dataflow_trace.csv]")
        sys.exit(1)
    
    activity_csv = sys.argv[1]
    
    # Show core timelines
    print_core_timeline(activity_csv, cycles_range=(0, 25))
    
    # Show data flow
    print_dataflow_sequence(activity_csv, cycles_range=(0, 20))
    
    # Show backpressure
    print_backpressure_summary(activity_csv)
