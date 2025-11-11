#!/usr/bin/env python3
"""
Interactive CGRA Debugger Console
Query and explore logs interactively
"""

import json
import sys
from collections import defaultdict
from cmd import Cmd

class CGRADebugConsole(Cmd):
    """Interactive CGRA debugger"""
    
    intro = """
    ╔════════════════════════════════════════════════════════════╗
    ║        CGRA Interactive Debugger Console                 ║
    ║  Commands: help, cycle, core, flow, block, quit           ║
    ╚════════════════════════════════════════════════════════════╝
    """
    
    prompt = "cgra> "
    
    def __init__(self, log_file):
        super().__init__()
        self.log_file = log_file
        self.events = []
        self.events_by_cycle = defaultdict(list)
        self.max_cycle = 0
        self._load_log()
    
    def _load_log(self):
        """Load and parse log file"""
        print(f"[*] Loading log file: {self.log_file}")
        
        with open(self.log_file, 'r') as f:
            for line in f:
                if not line.strip():
                    continue
                try:
                    event = json.loads(line)
                    self.events.append(event)
                    time = event.get('Time')
                    if time is not None:
                        self.max_cycle = max(self.max_cycle, time)
                        self.events_by_cycle[int(time)].append(event)
                except:
                    pass
        
        print(f"[+] Loaded {len(self.events)} events, max cycle: {self.max_cycle}\n")
    
    def do_cycle(self, arg):
        """Show all activity in a specific cycle: cycle <N>"""
        try:
            cycle = int(arg.strip())
            events = self.events_by_cycle.get(cycle, [])
            
            if not events:
                print(f"No events in cycle {cycle}")
                return
            
            print(f"\n{'='*120}")
            print(f"CYCLE {cycle} - {len(events)} events")
            print(f"{'='*120}\n")
            
            # Group by core
            by_core = defaultdict(list)
            for event in events:
                x, y = event.get('X'), event.get('Y')
                if x is not None and y is not None:
                    by_core[(x, y)].append(event)
            
            for (x, y) in sorted(by_core.keys()):
                core_events = by_core[(x, y)]
                print(f"PE({x},{y}):")
                
                for event in core_events:
                    msg = event.get('msg', '?')
                    
                    if msg == 'DataFlow':
                        direction = event.get('Direction', '?')
                        data = event.get('Data', '?')
                        behavior = event.get('Behavior', '?')
                        print(f"  ✦ {behavior:8} {direction:6} Data={data}")
                    
                    elif msg == 'Backpressure':
                        reason = event.get('Reason', '?')
                        print(f"  ✗ BLOCKED: {reason[:60]}")
                    
                    elif msg == 'InstExec':
                        opcode = event.get('OpCode', '?')
                        print(f"  ◆ EXEC: {opcode}")
                
                print()
        
        except ValueError:
            print("Usage: cycle <number>")
    
    def do_core(self, arg):
        """Show state of a core at a cycle: core <X> <Y> [start] [end]"""
        try:
            parts = arg.strip().split()
            if len(parts) < 2:
                print("Usage: core <X> <Y> [start_cycle] [end_cycle]")
                return
            
            x, y = int(parts[0]), int(parts[1])
            start = int(parts[2]) if len(parts) > 2 else 0
            end = int(parts[3]) if len(parts) > 3 else min(start + 10, self.max_cycle)
            
            print(f"\n{'='*140}")
            print(f"PE({x},{y}) from Cycle {start} to {end}")
            print(f"{'='*140}\n")
            print(f"{'Cycle':<8} {'Event Type':<15} {'Details':<100}")
            print(f"{'-'*140}\n")
            
            for cycle in range(start, int(end) + 1):
                events = self.events_by_cycle.get(cycle, [])
                core_events = [e for e in events if e.get('X') == x and e.get('Y') == y]
                
                if core_events:
                    for event in core_events:
                        msg = event.get('msg', '?')
                        
                        if msg == 'DataFlow':
                            detail = f"{event.get('Behavior')} {event.get('Direction')} Data={event.get('Data')}"
                        elif msg == 'Backpressure':
                            detail = f"BLOCKED: {event.get('Reason', '?')[:80]}"
                        else:
                            detail = str(event.get('msg', '?'))
                        
                        print(f"{cycle:<8} {msg:<15} {detail:<100}")
                else:
                    print(f"{cycle:<8} {'IDLE':<15}")
        
        except (ValueError, IndexError):
            print("Usage: core <X> <Y> [start_cycle] [end_cycle]")
    
    def do_flow(self, arg):
        """Show data flow at a cycle: flow <cycle> or flow all"""
        try:
            if arg.strip() == 'all':
                start, end = 0, self.max_cycle
            else:
                cycle = int(arg.strip())
                start = end = cycle
            
            print(f"\n{'='*120}")
            print(f"DATA FLOWS (Cycles {start} to {end})")
            print(f"{'='*120}\n")
            
            for cycle in range(int(start), int(end) + 1):
                events = self.events_by_cycle.get(cycle, [])
                flows = [e for e in events if e.get('msg') == 'DataFlow']
                
                if flows:
                    print(f"Cycle {cycle}:")
                    for flow in flows:
                        x, y = flow.get('X'), flow.get('Y')
                        direction = flow.get('Direction', '?')
                        data = flow.get('Data', '?')
                        behavior = flow.get('Behavior', '?')
                        
                        arrow = "→" if behavior == 'Send' else "←"
                        print(f"  {arrow} PE({x},{y}) {behavior:8} {direction:6} Data={data}")
                    print()
        
        except ValueError:
            print("Usage: flow <cycle> or flow all")
    
    def do_block(self, arg):
        """Show backpressure at a cycle: block <cycle>"""
        try:
            cycle = int(arg.strip())
            events = self.events_by_cycle.get(cycle, [])
            blocks = [e for e in events if e.get('msg') == 'Backpressure']
            
            if not blocks:
                print(f"No backpressure events in cycle {cycle}")
                return
            
            print(f"\n{'='*120}")
            print(f"BACKPRESSURE EVENTS - CYCLE {cycle}")
            print(f"{'='*120}\n")
            
            for event in blocks:
                x, y = event.get('X'), event.get('Y')
                reason = event.get('Reason', '?')
                event_type = event.get('Type', '?')
                
                print(f"PE({x},{y}): [{event_type}]")
                print(f"  Reason: {reason}\n")
        
        except ValueError:
            print("Usage: block <cycle>")
    
    def do_stats(self, arg):
        """Show statistics"""
        print(f"\nGCRA Execution Statistics")
        print(f"{'='*60}")
        print(f"Total events: {len(self.events)}")
        print(f"Max cycle: {self.max_cycle}")
        
        # Count by message type
        by_msg = defaultdict(int)
        for event in self.events:
            msg = event.get('msg', 'Unknown')
            by_msg[msg] += 1
        
        print(f"\nEvents by type:")
        for msg in sorted(by_msg.keys()):
            print(f"  {msg:20} {by_msg[msg]:5}")
        
        # Count blocked cycles
        blocked_cycles = 0
        for cycle_events in self.events_by_cycle.values():
            if any(e.get('msg') == 'Backpressure' for e in cycle_events):
                blocked_cycles += 1
        
        print(f"\nCycles with backpressure: {blocked_cycles}/{int(self.max_cycle)+1}")
    
    def do_quit(self, arg):
        """Exit debugger"""
        print("\nGoodbye!")
        return True
    
    def do_exit(self, arg):
        """Exit debugger"""
        return self.do_quit(arg)
    
    def do_help(self, arg):
        """Show help"""
        help_text = """
    Available Commands:
    
    cycle <N>              Show all events in cycle N
    core <X> <Y> [s] [e]   Show core PE(X,Y) state from cycle s to e
    flow <N|all>           Show data flows in cycle N (or all cycles)
    block <N>              Show backpressure events in cycle N
    stats                  Show execution statistics
    
    Examples:
    > cycle 5              # Show cycle 5
    > core 2 2 0 10        # Show PE(2,2) from cycles 0 to 10
    > flow all             # Show all data flows
    > block 3              # Show backpressure in cycle 3
        """
        print(help_text)


if __name__ == '__main__':
    if len(sys.argv) < 2:
        print("Usage: python3 debug_console.py <log_file>")
        sys.exit(1)
    
    console = CGRADebugConsole(sys.argv[1])
    console.cmdloop()
