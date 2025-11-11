#!/usr/bin/env python3
"""
CGRA State CSV Export Tool
Exports cycle-by-cycle core states to CSV for analysis in Excel/Spreadsheet
"""

import json
import sys
import csv
from collections import defaultdict
from typing import Dict, List, Any, Tuple

class CGRAStateExporter:
    def __init__(self, log_file: str):
        self.log_file = log_file
        self.events_by_cycle = defaultdict(lambda: defaultdict(list))
        self.max_cycle = 0
        
    def parse_log(self):
        """Parse JSON log file"""
        print(f"[*] Parsing log: {self.log_file}")
        with open(self.log_file, 'r') as f:
            for line in f:
                if not line.strip():
                    continue
                try:
                    entry = json.loads(line)
                    time = entry.get('Time')
                    if time is not None:
                        self.max_cycle = max(self.max_cycle, time)
                        self.events_by_cycle[time][entry.get('msg', 'Unknown')].append(entry)
                except json.JSONDecodeError:
                    pass
    
    def export_core_activity_csv(self, output_file: str = "core_activity.csv"):
        """Export core activity as CSV"""
        print(f"[*] Exporting core activity to: {output_file}")
        
        # Collect all unique cores
        cores = set()
        for cycle_events in self.events_by_cycle.values():
            for msg, events in cycle_events.items():
                for event in events:
                    x, y = event.get('X'), event.get('Y')
                    if x is not None and y is not None:
                        cores.add((x, y))
        
        cores = sorted(cores)
        
        with open(output_file, 'w', newline='') as f:
            writer = csv.writer(f)
            
            # Header: Cycle | PE(0,0)-Status | PE(0,0)-Inst | PE(0,0)-BackP | PE(1,0)-Status | ...
            header = ['Cycle']
            for x, y in cores:
                header.extend([
                    f'PE({x},{y})-Status',
                    f'PE({x},{y})-Opcode',
                    f'PE({x},{y})-SendDir',
                    f'PE({x},{y})-SendData',
                    f'PE({x},{y})-RecvDir',
                    f'PE({x},{y})-RecvData',
                    f'PE({x},{y})-BackP'
                ])
            writer.writerow(header)
            
            # Data rows
            for cycle in range(self.max_cycle + 1):
                row = [cycle]
                
                for x, y in cores:
                    cycle_events = self.events_by_cycle[cycle]
                    
                    # Find status
                    status = 'IDLE'
                    opcode = '-'
                    send_dir = '-'
                    send_data = '-'
                    recv_dir = '-'
                    recv_data = '-'
                    backp = '-'
                    
                    # Process InstExec
                    for event in cycle_events.get('InstExec', []):
                        if event.get('X') == x and event.get('Y') == y:
                            status = 'EXEC'
                            opcode = event.get('OpCode', '?')
                    
                    # Process DataFlow Sends
                    for event in cycle_events.get('DataFlow', []):
                        if event.get('X') == x and event.get('Y') == y and event.get('Behavior') == 'Send':
                            send_dir = event.get('Direction', '?')
                            send_data = event.get('Data', '?')
                    
                    # Process DataFlow Receives
                    for event in cycle_events.get('DataFlow', []):
                        if event.get('X') == x and event.get('Y') == y and event.get('Behavior') == 'Receive':
                            recv_dir = event.get('Direction', '?')
                            recv_data = event.get('Data', '?')
                    
                    # Check for backpressure
                    if cycle_events.get('Backpressure') or cycle_events.get('InstGroup_Blocked'):
                        for event in cycle_events.get('Backpressure', []) + cycle_events.get('InstGroup_Blocked', []):
                            if event.get('X') == x and event.get('Y') == y:
                                backp = 'YES'
                                status = 'BLOCKED'
                    
                    row.extend([status, opcode, send_dir, send_data, recv_dir, recv_data, backp])
                
                writer.writerow(row)
        
        print(f"[+] Exported {self.max_cycle + 1} cycles")
    
    def export_dataflow_csv(self, output_file: str = "dataflow_trace.csv"):
        """Export all data flows as CSV"""
        print(f"[*] Exporting data flows to: {output_file}")
        
        with open(output_file, 'w', newline='') as f:
            writer = csv.writer(f)
            writer.writerow(['Cycle', 'From', 'To', 'Direction', 'Data', 'Predicate', 'Behavior'])
            
            for cycle in sorted(self.events_by_cycle.keys()):
                for event in self.events_by_cycle[cycle].get('DataFlow', []):
                    x, y = event.get('X'), event.get('Y')
                    direction = event.get('Direction', '?')
                    data = event.get('Data', '?')
                    pred = event.get('Pred', '?')
                    behavior = event.get('Behavior', '?')
                    
                    from_to = f"PE({x},{y})"
                    to_pe = {
                        'North': f"PE({x},{y-1})",
                        'South': f"PE({x},{y+1})",
                        'East': f"PE({x+1},{y})",
                        'West': f"PE({x-1},{y})"
                    }
                    
                    if behavior == 'Send':
                        to_pe_str = to_pe.get(direction, '?')
                    else:
                        # Reverse the direction for receive
                        rev_dir = {'North': 'South', 'South': 'North', 'East': 'West', 'West': 'East'}
                        from_to = to_pe.get(rev_dir.get(direction, '?'), '?')
                        to_pe_str = f"PE({x},{y})"
                    
                    writer.writerow([cycle, from_to, to_pe_str, direction, data, pred, behavior])
    
    def export_backpressure_csv(self, output_file: str = "backpressure_analysis.csv"):
        """Export backpressure events as CSV"""
        print(f"[*] Exporting backpressure to: {output_file}")
        
        with open(output_file, 'w', newline='') as f:
            writer = csv.writer(f)
            writer.writerow(['Cycle', 'Core', 'OpCode', 'Direction', 'Reason', 'EventType'])
            
            for cycle in sorted(self.events_by_cycle.keys()):
                for event in self.events_by_cycle[cycle].get('Backpressure', []):
                    x, y = event.get('X'), event.get('Y')
                    core = f"PE({x},{y})"
                    opcode = event.get('OpCode', '?')
                    direction = event.get('Direction', '?')
                    reason = event.get('Reason', '?')
                    event_type = 'Backpressure'
                    
                    writer.writerow([cycle, core, opcode, direction, reason, event_type])
                
                for event in self.events_by_cycle[cycle].get('InstGroup_Blocked', []):
                    x, y = event.get('X'), event.get('Y')
                    core = f"PE({x},{y})"
                    opcode = event.get('OpCode', '?')
                    direction = event.get('Direction', '?')
                    reason = event.get('Reason', '?')
                    event_type = 'InstGroup_Blocked'
                    
                    writer.writerow([cycle, core, opcode, direction, reason, event_type])


if __name__ == '__main__':
    if len(sys.argv) < 2:
        print("Usage: python3 export_cgra_csv.py <log_file> [output_dir]")
        sys.exit(1)
    
    log_file = sys.argv[1]
    output_dir = sys.argv[2] if len(sys.argv) > 2 else "."
    
    exporter = CGRAStateExporter(log_file)
    exporter.parse_log()
    
    exporter.export_core_activity_csv(f"{output_dir}/core_activity.csv")
    exporter.export_dataflow_csv(f"{output_dir}/dataflow_trace.csv")
    exporter.export_backpressure_csv(f"{output_dir}/backpressure_analysis.csv")
    
    print("[+] All CSV files generated successfully")
