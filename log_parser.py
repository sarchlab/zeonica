#!/usr/bin/env python3
"""
Enhanced Log Parser for CGRA Simulation
Enhanced CGRA simulation log parser and visualization tool
"""

import re
import json
import matplotlib.pyplot as plt
import matplotlib.patches as patches
import numpy as np
from collections import defaultdict
import os
import argparse
import sys

# CGRA array configuration constants
DEFAULT_GRID_ROWS = 3
DEFAULT_GRID_COLS = 3
class EnhancedLogParser:
    def __init__(self, grid_rows=DEFAULT_GRID_ROWS, grid_cols=DEFAULT_GRID_COLS):
        self.timestamps = defaultdict(list)
        self.device_positions = {}
        self.driver_positions = {}  # 存储Driver位置
        self.grid_rows = grid_rows
        self.grid_cols = grid_cols
        # Pending data tracking: {(source, target): [data_values]}
        self.pending_data = defaultdict(list)
        # Track data flow history for FIFO simulation
        self.data_flow_history = []
        # Track instruction information: {timestamp: {(x, y): instruction_name}}
        self.instructions = defaultdict(dict)
        
    def parse_log_from_stdin(self):
        """Parse log from standard input"""
        try:
            print("Please enter log content (press Ctrl+D to end input):")
            log_content = sys.stdin.read()
            return self.parse_log(log_content)
        except KeyboardInterrupt:
            print("\nInput interrupted")
            return False
        except Exception as e:
            print(f"Error reading input: {e}")
            return False
    
    def parse_log_file(self, file_path):
        """Parse log from file"""
        try:
            with open(file_path, 'r', encoding='utf-8') as f:
                log_content = f.read()
            return self.parse_log(log_content)
        except FileNotFoundError:
            print(f"Error: File not found {file_path}")
            return False
        except Exception as e:
            print(f"Error reading file: {e}")
            return False
    
    def parse_log(self, log_content):
        """Parse log content from JSON format"""
        lines = log_content.strip().split('\n')
        
        for line in lines:
            line = line.strip()
            if not line:
                continue
                
            try:
                # Parse JSON line
                log_entry = json.loads(line)
                
                # Extract timestamp from Time field and round to nearest integer
                timestamp = round(float(log_entry.get('Time', 0)))
                
                # Check if this is a DataFlow message
                if log_entry.get('msg') == 'DataFlow':
                    behavior = log_entry.get('Behavior')
                    data = str(log_entry.get('Data', ''))
                    
                    if behavior in ['Recv', 'Send', 'FeedIn', 'Collect']:
                        # Extract source and destination directly from JSON fields
                        if behavior == 'Recv':
                            src = log_entry.get('Src', '')
                            dst = log_entry.get('Dst', '')
                        elif behavior == 'Send':
                            src = log_entry.get('Src', '')
                            dst = log_entry.get('Dst', '')
                        elif behavior == 'FeedIn':
                            src = log_entry.get('From', '')
                            dst = log_entry.get('To', '')
                        elif behavior == 'Collect':
                            # For Collect, we need to infer the route from the destination
                            src = ''  # Collect doesn't have a clear source
                            dst = log_entry.get('From', '')  # In Collect, 'From' is actually the destination
                        
                        operation = {
                            'type': behavior,
                            'data': data,
                            'src': src,
                            'dst': dst,
                            'content': line
                        }
                        
                        self.timestamps[timestamp].append(operation)
                        
                        # Update data state and pending data tracking
                        self._update_data_state(timestamp, operation)
                        self._update_pending_data(timestamp, operation)
                        
                        # Extract device position information
                        self._extract_device_positions_from_json(log_entry)
                
                # Check if this is an Inst message
                elif log_entry.get('msg') == 'Inst':
                    x = log_entry.get('X', 0)
                    y = log_entry.get('Y', 0)
                    instruction = log_entry.get('OpCode', '')
                    
                    if instruction:
                        # Store instruction at the specified position and timestamp
                        self.instructions[timestamp][(x, y)] = instruction
                        
            except json.JSONDecodeError:
                # Skip invalid JSON lines
                continue
            except Exception as e:
                # Skip lines that cause other errors
                continue
        
        return True
    
    def _extract_device_positions_from_json(self, log_entry):
        """Extract device position information from JSON log entry"""
        # Extract Tile positions from various fields
        for field in ['Src', 'Dst', 'From', 'To']:
            value = log_entry.get(field, '')
            if value:
                # Match Tile positions
                tile_matches = re.finditer(r'Device\.Tile\[(\d+)\]\[(\d+)\]', value)
                for match in tile_matches:
                    row, col = int(match.group(1)), int(match.group(2))
                    device_id = f"Tile[{row}][{col}]"
                    self.device_positions[device_id] = (row, col)
                
                # Match Driver positions
                driver_matches = re.finditer(r'Driver\.Device(\w+)\[(\d+)\]', value)
                for match in driver_matches:
                    direction = match.group(1)
                    index = int(match.group(2))
                    self.driver_positions[f"Driver_{direction}_{index}"] = (direction, index)
    
    def _extract_device_positions(self, content):
        """Extract device position information (legacy method for backward compatibility)"""
        # Match Tile positions
        tile_matches = re.finditer(r'Device\.Tile\[(\d+)\]\[(\d+)\]', content)
        for match in tile_matches:
            row, col = int(match.group(1)), int(match.group(2))
            device_id = f"Tile[{row}][{col}]"
            self.device_positions[device_id] = (row, col)
        
        # Match Driver positions
        driver_matches = re.finditer(r'Driver\.Device(\w+)\[(\d+)\]', content)
        for match in driver_matches:
            direction = match.group(1)
            index = int(match.group(2))
            self.driver_positions[f"Driver_{direction}_{index}"] = (direction, index)
    
    def _parse_device_position(self, device_string):
        """Parse device position from device string (e.g., 'Device.Tile[0][1].Core.North')"""
        if not device_string:
            return None
            
        # Handle Tile positions
        tile_match = re.search(r'Device\.Tile\[(\d+)\]\[(\d+)\]\.Core\.(\w+)', device_string)
        if tile_match:
            row = int(tile_match.group(1))
            col = int(tile_match.group(2))
            port = tile_match.group(3)
            return ('tile', row, col, port)
        
        # Handle Driver positions
        driver_match = re.search(r'Driver\.Device(\w+)\[(\d+)\]', device_string)
        if driver_match:
            direction = driver_match.group(1)
            index = int(driver_match.group(2))
            
            # Determine Driver position based on direction (row, col)
            if direction == 'West':
                return ('driver', index, -1, 'East')
            elif direction == 'East':
                return ('driver', index, self.grid_cols, 'West')
            elif direction == 'North':
                return ('driver', -1, index, 'South')
            elif direction == 'South':
                return ('driver', self.grid_rows, index, 'North')
        
        return None
    
    def _get_instruction_at_position(self, timestamp, x, y):
        """Get instruction at specific position and timestamp"""
        # Check if there's an instruction at this timestamp and position
        if timestamp in self.instructions:
            position = (x, y)
            if position in self.instructions[timestamp]:
                return self.instructions[timestamp][position]
        return None
    
    def _parse_route(self, route):
        """Parse route information"""
        # Handle Tile to Tile routing FIRST (most specific pattern)
        tile_match = re.search(r'Device\.Tile\[(\d+)\]\[(\d+)\]\.Core\.(\w+)->Device\.Tile\[(\d+)\]\[(\d+)\]\.Core\.(\w+)', route)
        if tile_match:
            source_row = int(tile_match.group(1))
            source_col = int(tile_match.group(2))
            source_port = tile_match.group(3)
            target_row = int(tile_match.group(4))
            target_col = int(tile_match.group(5))
            target_port = tile_match.group(6)
            
            source = ('tile', source_row, source_col, source_port)
            target = ('tile', target_row, target_col, target_port)
            return source, target
        
        # Handle Tile to Driver routing (Send operations)
        tile_to_driver_match = re.search(r'Device\.Tile\[(\d+)\]\[(\d+)\]\.Core\.(\w+)->Driver\.Device(\w+)\[(\d+)\]', route)
        if tile_to_driver_match:
            source_row = int(tile_to_driver_match.group(1))
            source_col = int(tile_to_driver_match.group(2))
            source_port = tile_to_driver_match.group(3)
            direction = tile_to_driver_match.group(4)
            driver_index = int(tile_to_driver_match.group(5))
            
            source = ('tile', source_row, source_col, source_port)
            
            # Determine Driver position based on direction (row, col)
            if direction == 'West':
                target = ('driver', driver_index, -1, 'East')
            elif direction == 'East':
                target = ('driver', driver_index, self.grid_cols, 'West')
            elif direction == 'North':
                target = ('driver', self.grid_rows, driver_index, 'South')
            elif direction == 'South':
                target = ('driver', -1, driver_index, 'North')
            else:
                target = None
            
            return source, target
        
        # Handle Driver to Tile routing (Recv operations)
        driver_match = re.search(r'Driver\.Device(\w+)\[(\d+)\]->Device\.Tile\[(\d+)\]\[(\d+)\]\.Core\.(\w+)', route)
        if driver_match:
            direction = driver_match.group(1)
            driver_index = int(driver_match.group(2))
            tile_row = int(driver_match.group(3))
            tile_col = int(driver_match.group(4))
            port = driver_match.group(5)
            
            # Driver position determined by direction (row, col)
            # West drivers sit at (row=driver_index, col=-1), sending East
            # East drivers sit at (row=driver_index, col=self.grid_cols), sending West
            # North drivers sit at (row=self.grid_rows, col=driver_index), sending South
            # South drivers sit at (row=-1, col=driver_index), sending North
            if direction == 'West':
                source = ('driver', driver_index, -1, 'East')
            elif direction == 'East':
                source = ('driver', driver_index, self.grid_cols, 'West')
            elif direction == 'North':
                source = ('driver', self.grid_rows, driver_index, 'South')
            elif direction == 'South':
                source = ('driver', -1, driver_index, 'North')
            else:
                source = None
            
            target = ('tile', tile_row, tile_col, port)
            return source, target
        
        # Handle Collect operation routing - from Tile to Driver
        # For JSON format, route contains only the Driver destination
        collect_match = re.search(r'Driver\.Device(\w+)\[(\d+)\]', route)
        if collect_match:
            direction = collect_match.group(1)
            driver_index = int(collect_match.group(2))
            
            # Collect operation: data flows from Tile to Driver
            # Determine boundary Tile data source based on Driver direction
            if direction == 'West':
                # Data from westernmost Tile (column 0)
                source = ('tile', driver_index, 0, 'West')
                target = ('driver', driver_index, -1, 'East')
            elif direction == 'East':
                # Data from easternmost Tile (last column)
                source = ('tile', driver_index, self.grid_cols - 1, 'East')
                target = ('driver', driver_index, self.grid_cols, 'West')
            elif direction == 'North':
                # Data from northernmost Tile (row 0)
                source = ('tile', 0, driver_index, 'North')
                target = ('driver', self.grid_rows, driver_index, 'South')
            elif direction == 'South':
                # Data from southernmost Tile (last row)
                source = ('tile', self.grid_rows - 1, driver_index, 'South')
                target = ('driver', -1, driver_index, 'North')
            else:
                return None, None
            
            return source, target
        
        # Handle Feed in operation routing - from Driver to Tile
        # Feed in format: "Device.Tile[0][0].Core.West" (route parameter only contains the part after 'to')
        feed_match = re.search(r'Device\.Tile\[(\d+)\]\[(\d+)\]\.Core\.(\w+)', route)
        if feed_match:
            tile_row = int(feed_match.group(1))
            tile_col = int(feed_match.group(2))
            port = feed_match.group(3)
            
            # Feed in operation: data flows from Driver to Tile
            # Determine corresponding Driver based on port direction (row, col)
            if port == 'West':
                # Input from West Driver (driver at col=-1)
                source = ('driver', tile_row, -1, 'East')
            elif port == 'East':
                # Input from East Driver (driver at col=self.grid_cols)
                source = ('driver', tile_row, self.grid_cols, 'West')
            elif port == 'North':
                # Input from North Driver (driver at row=self.grid_rows)
                source = ('driver', self.grid_rows, tile_col, 'North')
            elif port == 'South':
                # Input from South Driver (driver at row=-1)
                source = ('driver', -1, tile_col , 'South')
            else:
                source = None
            
            target = ('tile', tile_row, tile_col, port)
            return source, target
        
        return None, None
    
    def _update_data_state(self, timestamp, operation):
        """Update data state"""
        if operation['type'] == 'Collect':
            # Parse Collect operation, data collected from Driver
            # For JSON format, dst contains the destination (Driver)
            dst = operation.get('dst', '')
            driver_match = re.search(r'Driver\.Device(\w+)\[(\d+)\]', dst)
            if driver_match:
                direction = driver_match.group(1)
                index = int(driver_match.group(2))
                
                # Add to Driver position information
                self.driver_positions[f"Driver_{direction}_{index}"] = (direction, index)
    
    def _update_pending_data(self, timestamp, operation):
        """Update pending data tracking for FIFO simulation"""
        data_value = operation['data']
        src = operation.get('src', '')
        dst = operation.get('dst', '')
        
        # Parse source and destination directly from JSON fields
        source = self._parse_device_position(src)
        target = self._parse_device_position(dst)
        
        if not source or not target:
            return
        
        # Create link key (source -> target)
        link_key = (source, target)
        
        # Record data flow event
        flow_event = {
            'timestamp': timestamp,
            'operation_type': operation['type'],
            'data_value': data_value,
            'source': source,
            'target': target,
            'link_key': link_key
        }
        self.data_flow_history.append(flow_event)
        
        # Update pending data based on operation type
        if operation['type'] in ['Send', 'FeedIn']:
            # Data is sent/feed - add to pending queue
            self.pending_data[link_key].append(data_value)
        elif operation['type'] in ['Recv', 'Collect']:
            # Data is received/collected - remove from pending queue (FIFO)
            if link_key in self.pending_data and self.pending_data[link_key]:
                # Remove the oldest data (FIFO behavior)
                self.pending_data[link_key].pop(0)
    
    def get_grid_size(self):
        """Get grid size"""
        if not self.device_positions:
            return (self.grid_rows, self.grid_cols)  # Use configured default values
        
        max_row = max(pos[0] for pos in self.device_positions.values()) + 1
        max_col = max(pos[1] for pos in self.device_positions.values()) + 1
        return (max_row, max_col)
    
    def create_visualization(self, timestamp, operations, output_dir="output"):
        """Create visualization chart for specified timestamp"""
        # Remove early return - we want to generate charts even when there are no operations
        
        # Create output directory
        os.makedirs(output_dir, exist_ok=True)
        
        # Get grid size
        grid_rows, grid_cols = self.get_grid_size()
        
        # Create figure
        fig, ax = plt.subplots(figsize=(15, 12))
        # Set display range: include Driver and Tile
        ax.set_xlim(-1.5, grid_cols + 0.5)
        ax.set_ylim(-1.5, grid_rows + 0.5)
        ax.set_aspect('equal')
        
        # Draw grid
        self._draw_grid(ax, grid_rows, grid_cols)
        
        # Draw devices
        self._draw_devices(ax, grid_rows, grid_cols, timestamp)
        
        # Draw Driver
        self._draw_drivers(ax, grid_rows, grid_cols)
        
        # Draw data transmission arrows
        self._draw_data_arrows(ax, operations, grid_rows, grid_cols)
        
        # Draw FIFO boxes with pending data
        self._draw_fifo_boxes(ax, timestamp, grid_rows, grid_cols)
        
        # Set title and labels
        ax.set_title(f'CGRA Communication at Time {timestamp:.3f}', fontsize=16, fontweight='bold')
        ax.set_xlabel('Column', fontsize=14)
        ax.set_ylabel('Row', fontsize=14)
        
        # Save image
        filename = f"{output_dir}/timestamp_{timestamp:.3f}.png"
        plt.savefig(filename, dpi=300, bbox_inches='tight')
        plt.close()
        
        print(f"Generated visualization: {filename}")
    
    def _draw_grid(self, ax, rows, cols):
        """Draw grid lines"""
        for i in range(rows + 1):
            ax.axhline(y=i-0.5, color='gray', alpha=0.3, linewidth=0.5)
        for j in range(cols + 1):
            ax.axvline(x=j-0.5, color='gray', alpha=0.3, linewidth=0.5)
    
    def _draw_devices(self, ax, rows, cols, timestamp):
        """Draw device squares"""
        for row in range(rows):
            for col in range(cols):
                tile_id = f"Tile[{row}][{col}]"
                
                # Draw device square
                rect = patches.Rectangle((col-0.4, row-0.4), 0.8, 0.8, 
                                       linewidth=2, edgecolor='black', 
                                       facecolor='lightblue', alpha=0.8)
                ax.add_patch(rect)
                
                # Add device label
                ax.text(col, row, f'Tile[{row}][{col}]', 
                       ha='center', va='center', fontsize=10, fontweight='bold')
                
                # Add instruction label in the upper-middle part of the device if available
                instruction = self._get_instruction_at_position(timestamp, col, row)
                if instruction:
                    ax.text(col, row + 0.25, instruction, 
                           ha='center', va='center', fontsize=7, fontweight='bold',
                           bbox=dict(boxstyle='round,pad=0.15', 
                                   facecolor='lightgreen', 
                                   edgecolor='darkgreen',
                                   alpha=0.9))
    
    def _draw_drivers(self, ax, rows, cols):
        """Draw Driver devices"""
        for driver_id, (direction, index) in self.driver_positions.items():
            if direction == 'West':
                x, y = -1, index
                label = f'Driver\nWest[{index}]'
            elif direction == 'East':
                x, y = cols, index
                label = f'Driver\nEast[{index}]'
            elif direction == 'North':
                x, y = index, -1
                label = f'Driver\nNorth[{index}]'
            elif direction == 'South':
                x, y = index, rows
                label = f'Driver\nSouth[{index}]'
            else:
                continue
            
            # Draw Driver circle
            circle = patches.Circle((x, y), 0.3, linewidth=2, edgecolor='red', 
                                  facecolor='orange', alpha=0.8)
            ax.add_patch(circle)
            
            # Add Driver label
            ax.text(x, y, label, ha='center', va='center', fontsize=9, fontweight='bold')
    
    def _draw_data_arrows(self, ax, operations, grid_rows, grid_cols):
        """Draw data transmission arrows"""
        # Define colors and styles: Collect and Recv use green dashed, Send and Feed use red solid
        receive_style = {'Recv': ('green', 'dashed'), 'Collect': ('green', 'dashed')}
        send_style = {'Send': ('red', 'solid'), 'FeedIn': ('red', 'solid')}
        
        for operation in operations:
            
            op_type = operation['type']
            data_value = operation['data']
            src = operation.get('src', '')
            dst = operation.get('dst', '')
            
            # Parse source and destination directly from JSON fields
            source = self._parse_device_position(src)
            target = self._parse_device_position(dst)
            
            print("Drawing data arrows for operation: ", operation)
            print("Type: ", op_type)
            print("Source: ", source)
            print("Target: ", target)
            
            if not source or not target:
                continue
            
            # Get source and target coordinates
            source_pos = self._get_position_coordinates(source, grid_rows, grid_cols)
            target_pos = self._get_position_coordinates(target, grid_rows, grid_cols)
            
            if not source_pos or not target_pos:
                continue
            
            # Determine arrow color and style
            if op_type in receive_style:
                arrow_color, linestyle = receive_style[op_type]
                arrow_style = '->'
            elif op_type in send_style:
                arrow_color, linestyle = send_style[op_type]
                arrow_style = '->'
            else:
                continue
            
            # Calculate arrow positions with half length
            # For Send/Feed: keep source position, move target closer
            # For Collect/Recv: keep target position, move source closer
            if op_type in ['Send', 'FeedIn']:
                # Keep source position, move target halfway towards source
                arrow_source = source_pos
                arrow_target = ((source_pos[0] + target_pos[0]) / 2, (source_pos[1] + target_pos[1]) / 2)
            else:  # Collect/Recv
                # Keep target position, move source halfway towards target
                arrow_source = ((source_pos[0] + target_pos[0]) / 2, (source_pos[1] + target_pos[1]) / 2)
                arrow_target = target_pos
            
            # Data label at the middle of the shortened arrow
            label_x = (arrow_source[0] + arrow_target[0]) / 2
            label_y = (arrow_source[1] + arrow_target[1]) / 2
            
            # Draw arrow
            ax.annotate('', xy=arrow_target, xytext=arrow_source,
                       arrowprops=dict(arrowstyle=arrow_style, 
                                     color=arrow_color, 
                                     linestyle=linestyle,
                                     lw=2, 
                                     alpha=0.8,
                                     shrinkA=0.3,
                                     shrinkB=0.3))
            
            # Add data value label at calculated position
            ax.text(label_x, label_y, data_value, 
                   ha='center', va='center', 
                   fontsize=8, fontweight='bold',
                   bbox=dict(boxstyle='round,pad=0.2', 
                           facecolor='white', 
                           edgecolor=arrow_color,
                           alpha=0.8))
    
    def _get_position_coordinates(self, position, grid_rows, grid_cols):
        """Get position coordinates"""
        pos_type, row, col, port = position
        
        if pos_type == 'tile':
            # Tile position
            return (col, row)
        elif pos_type == 'driver':
            # Driver position: row and col directly represent coordinates on the boundary
            return (col, row)
        
        return None
    
    def _draw_fifo_boxes(self, ax, timestamp, grid_rows, grid_cols):
        """Draw FIFO boxes showing pending data on links"""
        # Get current pending data state at this timestamp
        current_pending = self._get_pending_data_at_timestamp(timestamp)
        
        for link_key, pending_values in current_pending.items():
            if not pending_values:  # Skip empty FIFOs
                continue
                
            source, target = link_key
            source_pos = self._get_position_coordinates(source, grid_rows, grid_cols)
            target_pos = self._get_position_coordinates(target, grid_rows, grid_cols)
            
            if not source_pos or not target_pos:
                continue
            
            # Calculate FIFO box position (middle of link, slightly above)
            fifo_x = (source_pos[0] + target_pos[0]) / 2
            fifo_y = (source_pos[1] + target_pos[1]) / 2 + 0.15  # Slightly above the link
            
            # Add data values only
            data_text = ', '.join(map(str, pending_values))
            ax.text(fifo_x, fifo_y, data_text,
                   ha='center', va='center',
                   fontsize=7, fontweight='bold',
                   bbox=dict(boxstyle='round,pad=0.1',
                           facecolor='yellow',
                           edgecolor='black',
                           alpha=0.9))
    
    def _get_pending_data_at_timestamp(self, timestamp):
        """Get pending data state at a specific timestamp"""
        # Reconstruct pending data state at this timestamp
        temp_pending = defaultdict(list)
        
        for event in self.data_flow_history:
            if event['timestamp'] <= timestamp:
                link_key = event['link_key']
                if event['operation_type'] in ['Send', 'FeedIn']:
                    # Add data to FIFO
                    temp_pending[link_key].append(event['data_value'])
                elif event['operation_type'] in ['Recv', 'Collect']:
                    # Remove data from FIFO (FIFO behavior)
                    if link_key in temp_pending and temp_pending[link_key]:
                        temp_pending[link_key].pop(0)
        
        return temp_pending
    

    

    
    def print_parsed_data(self):
        """Print parsed data"""
        if not self.timestamps:
            print("No valid timestamp data found")
            return
        
        print(f"\n=== Parsed Data Overview ===")
        print(f"Total timestamps found: {len(self.timestamps)}")
        print(f"Device positions: {len(self.device_positions)} Tiles")
        print(f"Driver positions: {len(self.driver_positions)} Drivers")
        print(f"Instructions found: {sum(len(inst_dict) for inst_dict in self.instructions.values())}")
        print(f"Configured grid size: {self.grid_rows}x{self.grid_cols}")
        
        print(f"\n=== Device Position Information ===")
        for device_id, (row, col) in self.device_positions.items():
            print(f"  {device_id}: position({row}, {col})")
        
        print(f"\n=== Driver Position Information ===")
        for driver_id, (direction, index) in self.driver_positions.items():
            print(f"  {driver_id}: direction={direction}, index={index}")
        
        print(f"\n=== Detailed Data by Timestamp ===")
        for timestamp in sorted(self.timestamps.keys()):
            operations = self.timestamps[timestamp]
            print(f"\nTimestamp {timestamp}:")
            print(f"  Number of operations: {len(operations)}")
            
            # Show instructions at this timestamp
            if timestamp in self.instructions:
                print(f"  Instructions:")
                for (x, y), instruction in self.instructions[timestamp].items():
                    print(f"    Position ({x}, {y}): {instruction}")
            
            # Show pending data at this timestamp
            current_pending = self._get_pending_data_at_timestamp(timestamp)
            if current_pending:
                print(f"  Pending data in FIFOs:")
                for link_key, pending_values in current_pending.items():
                    source, target = link_key
                    print(f"    {source} -> {target}: {pending_values}")
            
            for i, op in enumerate(operations, 1):
                print(f"  Operation {i}:")
                print(f"    Type: {op['type']}")
                print(f"    Data: {op['data']}")
                print(f"    Source: {op.get('src', '')}")
                print(f"    Target: {op.get('dst', '')}")
                
                # Parse route information and display source and target
                source = self._parse_device_position(op.get('src', ''))
                target = self._parse_device_position(op.get('dst', ''))
                if source and target:
                    print(f"    Parsed Source: {source}")
                    print(f"    Parsed Target: {target}")
                else:
                    print(f"    Route parsing failed")
                print()
    
    def process_all_timestamps(self, output_dir="output"):
        """Process all timestamps and generate visualizations"""
        if not self.timestamps:
            print("No valid timestamp data found")
            return
        
        # Print parsed data
        self.print_parsed_data()
        
        print(f"\n=== Generating Visualization Charts ===")
        print(f"Found {len(self.timestamps)} timestamps with data")
        
        # Find the maximum timestamp from the data
        max_timestamp = max(self.timestamps.keys()) if self.timestamps else 0
        
        # Generate charts from 0 to max_timestamp (inclusive)
        start_timestamp = 0
        
        print(f"Generating charts from {start_timestamp} to {max_timestamp} (total: {max_timestamp - start_timestamp + 1} charts)")
        
        generated_count = 0
        
        for current_timestamp in range(start_timestamp, max_timestamp + 1):
            # Get operations for this timestamp (empty list if no data)
            operations = self.timestamps.get(current_timestamp, [])
            
            print(f"Processing timestamp {current_timestamp}: {len(operations)} operations")
            self.create_visualization(current_timestamp, operations, output_dir)
            
            generated_count += 1
        
        print(f"All {generated_count} visualization charts saved to {output_dir} directory")

def main():
    parser = argparse.ArgumentParser(description='CGRA Log Parser and Visualizer')
    parser.add_argument('log_file', nargs='?', help='Log file path (optional, if not provided will read from standard input)')
    parser.add_argument('-o', '--output', default='output', help='Output directory (default: output)')
    parser.add_argument('--rows', type=int, default=DEFAULT_GRID_ROWS, help=f'CGRA grid rows (default: {DEFAULT_GRID_ROWS})')
    parser.add_argument('--cols', type=int, default=DEFAULT_GRID_COLS, help=f'CGRA grid columns (default: {DEFAULT_GRID_COLS})')
    
    args = parser.parse_args()
    
    # Create parser
    log_parser = EnhancedLogParser(grid_rows=args.rows, grid_cols=args.cols)
    
    # Parse log
    if args.log_file:
        # Read from file
        success = log_parser.parse_log_file(args.log_file)
    else:
        # Read from standard input
        success = log_parser.parse_log_from_stdin()
    
    if success:
        # Generate visualization
        log_parser.process_all_timestamps(args.output)
    else:
        print("Failed to parse log")

if __name__ == "__main__":
    # If no command line arguments, use example data
    import sys
    if len(sys.argv) == 1:
        # Example usage
        log_content = """
 8.000000, Device.Tile[0][0].Core, Inst {[{GPRED {[{false R East}]} {[{false R West} {false R South}]}}]}
  8.000000, Device.Tile[0][0].Core, Recv 9 Driver.DeviceWest[0]->Device.Tile[0][0].Core.West, Color 0
  8.000000, Device.Tile[1][0].Core, Send 1 Device.Tile[1][0].Core.North->Device.Tile[0][0].Core.South, Color 0
  8.000000, Device.Tile[1][0].Core, inst: {[{MOV {[{false R East}]} {[{false R West}]}}]} inst_length: 1
  8.000000, Device.Tile[1][0].Core, Recv 0 Driver.DeviceWest[1]->Device.Tile[1][0].Core.West, Color 0
  8.000000, Device.Tile[0][1].Core, inst: {[{GPRED {[{false R East}]} {[{false R West} {false R South}]}}]} inst_length: 1
  8.000000, Device.Tile[0][1].Core, Recv 8 Device.Tile[0][0].Core.East->Device.Tile[0][1].Core.West, Color 0
  8.000000, Device.Tile[1][1].Core, Send 0 Device.Tile[1][1].Core.North->Device.Tile[0][1].Core.South, Color 0
  9.000000, Device.Tile[0][0].Core, Send 5 Device.Tile[0][0].Core.East->Device.Tile[0][1].Core.West, Color 0
  9.000000, Device.Tile[1][1].Core, Recv 3 Device.Tile[1][0].Core.South->Device.Tile[1][1].Core.North, Color 0
"""
        
        log_parser = EnhancedLogParser()
        log_parser.parse_log(log_content)
        log_parser.process_all_timestamps()
        print("Visualization generation with example data completed!")
    else:
        main()
