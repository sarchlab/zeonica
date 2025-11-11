#!/usr/bin/env python3
"""
Detailed Backpressure Diagnosis Tool
Shows exact reasons for backpressure and data availability
"""

import json
import sys
from collections import defaultdict
from typing import Dict, List, Any

class BackpressureDiagnosis:
    def __init__(self, log_file: str):
        self.log_file = log_file
        self.events = []
        
    def parse_log(self):
        """Parse log file"""
        with open(self.log_file, 'r') as f:
            for line in f:
                if line.strip():
                    try:
                        self.events.append(json.loads(line))
                    except:
                        pass
    
    def find_blocking_reasons(self, x: int, y: int, max_cycle: int = 30):
        """Find all blocking reasons for a specific core"""
        print(f"\n{'='*150}")
        print(f"DETAILED BLOCKING ANALYSIS: PE({x},{y})")
        print(f"{'='*150}\n")
        
        core_name = f"Tile[{y}][{x}].Core"
        blocking_events = []
        
        for event in self.events:
            msg = event.get('msg', '')
            time = event.get('Time')
            
            if time is not None and time > max_cycle:
                break
            
            if msg == 'Backpressure' and event.get('X') == x and event.get('Y') == y:
                blocking_events.append(event)
        
        # Group by timestep
        by_time = defaultdict(list)
        for event in blocking_events:
            time = event.get('Time')
            by_time[time].append(event)
        
        print(f"Total blocking events: {len(blocking_events)}\n")
        
        for time in sorted(by_time.keys())[:15]:  # Show first 15 cycles with blocking
            events_at_time = by_time[time]
            print(f"Cycle {time}:")
            
            for event in events_at_time:
                event_type = event.get('Type', '?')
                reason = event.get('Reason', '?')
                opcode = event.get('OpCode', '?')
                direction = event.get('Direction', '?')
                
                if event_type == 'CheckFlagsFailed':
                    recv_buf_ready = event.get('RecvBufHeadReady', False)
                    color_idx = event.get('ColorIdx', '?')
                    dir_idx = event.get('DirIdx', '?')
                    print(f"  [{event_type}] {opcode} to {direction}: RecvBufHeadReady={recv_buf_ready} (ColorIdx={color_idx}, DirIdx={dir_idx})")
                
                elif event_type == 'RecvSkipped':
                    old_data = event.get('OldData', '?')
                    new_data = event.get('NewData', '?')
                    print(f"  [{event_type}] {direction}: Old data {old_data} not consumed, new data {new_data} waiting")
                
                else:
                    print(f"  [{event_type}] {reason}")
    
    def show_dataflow_vs_blockage(self, x: int, y: int, max_cycle: int = 20):
        """Show data flow and corresponding blockage"""
        print(f"\n{'='*150}")
        print(f"DATA FLOW vs BLOCKAGE: PE({x},{y})")
        print(f"{'='*150}\n")
        print(f"{'Cycle':<8} {'Sends':<40} {'Receives':<40} {'Status':<15}")
        print(f"{'-'*150}\n")
        
        # Collect data flows
        flows_by_cycle = defaultdict(lambda: {'sends': [], 'receives': []})
        blocks_by_cycle = defaultdict(bool)
        
        for event in self.events:
            time = event.get('Time')
            if time is None or time > max_cycle:
                continue
            
            if event.get('X') == x and event.get('Y') == y:
                if event.get('msg') == 'DataFlow':
                    direction = event.get('Direction', '?')
                    data = event.get('Data', '?')
                    behavior = event.get('Behavior', '?')
                    
                    if behavior == 'Send':
                        flows_by_cycle[time]['sends'].append(f"{direction}:{data}")
                    else:
                        flows_by_cycle[time]['receives'].append(f"{direction}:{data}")
                
                elif event.get('msg') == 'Backpressure':
                    blocks_by_cycle[time] = True
        
        for cycle in sorted(set(flows_by_cycle.keys()) | set(blocks_by_cycle.keys())):
            sends_str = ', '.join(flows_by_cycle[cycle]['sends']) if flows_by_cycle[cycle]['sends'] else '-'
            recvs_str = ', '.join(flows_by_cycle[cycle]['receives']) if flows_by_cycle[cycle]['receives'] else '-'
            status = '🔴 BLOCKED' if blocks_by_cycle[cycle] else '✓ OK'
            
            print(f"{cycle:<8} {sends_str:<40} {recvs_str:<40} {status:<15}")
    
    def trace_single_instruction(self, x: int, y: int, cycle: int, opcode: str = None):
        """Trace execution of a single instruction across network"""
        print(f"\n{'='*150}")
        print(f"INSTRUCTION TRACE: PE({x},{y}) at Cycle {cycle}")
        print(f"{'='*150}\n")
        
        # Find the instruction
        for event in self.events:
            if (event.get('Time') == cycle and 
                event.get('X') == x and 
                event.get('Y') == y and 
                event.get('msg') in ['InstExec', 'Backpressure']):
                
                print(f"Event Type: {event.get('msg')}")
                print(f"OpCode: {event.get('OpCode', '?')}")
                print(f"Direction: {event.get('Direction', '?')}")
                print(f"Color: {event.get('Color', '?')}")
                
                if event.get('msg') == 'Backpressure':
                    print(f"Reason: {event.get('Reason', '?')}")
                    print(f"RecvBufHeadReady: {event.get('RecvBufHeadReady', '?')}")

if __name__ == '__main__':
    if len(sys.argv) < 2:
        print("Usage: python3 diagnose_backpressure.py <log_file>")
        sys.exit(1)
    
    log_file = sys.argv[1]
    diagnosis = BackpressureDiagnosis(log_file)
    diagnosis.parse_log()
    
    # Analyze key cores
    print("[*] Analyzing blocking patterns for key cores...\n")
    
    for x, y in [(2, 2), (1, 2), (3, 2), (3, 1)]:
        diagnosis.find_blocking_reasons(x, y, max_cycle=15)
        diagnosis.show_dataflow_vs_blockage(x, y, max_cycle=15)
