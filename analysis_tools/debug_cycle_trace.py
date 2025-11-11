#!/usr/bin/env python3
"""
Cycle-by-Cycle CGRA Kernel Debugger
Provides detailed state tracking for each core at each cycle
"""

import json
import sys
from collections import defaultdict, OrderedDict
from typing import Dict, List, Any, Tuple
import re

class CGRADebugger:
    def __init__(self, log_file: str, grid_rows: int = 4, grid_cols: int = 4):
        self.log_file = log_file
        self.grid_rows = grid_rows
        self.grid_cols = grid_cols
        
        # State tracking: {(x, y): {cycle: state_dict}}
        self.core_states = defaultdict(lambda: defaultdict(dict))
        
        # Data flow: {(from_x, from_y, from_dir): [(cycle, data, pred), ...]}
        self.data_flows = defaultdict(list)
        
        # Instruction tracking: {(x, y): {cycle: [instruction_names]}}
        self.instructions_executed = defaultdict(lambda: defaultdict(list))
        
        # Backpressure events
        self.backpressure_events = defaultdict(list)
        
        # Register values: {(x, y): {cycle: {register: value}}}
        self.register_values = defaultdict(lambda: defaultdict(dict))
        
        self.max_cycle = 0
        
    def parse_log(self):
        """Parse the JSON log file"""
        print(f"[*] Parsing log file: {self.log_file}")
        try:
            with open(self.log_file, 'r') as f:
                for line_num, line in enumerate(f, 1):
                    if not line.strip():
                        continue
                    try:
                        entry = json.loads(line)
                        self._process_entry(entry)
                    except json.JSONDecodeError:
                        pass  # Skip malformed lines
                    except Exception as e:
                        pass  # Skip processing errors
        except FileNotFoundError:
            print(f"[!] Log file not found: {self.log_file}")
            sys.exit(1)
        
        print(f"[+] Log parsed. Max cycle: {self.max_cycle}")
        
    def _process_entry(self, entry: Dict[str, Any]):
        """Process a single log entry"""
        msg = entry.get('msg', '')
        time = entry.get('Time')
        
        if time is not None:
            self.max_cycle = max(self.max_cycle, time)
        
        if msg == 'DataFlow':
            self._process_dataflow(entry)
        elif msg in ['Backpressure', 'InstGroup_Blocked', 'InstGroup_NotRun']:
            self._process_backpressure(entry)
        elif msg == 'InstExec':
            self._process_instruction(entry)
    
    def _process_dataflow(self, entry: Dict[str, Any]):
        """Process DataFlow entries"""
        try:
            x, y = entry.get('X'), entry.get('Y')
            direction = entry.get('Direction', '')
            time = entry.get('Time')
            data = entry.get('Data')
            pred = entry.get('Pred')
            behavior = entry.get('Behavior', '')
            
            if x is not None and y is not None:
                key = (x, y, direction, behavior)
                self.data_flows[key].append((time, data, pred))
        except Exception as e:
            pass
    
    def _process_backpressure(self, entry: Dict[str, Any]):
        """Process backpressure events"""
        try:
            x, y = entry.get('X'), entry.get('Y')
            time = entry.get('Time')
            reason = entry.get('Reason', '')
            opcode = entry.get('OpCode', '')
            
            if x is not None and y is not None:
                self.backpressure_events[(x, y)].append({
                    'cycle': time,
                    'opcode': opcode,
                    'reason': reason
                })
        except Exception as e:
            pass
    
    def _process_instruction(self, entry: Dict[str, Any]):
        """Process instruction execution"""
        try:
            x, y = entry.get('X'), entry.get('Y')
            time = entry.get('Time')
            opcode = entry.get('OpCode', '')
            
            if x is not None and y is not None:
                self.instructions_executed[(x, y)][time].append(opcode)
        except Exception as e:
            pass
    
    def print_cycle_summary(self, cycle: int):
        """Print detailed summary for a single cycle"""
        print(f"\n{'='*100}")
        print(f"CYCLE {cycle}")
        print(f"{'='*100}")
        
        # For each core, print its state
        for y in range(self.grid_rows):
            for x in range(self.grid_cols):
                self._print_core_state(x, y, cycle)
    
    def _print_core_state(self, x: int, y: int, cycle: int):
        """Print state of a single core for a given cycle"""
        core_id = f"PE({x},{y})"
        
        # Check if this core has any activity
        has_dataflow = any(flows and any(t == cycle for t, _, _ in flows) 
                          for (cx, cy, _, _), flows in self.data_flows.items() 
                          if cx == x and cy == y)
        
        has_backpressure = any(e.get('cycle') == cycle 
                              for e in self.backpressure_events.get((x, y), []))
        
        has_instruction = any(cycle in insts 
                             for (cx, cy), insts in self.instructions_executed.items() 
                             if cx == x and cy == y)
        
        if not (has_dataflow or has_backpressure or has_instruction):
            return
        
        print(f"\n{core_id}:")
        
        # Print instructions
        insts = self.instructions_executed.get((x, y), {}).get(cycle, [])
        if insts:
            print(f"  [EXEC] {', '.join(insts)}")
        
        # Print data flows (sends and receives)
        directions = ['North', 'South', 'East', 'West']
        for direction in directions:
            # Sends
            send_key = (x, y, direction, 'Send')
            if send_key in self.data_flows:
                for t, data, pred in self.data_flows[send_key]:
                    if t == cycle:
                        pred_str = f" [PRED={int(pred)}]" if not pred else ""
                        print(f"  [→ {direction}] Data={data}{pred_str}")
            
            # Receives
            recv_key = (x, y, direction, 'Receive')
            if recv_key in self.data_flows:
                for t, data, pred in self.data_flows[recv_key]:
                    if t == cycle:
                        pred_str = f" [PRED={int(pred)}]" if not pred else ""
                        print(f"  [← {direction}] Data={data}{pred_str}")
        
        # Print backpressure
        backpressure = [e for e in self.backpressure_events.get((x, y), []) 
                       if e.get('cycle') == cycle]
        if backpressure:
            for event in backpressure:
                reason = event.get('reason', 'Unknown')
                opcode = event.get('opcode', '?')
                print(f"  [BLOCKED] {opcode}: {reason}")
    
    def print_detailed_table(self, start_cycle: int = 0, end_cycle: int = None, cores: List[Tuple[int, int]] = None):
        """Print detailed table view of all cores over cycle range"""
        if end_cycle is None:
            end_cycle = min(self.max_cycle, start_cycle + 20)
        
        if cores is None:
            cores = [(x, y) for y in range(self.grid_rows) for x in range(self.grid_cols)]
        
        print(f"\n{'='*150}")
        print(f"DETAILED STATE TABLE: Cycles {start_cycle} to {end_cycle}")
        print(f"{'='*150}")
        
        for x, y in cores:
            print(f"\n{'-'*150}")
            print(f"PE({x},{y}) State Across Cycles")
            print(f"{'-'*150}")
            print(f"{'Cycle':<8} {'Instruction':<15} {'←N':<12} {'←S':<12} {'←E':<12} {'←W':<12} {'→N':<12} {'→S':<12} {'→E':<12} {'→W':<12} {'Status':<20}")
            print(f"{'-'*150}")
            
            for cycle in range(start_cycle, end_cycle + 1):
                insts = self.instructions_executed.get((x, y), {}).get(cycle, [])
                inst_str = ','.join(insts[:2]) if insts else '-'
                
                # Collect all data flows for this cycle
                recv_data = {}
                send_data = {}
                
                for direction in ['North', 'South', 'East', 'West']:
                    recv_key = (x, y, direction, 'Receive')
                    send_key = (x, y, direction, 'Send')
                    
                    for t, data, pred in self.data_flows.get(recv_key, []):
                        if t == cycle:
                            recv_data[direction] = f"{data}"
                    
                    for t, data, pred in self.data_flows.get(send_key, []):
                        if t == cycle:
                            send_data[direction] = f"{data}"
                
                # Check for backpressure
                backpressure = [e for e in self.backpressure_events.get((x, y), []) 
                               if e.get('cycle') == cycle]
                status_str = "BLOCKED" if backpressure else '-'
                
                print(f"{cycle:<8} {inst_str:<15} {recv_data.get('North', '-'):<12} {recv_data.get('South', '-'):<12} {recv_data.get('East', '-'):<12} {recv_data.get('West', '-'):<12} {send_data.get('North', '-'):<12} {send_data.get('South', '-'):<12} {send_data.get('East', '-'):<12} {send_data.get('West', '-'):<12} {status_str:<20}")
    
    def print_data_flow_analysis(self, start_cycle: int = 0, end_cycle: int = None):
        """Print comprehensive data flow analysis"""
        if end_cycle is None:
            end_cycle = self.max_cycle
        
        print(f"\n{'='*120}")
        print(f"DATA FLOW ANALYSIS: Cycles {start_cycle} to {end_cycle}")
        print(f"{'='*120}")
        
        # Group by sender
        flows_by_sender = defaultdict(list)
        for (x, y, direction, behavior), flows in self.data_flows.items():
            if behavior == 'Send':
                for cycle, data, pred in flows:
                    if start_cycle <= cycle <= end_cycle:
                        flows_by_sender[(x, y)].append((cycle, direction, data, pred))
        
        for (x, y) in sorted(flows_by_sender.keys()):
            flows = flows_by_sender[(x, y)]
            print(f"\nPE({x},{y}) Sends:")
            for cycle, direction, data, pred in sorted(flows):
                pred_str = f"(Pred={int(pred)})" if not pred else ""
                print(f"  Cycle {cycle}: → {direction:<6} Data={data:<10} {pred_str}")
    
    def print_backpressure_analysis(self):
        """Print backpressure analysis"""
        print(f"\n{'='*120}")
        print(f"BACKPRESSURE ANALYSIS")
        print(f"{'='*120}")
        
        if not self.backpressure_events:
            print("[+] No backpressure events detected")
            return
        
        for (x, y) in sorted(self.backpressure_events.keys()):
            events = self.backpressure_events[(x, y)]
            print(f"\nPE({x},{y}) Backpressure Events: {len(events)} total")
            
            # Group by reason
            by_reason = defaultdict(int)
            for event in events:
                reason = event.get('reason', 'Unknown')
                by_reason[reason] += 1
            
            for reason, count in sorted(by_reason.items(), key=lambda x: -x[1]):
                print(f"  {reason}: {count} occurrences")
    
    def interactive_mode(self):
        """Interactive debugger mode"""
        print("\n" + "="*100)
        print("CGRA INTERACTIVE DEBUGGER")
        print("="*100)
        print("Commands:")
        print("  c <cycle>           - Show cycle summary")
        print("  t <x> <y> [start] [end] - Show table for core (x,y)")
        print("  f <start> <end>     - Show data flow analysis")
        print("  b                   - Show backpressure analysis")
        print("  q                   - Quit")
        print("="*100)
        
        while True:
            try:
                cmd = input("\n> ").strip().split()
                if not cmd:
                    continue
                
                if cmd[0] == 'c':
                    if len(cmd) > 1:
                        cycle = int(cmd[1])
                        self.print_cycle_summary(cycle)
                    else:
                        print("Usage: c <cycle>")
                
                elif cmd[0] == 't':
                    if len(cmd) >= 3:
                        x, y = int(cmd[1]), int(cmd[2])
                        start = int(cmd[3]) if len(cmd) > 3 else 0
                        end = int(cmd[4]) if len(cmd) > 4 else start + 20
                        self.print_detailed_table(start, end, [(x, y)])
                    else:
                        print("Usage: t <x> <y> [start] [end]")
                
                elif cmd[0] == 'f':
                    start = int(cmd[1]) if len(cmd) > 1 else 0
                    end = int(cmd[2]) if len(cmd) > 2 else self.max_cycle
                    self.print_data_flow_analysis(start, end)
                
                elif cmd[0] == 'b':
                    self.print_backpressure_analysis()
                
                elif cmd[0] == 'q':
                    break
                
            except (ValueError, IndexError) as e:
                print(f"[!] Error: {e}")
            except KeyboardInterrupt:
                break
    
    def generate_html_report(self, output_file: str = "cgra_debug_report.html"):
        """Generate interactive HTML report"""
        print(f"[*] Generating HTML report: {output_file}")
        
        # Generate cycle-by-cycle data for visualization
        cycle_data = []
        for cycle in range(0, self.max_cycle + 1, 5):  # Sample every 5 cycles
            core_states = {}
            for y in range(self.grid_rows):
                for x in range(self.grid_cols):
                    state = {
                        'x': x, 'y': y,
                        'insts': self.instructions_executed.get((x, y), {}).get(cycle, []),
                        'backpressured': any(e.get('cycle') == cycle 
                                           for e in self.backpressure_events.get((x, y), []))
                    }
                    core_states[(x, y)] = state
            cycle_data.append({'cycle': cycle, 'cores': core_states})
        
        html = """
        <html>
        <head>
            <title>CGRA Debug Report</title>
            <style>
                body { font-family: monospace; margin: 20px; }
                table { border-collapse: collapse; }
                td, th { border: 1px solid #ddd; padding: 8px; text-align: center; }
                .core { width: 100px; height: 100px; border: 2px solid black; display: inline-block; margin: 5px; }
                .core.executing { background-color: #90EE90; }
                .core.backpressured { background-color: #FFB6C6; }
                .core.idle { background-color: #E0E0E0; }
                h2 { margin-top: 40px; }
            </style>
        </head>
        <body>
            <h1>CGRA Debug Report</h1>
        """
        
        for cycle_info in cycle_data:
            html += f"<h2>Cycle {cycle_info['cycle']}</h2>"
            html += '<div style="width: 500px; display: inline-block;">'
            
            for y in range(self.grid_rows):
                for x in range(self.grid_cols):
                    state = cycle_info['cores'][(x, y)]
                    if state['backpressured']:
                        css_class = 'backpressured'
                    elif state['insts']:
                        css_class = 'executing'
                    else:
                        css_class = 'idle'
                    
                    inst_text = ','.join(state['insts'][:1]) if state['insts'] else '-'
                    html += f'<div class="core {css_class}" title="PE({x},{y})">{inst_text}</div>'
            
            html += '</div>'
        
        html += """
        </body>
        </html>
        """
        
        with open(output_file, 'w') as f:
            f.write(html)
        
        print(f"[+] HTML report generated: {output_file}")


if __name__ == '__main__':
    if len(sys.argv) < 2:
        print("Usage: python3 debug_cycle_trace.py <log_file>")
        sys.exit(1)
    
    log_file = sys.argv[1]
    debugger = CGRADebugger(log_file)
    debugger.parse_log()
    
    # Generate reports
    debugger.print_backpressure_analysis()
    debugger.print_detailed_table(0, min(20, debugger.max_cycle), [(0, 2), (1, 2), (2, 1), (2, 2), (3, 1), (3, 2)])
    debugger.print_data_flow_analysis(0, min(20, debugger.max_cycle))
    
    # Generate HTML
    debugger.generate_html_report()
    
    # Interactive mode
    print("\nEnter interactive mode? (y/n) ", end='')
    if input().lower() == 'y':
        debugger.interactive_mode()
