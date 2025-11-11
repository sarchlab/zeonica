#!/usr/bin/env python3
"""
Interactive CGRA Debugger with detailed state visualization
"""

import json
import sys
import os
from collections import defaultdict

try:
    import yaml
    YAML_AVAILABLE = True
except ImportError:
    YAML_AVAILABLE = False

def generate_interactive_html(log_file, output_file="cgra_debug.html"):
    """Generate interactive HTML debugger"""
    
    # Extract kernel name from log file path
    # e.g., test/axpy/axpy_run.log -> AXPY
    kernel_name = os.path.basename(os.path.dirname(log_file)).upper()
    
    # Try to find and parse YAML file for expected instructions
    expected_schedule = {}
    yaml_file = None
    if YAML_AVAILABLE:
        # Look for YAML file in the same directory or parent
        log_dir = os.path.dirname(log_file)
        kernel_lower = kernel_name.lower()
        
        # Try possible YAML locations
        possible_yaml_paths = [
            os.path.join(log_dir, f"{kernel_lower}.yaml"),
            os.path.join(log_dir, f"{kernel_lower}_run.yaml"),
            os.path.join(os.path.dirname(log_dir), "Zeonica_Testbench", "kernel", kernel_lower, f"{kernel_lower}.yaml"),
        ]
        
        for yaml_path in possible_yaml_paths:
            if os.path.exists(yaml_path):
                yaml_file = yaml_path
                print(f"[*] Found YAML schedule: {yaml_file}")
                try:
                    with open(yaml_file, 'r') as f:
                        yaml_data = yaml.safe_load(f)
                    
                    # Parse YAML to extract expected instruction schedule
                    if 'array_config' in yaml_data:
                        array_config = yaml_data['array_config']
                        for core in array_config.get('cores', []):
                            x = core.get('column')
                            y = core.get('row')
                            pe_key = (x, y)
                            expected_schedule[pe_key] = {}
                            
                            for entry in core.get('entries', []):
                                for instr in entry.get('instructions', []):
                                    timestep = instr.get('timestep')
                                    for op in instr.get('operations', []):
                                        opcode = op.get('opcode')
                                        expected_schedule[pe_key][timestep] = opcode
                    
                    print(f"[+] Loaded expected schedule for {len(expected_schedule)} PEs")
                except Exception as e:
                    print(f"[-] Error parsing YAML: {e}")
                    expected_schedule = {}
                break
    
    # Parse log - both legacy format and PEState
    events_by_cycle = defaultdict(list)
    pestate_by_cycle = defaultdict(dict)
    max_cycle = 0
    cores = set()
    has_pestate = False
    
    print(f"[*] Parsing {log_file}...")
    with open(log_file, 'r') as f:
        for line in f:
            if not line.strip():
                continue
            try:
                event = json.loads(line)
                
                # Check for PEState entries (new format)
                if event.get('msg') == 'PEState':
                    has_pestate = True
                    state = event.get('state', {})
                    cycle = state.get('time', 0)
                    x = state.get('x')
                    y = state.get('y')
                    if x is not None and y is not None:
                        max_cycle = max(max_cycle, cycle)
                        cores.add((x, y))
                        pestate_by_cycle[cycle][(x, y)] = state
                else:
                    # Legacy format
                    time = event.get('Time')
                    if time is not None:
                        max_cycle = max(max_cycle, time)
                        events_by_cycle[int(time)].append(event)
                        x, y = event.get('X'), event.get('Y')
                        if x is not None and y is not None:
                            cores.add((x, y))
            except:
                pass
    
    cores = sorted(cores)
    
    # Generate HTML
    html = """<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>CGRA Interactive Debugger</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Courier New', monospace;
            background-color: #1e1e1e;
            color: #d4d4d4;
            padding: 20px;
        }
        
        .container {
            max-width: 1600px;
            margin: 0 auto;
        }
        
        h1 {
            color: #4ec9b0;
            margin-bottom: 20px;
            text-align: center;
        }
        
        .controls {
            display: flex;
            gap: 20px;
            margin-bottom: 20px;
            padding: 15px;
            background-color: #252526;
            border-radius: 5px;
            flex-wrap: wrap;
            align-items: center;
        }
        
        .control-group {
            display: flex;
            gap: 10px;
            align-items: center;
        }
        
        label {
            color: #9cdcfe;
            font-weight: bold;
        }
        
        input[type="range"], input[type="number"] {
            padding: 5px;
            background-color: #3e3e42;
            color: #d4d4d4;
            border: 1px solid #555;
            border-radius: 3px;
        }
        
        button {
            padding: 8px 15px;
            background-color: #0e639c;
            color: white;
            border: none;
            border-radius: 3px;
            cursor: pointer;
            font-family: 'Courier New', monospace;
        }
        
        button:hover {
            background-color: #1177bb;
        }
        
        button.primary {
            background-color: #008000;
            padding: 10px 20px;
            font-weight: bold;
        }
        
        button.primary:hover {
            background-color: #00b000;
        }
        
        .modal {
            display: none;
            position: fixed;
            z-index: 1000;
            left: 0;
            top: 0;
            width: 100%;
            height: 100%;
            background-color: rgba(0,0,0,0.6);
            overflow: auto;
        }
        
        .modal.show {
            display: flex;
            align-items: center;
            justify-content: center;
        }
        
        .modal-content {
            background-color: #252526;
            padding: 20px;
            border: 2px solid #4ec9b0;
            border-radius: 10px;
            width: 90%;
            max-width: 1400px;
            max-height: 90vh;
            overflow-y: auto;
        }
        
        .modal-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 15px;
        }
        
        .modal-title {
            color: #4ec9b0;
            font-size: 18px;
            font-weight: bold;
        }
        
        .close-btn {
            color: #858585;
            font-size: 28px;
            font-weight: bold;
            cursor: pointer;
            background: none;
            border: none;
            padding: 0;
            width: 30px;
            height: 30px;
        }
        
        .close-btn:hover {
            color: #d4d4d4;
        }
        
        #timelineContainer {
            max-height: 600px;
            overflow: auto;
            margin-bottom: 10px;
        }
        
        .timeline-legend {
            display: flex;
            gap: 20px;
            margin-top: 10px;
            flex-wrap: wrap;
        }
        
        .legend-item {
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 12px;
        }
        
        /* Expected Schedule Table Styles */
        #expectedScheduleContainer table {
            font-family: 'Courier New', monospace;
        }
        
        #expectedScheduleContainer th {
            text-align: left;
            font-weight: bold;
            background-color: #333;
        }
        
        .legend-color {
            width: 20px;
            height: 20px;
            border-radius: 2px;
        }
        
        .timeline-info {
            margin-top: 15px;
            padding: 10px;
            background-color: #1e1e1e;
            border-left: 3px solid #0e639c;
            border-radius: 3px;
            font-size: 12px;
        }
        
        .content {
            display: flex;
            gap: 20px;
        }
        
        .grid-view {
            flex: 2;
        }
        
        .detail-view {
            flex: 1;
            background-color: #252526;
            border: 1px solid #555;
            border-radius: 5px;
            padding: 15px;
            max-height: 800px;
            overflow-y: auto;
        }
        
        .cycle-grid {
            margin-bottom: 20px;
            background-color: #1e1e1e;
            padding: 20px;
            border: 1px solid #555;
            border-radius: 5px;
            min-height: 650px;
        }
        
        .grid-wrapper {
            position: relative;
            width: 100%;
            height: 600px;
        }
        
        .grid-svg-overlay {
            position: absolute;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            pointer-events: none;
            z-index: 10;
        }
        
        .grid-container {
            position: absolute;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            z-index: 5;
            display: grid;
            grid-template-columns: repeat(4, 1fr);
            gap: 10px;
            padding: 10px;
        }
        
        @keyframes arrow-pulse {
            0% { opacity: 0.7; stroke-width: 3px; }
            50% { opacity: 0.85; stroke-width: 3.5px; }
            100% { opacity: 0.7; stroke-width: 3px; }
        }
        
        .data-flow-arrow {
            animation: arrow-pulse 2.5s ease-in-out infinite;
        }
        
        .pe-core {
            border: 2px solid #555;
            border-radius: 5px;
            padding: 15px;
            cursor: pointer;
            background-color: #2d2d30;
            min-height: 120px;
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }
        
        .pe-core:hover {
            border-color: #4ec9b0;
        }
        
        .pe-core.selected {
            border-color: #f44747;
            background-color: #3e2222;
        }
        
        .pe-core.active {
            border-color: #4caf50;
            background-color: #1b5e20;
        }
        
        .pe-core.blocked {
            border-color: #f44747;
            background-color: #3e2222;
        }
        
        .pe-label {
            font-weight: bold;
            color: #9cdcfe;
            margin-bottom: 8px;
            font-size: 14px;
        }
        
        .pe-info {
            font-size: 12px;
            color: #858585;
            line-height: 1.5;
        }
        
        .info-line {
            margin: 4px 0;
        }
        
        .status-label {
            color: #ce9178;
            font-weight: bold;
        }
        
        .status-idle {
            color: #858585;
        }
        
        .status-exec {
            color: #4ec9b0;
        }
        
        .status-blocked {
            color: #f44747;
        }
        
        .data-flow {
            color: #dcdcaa;
            font-size: 11px;
            margin: 2px 0;
        }
        
        .direction-north { color: #569cd6; }
        .direction-south { color: #569cd6; }
        .direction-east { color: #dcdcaa; }
        .direction-west { color: #dcdcaa; }
        
        .detail-title {
            color: #4ec9b0;
            font-size: 14px;
            font-weight: bold;
            margin-bottom: 10px;
            border-bottom: 1px solid #555;
            padding-bottom: 5px;
        }
        
        .detail-item {
            margin: 8px 0;
            padding: 8px;
            background-color: #1e1e1e;
            border-left: 3px solid #0e639c;
            border-radius: 2px;
        }
        
        .detail-key {
            color: #9cdcfe;
            font-weight: bold;
        }
        
        .detail-value {
            color: #ce9178;
            margin-left: 5px;
        }
        
        .recv-buffer {
            margin: 5px 0;
            padding: 5px;
            background-color: #1a1a1a;
            border-left: 2px solid #555;
            font-size: 11px;
        }
        
        .recv-ready-true {
            color: #4ec9b0;
        }
        
        .recv-ready-false {
            color: #f44747;
        }
        
        .cycle-slider {
            width: 200px;
        }
        
        .cycle-display {
            color: #4ec9b0;
            font-weight: bold;
            font-size: 14px;
            min-width: 100px;
        }
        
        .backpressure-info {
            background-color: #3e2222;
            border-left: 3px solid #f44747;
            padding: 10px;
            margin-top: 10px;
            border-radius: 3px;
        }
        
        .backpressure-reason {
            color: #f44747;
            font-size: 12px;
            margin-top: 5px;
        }
        
        .color-label {
            font-weight: bold;
            border-radius: 3px;
            display: inline-block;
            min-width: 20px;
            text-align: center;
        }
        
        .preload-info {
            background-color: #2a4a2a;
            border-left: 3px solid #66bb6a;
            color: #a8d5a8;
            padding: 4px;
            font-size: 11px;
            margin-top: 3px;
        }
        
        .recv-buffer {
            background-color: #2a3f4f;
            border-left: 3px solid #2196F3;
            padding: 4px;
            margin-top: 3px;
            font-size: 11px;
        }
        
        .recv-ready-true {
            color: #66bb6a;
            font-weight: bold;
        }
        
        .recv-ready-false {
            color: #f44747;
            font-weight: bold;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔍 CGRA Interactive Debugger - Kernel: <span id="kernelName" style="color: #4fc3f7;">KERNEL</span></h1>
        
        <div class="controls">
            <div class="control-group">
                <label>Cycle:</label>
                <input type="range" id="cycleSlider" class="cycle-slider" min="0" max="MAXCYCLE" value="0">
                <span class="cycle-display" id="cycleDisplay">Cycle 0</span>
                <span id="currentCycle">0</span>
            </div>
            <div class="control-group">
                <button onclick="previousCycle()">◀ Prev</button>
                <button onclick="nextCycle()">Next ▶</button>
                <button onclick="playAnimation()" style="background-color: #008000; padding: 8px 15px; font-weight: bold;">▶ Play</button>
                <button onclick="stopAnimation()" style="background-color: #cc6600; padding: 8px 15px; font-weight: bold;">⏸ Stop</button>
            </div>
            <div class="control-group">
                <label>Jump to cycle:</label>
                <input type="number" id="cycleInput" min="0" max="MAXCYCLE" value="0" style="width: 60px;">
                <button onclick="jumpToCycle()">Go</button>
            </div>
            <div class="control-group">
                <button onclick="resetSelection()">Clear Selection</button>
                <button onclick="openTimelineModal()" class="primary">📊 Timeline Overview</button>
                <button onclick="openExpectedScheduleModal()" class="primary">📋 Expected Schedule</button>
            </div>
        </div>
        
        <div class="content">
            <div class="grid-view">
                <div id="cycleGrid" class="cycle-grid"></div>
            </div>
            <div class="detail-view">
                <div id="detailPanel">
                    <div style="color: #858585; text-align: center; padding: 20px;">
                        Click on a PE to view details
                    </div>
                </div>
            </div>
        </div>
    </div>
    
    <!-- Timeline Modal -->
    <div id="timelineModal" class="modal">
        <div class="modal-content">
            <div class="modal-header">
                <div class="modal-title">📊 Execution Timeline - All PEs</div>
                <button class="close-btn" onclick="closeTimelineModal()">&times;</button>
            </div>
            <div style="font-size: 12px; color: #666; margin-bottom: 10px;">
                ✓ Each Column = One Cycle | ✓ Each Row in Column = One PE | ✓ Click any cell to jump to that cycle
            </div>
            
            <!-- Timeline grid will be populated by JavaScript -->
            <div id="timelineContainer">
                <!-- Will be populated by JavaScript -->
            </div>
            
            <div class="timeline-legend" style="margin-top: 10px;">
                <div class="legend-item">
                    <div class="legend-color" style="background-color: #f0f0f0; border: 1px solid #333; border-left: 4px solid #ccc;"></div>
                    <span>IDLE</span>
                </div>
                <div class="legend-item">
                    <div class="legend-color" style="background-color: #ccffcc; border: 1px solid #333; border-left: 4px solid #4caf50;"></div>
                    <span>EXEC (✓)</span>
                </div>
                <div class="legend-item">
                    <div class="legend-color" style="background-color: #ffcccc; border: 1px solid #333; border-left: 4px solid #f44336;"></div>
                    <span>BLOCKED (⚠️)</span>
                </div>
            </div>
            <div id="timelineInfo" class="timeline-info"></div>
        </div>
    </div>
    
    <!-- Expected Schedule Modal -->
    <div id="expectedScheduleModal" class="modal">
        <div class="modal-content">
            <div class="modal-header">
                <div class="modal-title">📋 Expected Instruction Schedule</div>
                <button class="close-btn" onclick="closeExpectedScheduleModal()">&times;</button>
            </div>
            <div style="font-size: 12px; color: #666; margin-bottom: 10px;">
                ✓ Expected instruction execution timeline extracted from YAML schedule | ✓ PE(X,Y): Instruction at Timestep
            </div>
            
            <!-- Expected schedule table will be populated by JavaScript -->
            <div id="expectedScheduleContainer" style="max-height: 70vh; overflow-y: auto;">
                <!-- Will be populated by JavaScript -->
            </div>
        </div>
    </div>
    
    <script>
        const eventsByTime = EVENTS_DATA;
        const coresList = CORES_LIST;
        const peStateData = PESTATE_DATA;
        const hasPEState = HAS_PESTATE;
        const expectedSchedule = EXPECTED_SCHEDULE_DATA;
        const maxCycle = MAXCYCLE;
        let currentCycle = 0;
        let selectedPE = null;
        let animationRunning = false;
        
        // Initialize kernel name
        document.getElementById('kernelName').textContent = 'KERNEL';
        
        // Initialize waveform from PEState
        function initWaveform() {
            if (!hasPEState) return;
            
            // Find parent container (main or body)
            const contentDiv = document.querySelector('.content');
            if (!contentDiv || !contentDiv.parentElement) return;
            
            // Check if waveform section already exists
            if (document.getElementById('waveformSection')) return;
            
            // Add waveform section AFTER content div (below grid)
            const waveSection = document.createElement('div');
            waveSection.id = 'waveformSection';
            waveSection.style.cssText = 'margin-top: 30px; margin-bottom: 30px; padding: 20px; background-color: #2d2d30; border: 1px solid #555; border-radius: 5px; width: calc(100% - 40px);';
            waveSection.innerHTML = `
                <h3 style="color: #4ec9b0; margin-bottom: 15px; font-size: 16px;">📊 Waveform View (PEState)</h3>
                <div style="display: flex; gap: 15px; margin-bottom: 15px; flex-wrap: wrap; align-items: center;">
                    <div>
                        <label style="color: #9cdcfe; font-weight: bold; font-size: 12px;">Select PE:</label>
                        <select id="peSelect" onchange="updateWaveform()" style="padding: 5px; background-color: #3e3e42; color: #d4d4d4; border: 1px solid #555; border-radius: 3px; font-size: 11px; margin-top: 3px;">
                            <option value="">All PEs</option>
                        </select>
                    </div>
                    <div>
                        <label style="color: #9cdcfe; font-weight: bold; font-size: 12px;">Signal:</label>
                        <select id="signalSelect" onchange="updateWaveform()" style="padding: 5px; background-color: #3e3e42; color: #d4d4d4; border: 1px solid #555; border-radius: 3px; font-size: 11px; margin-top: 3px;">
                            <option value="opcode">Opcode</option>
                            <option value="status">Status</option>
                            <option value="pc">Program Counter</option>
                            <option value="block_reason">Block Reason</option>
                            <option value="io_count">I/O Activity</option>
                        </select>
                    </div>
                    <div>
                        <label style="color: #9cdcfe; font-weight: bold; font-size: 12px;">Format:</label>
                        <select id="formatSelect" onchange="updateWaveform()" style="padding: 5px; background-color: #3e3e42; color: #d4d4d4; border: 1px solid #555; border-radius: 3px; font-size: 11px; margin-top: 3px;">
                            <option value="auto">Auto</option>
                            <option value="hex">Hex</option>
                            <option value="dec">Decimal</option>
                        </select>
                    </div>
                    <div style="margin-left: auto; display: flex; gap: 10px;">
                        <label style="color: #9cdcfe; font-weight: bold; font-size: 12px;">Zoom:</label>
                        <button onclick="zoomWaveform(-1)" style="padding: 5px 10px; background-color: #3e3e42; color: #d4d4d4; border: 1px solid #555; border-radius: 3px; cursor: pointer; font-size: 11px;">🔍−</button>
                        <button onclick="zoomWaveform(1)" style="padding: 5px 10px; background-color: #3e3e42; color: #d4d4d4; border: 1px solid #555; border-radius: 3px; cursor: pointer; font-size: 11px;">🔍+</button>
                        <button onclick="resetWaveformZoom()" style="padding: 5px 10px; background-color: #3e3e42; color: #d4d4d4; border: 1px solid #555; border-radius: 3px; cursor: pointer; font-size: 11px;">Reset</button>
                    </div>
                </div>
                <div id="waveChart" style="background-color: #1e1e1e; border: 1px solid #555; border-radius: 3px; padding: 15px; overflow-x: auto; overflow-y: auto; min-height: 250px; max-height: 500px;" onwheel="handleWaveformWheel(event)">
                    <svg id="waveSvg" style="display: none;"></svg>
                    <div id="waveMessage" style="color: #858585; text-align: center; padding: 50px; font-size: 12px;">Click Play to update waveform with cycle data...</div>
                </div>
            `;
            contentDiv.parentElement.insertBefore(waveSection, contentDiv.nextSibling);
            
            // Initialize PE select
            const peSelect = document.getElementById('peSelect');
            coresList.forEach(([x, y]) => {
                const option = document.createElement('option');
                option.value = `${x},${y}`;
                option.textContent = `PE (${x},${y})`;
                peSelect.appendChild(option);
            });
        }
        
        // Waveform zoom state
        let waveformZoom = 1.0;
        
        function zoomWaveform(direction) {
            // direction: 1 for zoom in, -1 for zoom out
            const zoomFactor = 1.2;
            if (direction > 0) {
                waveformZoom *= zoomFactor;
            } else {
                waveformZoom /= zoomFactor;
                if (waveformZoom < 0.5) waveformZoom = 0.5;
            }
            updateWaveform();
        }
        
        function resetWaveformZoom() {
            waveformZoom = 1.0;
            updateWaveform();
        }
        
        function handleWaveformWheel(event) {
            if (event.ctrlKey || event.metaKey) {
                event.preventDefault();
                const direction = event.deltaY > 0 ? -1 : 1;
                zoomWaveform(direction);
            }
        }
        
        function updateWaveform() {
            if (!hasPEState) return;
            
            const peSelect = document.getElementById('peSelect').value;
            const signal = document.getElementById('signalSelect').value;
            const format = document.getElementById('formatSelect').value;
            const waveSvg = document.getElementById('waveSvg');
            const waveMessage = document.getElementById('waveMessage');
            
            // Clear previous
            waveSvg.innerHTML = '';
            waveMessage.style.display = 'none';
            waveSvg.style.display = 'block';
            
            // Get PEs to display
            let pesDisplay = [];
            if (peSelect) {
                pesDisplay = [peSelect];
            } else {
                pesDisplay = coresList.map(([x, y]) => `${x},${y}`);
            }
            
            const signalHeight = 70;
            const chartMargin = { left: 100, right: 20, top: 20, bottom: 40 };
            let chartWidth = 1400 * waveformZoom;  // Apply zoom factor to width
            const chartHeight = pesDisplay.length * signalHeight + chartMargin.top + chartMargin.bottom;
            
            // Ensure minimum width for readability
            if (chartWidth < 600) chartWidth = 600;
            
            waveSvg.setAttribute('viewBox', `0 0 ${chartWidth} ${chartHeight}`);
            waveSvg.style.width = chartWidth + 'px';
            
            let yPos = chartMargin.top;
            
            pesDisplay.forEach((peKey) => {
                const [x, y] = peKey.split(',').map(Number);
                
                // PE label - positioned to the left of the waveform
                const label = document.createElementNS('http://www.w3.org/2000/svg', 'text');
                label.setAttribute('x', chartMargin.left - 10);
                label.setAttribute('y', yPos + signalHeight / 2 + 4);
                label.setAttribute('font-size', '12');
                label.setAttribute('fill', '#9cdcfe');
                label.setAttribute('font-weight', 'bold');
                label.setAttribute('text-anchor', 'end');
                label.textContent = `PE(${x},${y})`;
                waveSvg.appendChild(label);
                
                // Draw waveform for this PE
                const cycleWidth = (chartWidth - chartMargin.left - chartMargin.right) / (maxCycle + 1);
                const baseY = yPos + signalHeight / 2;
                
                for (let c = 0; c <= maxCycle; c++) {
                    const cycleState = peStateData[String(c)]?.[peKey];
                    if (!cycleState) continue;
                    
                    let value = '-';
                    let color = '#858585';
                    
                    if (signal === 'opcode') {
                        value = cycleState.opcode || '-';
                        color = '#4ec9b0';
                    } else if (signal === 'status') {
                        value = cycleState.status || 'Idle';
                        color = cycleState.status === 'Blocked' ? '#f44747' : '#4caf50';
                    } else if (signal === 'pc') {
                        value = cycleState.pc !== undefined ? cycleState.pc : '-';
                        color = '#9cdcfe';
                    } else if (signal === 'block_reason') {
                        if (cycleState.block_reason) {
                            value = cycleState.block_reason.code || 'BLOCKED';
                            color = '#f44747';
                        }
                    } else if (signal === 'io_count') {
                        const inputs = (cycleState.inputs || []).length;
                        const outputs = (cycleState.outputs || []).length;
                        value = inputs + outputs;
                        color = '#b5cea8';
                    }
                    
                    const x1 = chartMargin.left + c * cycleWidth;
                    const x2 = chartMargin.left + (c + 1) * cycleWidth;
                    
                    // Draw line
                    const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
                    line.setAttribute('x1', x1);
                    line.setAttribute('y1', baseY);
                    line.setAttribute('x2', x2);
                    line.setAttribute('y2', baseY);
                    line.setAttribute('stroke', color);
                    line.setAttribute('stroke-width', '2.5');
                    waveSvg.appendChild(line);
                    
                    // Value label
                    const text = document.createElementNS('http://www.w3.org/2000/svg', 'text');
                    text.setAttribute('x', (x1 + x2) / 2);
                    text.setAttribute('y', baseY - 10);
                    text.setAttribute('font-size', '10');
                    text.setAttribute('fill', color);
                    text.setAttribute('text-anchor', 'middle');
                    text.setAttribute('font-weight', 'bold');
                    text.textContent = String(value).substring(0, 12);
                    waveSvg.appendChild(text);
                    
                    // Clock edge
                    const edge = document.createElementNS('http://www.w3.org/2000/svg', 'line');
                    edge.setAttribute('x1', x2);
                    edge.setAttribute('y1', baseY - 8);
                    edge.setAttribute('x2', x2);
                    edge.setAttribute('y2', baseY + 8);
                    edge.setAttribute('stroke', '#555');
                    edge.setAttribute('stroke-width', '1');
                    waveSvg.appendChild(edge);
                }
                
                yPos += signalHeight;
            });
            
            // Draw cycle markers
            const cycleWidth = (chartWidth - chartMargin.left - chartMargin.right) / (maxCycle + 1);
            const labelY = yPos + 20;
            // Adjust interval based on zoom - show more labels when zoomed in
            let cycleInterval = Math.ceil((maxCycle + 1) / (15 / waveformZoom));
            if (cycleInterval < 1) cycleInterval = 1;
            
            for (let c = 0; c <= maxCycle; c += cycleInterval) {
                const x = chartMargin.left + c * cycleWidth;
                const text = document.createElementNS('http://www.w3.org/2000/svg', 'text');
                text.setAttribute('x', x);
                text.setAttribute('y', labelY);
                text.setAttribute('font-size', '10');
                text.setAttribute('fill', '#999');
                text.setAttribute('text-anchor', 'middle');
                text.setAttribute('font-weight', 'bold');
                text.textContent = `C${c}`;
                waveSvg.appendChild(text);
            }
        }
        
        function playAnimation() {
            animationRunning = true;
            const animate = () => {
                if (animationRunning && currentCycle < maxCycle) {
                    currentCycle++;
                    document.getElementById('currentCycle').textContent = currentCycle;
                    updateGrid();
                    updateWaveform();
                    setTimeout(animate, 150);
                }
            };
            animate();
        }
        
        function stopAnimation() {
            animationRunning = false;
        }
        
        function updateGrid() {
            const grid = document.getElementById('cycleGrid');
            grid.innerHTML = '';
            
            // Get cycle data based on format available
            let cycleEvents = [];
            let peStateInfo = {};
            
            if (hasPEState) {
                // Use PEState data and also try legacy events as fallback
                peStateInfo = peStateData[String(currentCycle)] || {};
                cycleEvents = eventsByTime[currentCycle] || [];
            } else {
                // Legacy format only
                cycleEvents = eventsByTime[currentCycle] || [];
            }
            
            // Build core state map
            const coreState = {};
            const dataFlows = [];  // Store all data flows for arrow rendering
            
            coresList.forEach(([x, y]) => {
                const peKey = `${x},${y}`;
                coreState[peKey] = {
                    status: 'IDLE',
                    opcode: '-',
                    sends: {},
                    receives: {},
                    backpressure: null,
                    hasActivity: false,
                    dataColor: null,
                    isPreload: false,
                    writeMemoryData: [],
                    bufferStatus: {},
                    backpressureEvents: [],
                    peStateInfo: peStateInfo[peKey] || null  // Store PEState if available
                };
            });
            
            // First, populate from PEState if available
            if (hasPEState) {
                Object.keys(peStateInfo).forEach(peKey => {
                    if (coreState[peKey]) {
                        const state = peStateInfo[peKey];
                        coreState[peKey].opcode = state.opcode || '-';
                        coreState[peKey].status = state.status === 'Blocked' ? 'BLOCKED' : 'IDLE';
                        
                        // Mark as having activity if running or blocked
                        if (state.status !== 'Idle') {
                            coreState[peKey].hasActivity = true;
                        }
                        
                        // Add input/output info
                        const inputs = state.inputs || [];
                        const outputs = state.outputs || [];
                        
                        inputs.forEach(inp => {
                            coreState[peKey].receives[inp.direction] = inp.data;
                            coreState[peKey].hasActivity = true;
                        });
                        
                        outputs.forEach(out => {
                            coreState[peKey].sends[out.direction] = out.data;
                            coreState[peKey].hasActivity = true;
                        });
                    }
                });
            }
            
            // Then process legacy events (they override/enhance PEState)
            cycleEvents.forEach(event => {
                const x = event.X;
                const y = event.Y;
                if (x === undefined || y === undefined) return;
                
                const key = `${x},${y}`;
                const msg = event.msg || '';
                
                if (msg === 'DataFlow') {
                    const direction = event.Direction || '?';
                    const data = event.Data;
                    const behavior = event.Behavior || '?';
                    const color = event.Color;
                    
                    coreState[key].hasActivity = true;
                    
                    if (color !== undefined && color !== null) {
                        coreState[key].dataColor = color;
                    }
                    
                    if (behavior === 'Send') {
                        coreState[key].sends[direction] = data;

                        if (event.From && event.To) {
                            const fromMatch = event.From.match(/Tile\[(\d+)\]\[(\d+)\]/);
                            if (fromMatch && parseInt(fromMatch[1]) === x && parseInt(fromMatch[2]) === y) {
                                dataFlows.push({
                                    from: event.From,
                                    to: event.To,
                                    type: 'send',
                                    x: x,
                                    y: y,
                                    direction: direction,
                                    data: data
                                });
                            }
                        }
                    } else {
                        coreState[key].receives[direction] = data;
                        if (event.From && event.To) {
                            const toMatch = event.To.match(/Tile\[(\d+)\]\[(\d+)\]/);
                            if (toMatch && parseInt(toMatch[1]) === x && parseInt(toMatch[2]) === y) {
                                dataFlows.push({
                                    from: event.From,
                                    to: event.To,
                                    type: 'recv',
                                    x: x,
                                    y: y,
                                    direction: direction,
                                    data: data
                                });
                            }
                        }
                    }
                } else if (msg === 'Backpressure') {
                    coreState[key].status = 'BLOCKED';
                    coreState[key].backpressure = event.Reason || 'Unknown';
                    if (event.OpCode) {
                        coreState[key].opcode = event.OpCode;
                    }
                    
                    if (event.RecvBufHeadReady !== undefined) {
                        coreState[key].bufferStatus.RecvBufHeadReady = event.RecvBufHeadReady;
                    }
                    
                    coreState[key].backpressureEvents.push({
                        type: event.Type,
                        reason: event.Reason,
                        direction: event.Direction,
                        recvBufHeadReady: event.RecvBufHeadReady,
                        color: event.Color,
                        colorIdx: event.ColorIdx
                    });
                } else if (msg === 'InstExec') {
                    coreState[key].hasActivity = true;
                    coreState[key].status = 'EXEC';
                    coreState[key].opcode = event.OpCode || '?';
                } else if (msg === 'Memory') {
                    coreState[key].hasActivity = true;
                    const behavior = event.Behavior || '?';
                    if (behavior === 'WriteMemory') {
                        coreState[key].isPreload = (currentCycle === 0);
                        coreState[key].writeMemoryData.push(event.Data);
                    }
                }
            });
            
            // Update status based on activity and backpressure
            Object.keys(coreState).forEach(key => {
                if (coreState[key].status === 'BLOCKED') {
                    // Keep BLOCKED status
                } else if (coreState[key].hasActivity) {
                    coreState[key].status = 'EXEC';
                }
            });
            
            // Create wrapper for grid and SVG overlay
            const wrapper = document.createElement('div');
            wrapper.className = 'grid-wrapper';
            
            // Create SVG overlay for arrows
            const svgOverlay = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
            svgOverlay.setAttribute('class', 'grid-svg-overlay');
            svgOverlay.setAttribute('viewBox', '0 0 1000 1000');
            svgOverlay.setAttribute('preserveAspectRatio', 'none');
            
            // Create grid container for PE boxes
            const gridContainer = document.createElement('div');
            gridContainer.className = 'grid-container';
            
            // Get grid dimensions (determine from cores). Ensure at least a 4x4 grid for clarity.
            let maxX = 0, maxY = 0;
            coresList.forEach(([x, y]) => {
                maxX = Math.max(maxX, x);
                maxY = Math.max(maxY, y);
            });

            const gridCols = Math.max(4, maxX + 1);
            const gridRows = Math.max(4, maxY + 1);

            // Prepare full grid coordinates (0,0 bottom-left) and render every PE (idle if no activity)
            const fullGrid = [];
            for (let gy = 0; gy < gridRows; gy++) {
                for (let gx = 0; gx < gridCols; gx++) {
                    fullGrid.push([gx, gy]);
                }
            }

            // Adjust grid container columns/rows to match
            gridContainer.style.gridTemplateColumns = `repeat(${gridCols}, 1fr)`;
            gridContainer.style.gridTemplateRows = `repeat(${gridRows}, 1fr)`;

            // Render all PEs and position them so (0,0) is bottom-left
            fullGrid.forEach(([x, y]) => {
                const key = `${x},${y}`;
                const state = coreState[key] || { status: 'IDLE', opcode: '-', sends: {}, receives: {}, backpressure: null };

                const pe = document.createElement('div');
                pe.className = 'pe-core';
                pe.setAttribute('data-x', x);
                pe.setAttribute('data-y', y);

                // Position using CSS grid: grid-column starts at 1; grid-row start inverted for bottom-left origin
                pe.style.gridColumnStart = (x + 1).toString();
                // CSS grid rows start at top(1) so invert: bottom row should be row = gridRows
                pe.style.gridRowStart = (gridRows - y).toString();

                if (selectedPE === key) {
                    pe.classList.add('selected');
                }
                if (state.status === 'BLOCKED') {
                    pe.classList.add('blocked');
                } else if (state.status === 'EXEC') {
                    pe.classList.add('active');
                }

                let statusClass = state.status === 'IDLE' ? 'status-idle' : 
                                 state.status === 'EXEC' ? 'status-exec' : 'status-blocked';

                let html = `<div class="pe-label">PE(${x},${y})</div>`;
                html += `<div class="pe-info">`;
                html += `<div class="info-line"><span class="status-label ${statusClass}">${state.status}</span></div>`;

                if (state.opcode !== '-') {
                    html += `<div class="info-line">OpCode: <span class="status-label">${state.opcode}</span></div>`;
                }

                // Show WriteMemory preload indicator (Cycle 0)
                if (state.isPreload && state.writeMemoryData.length > 0) {
                    html += `<div class="info-line preload-info">📦 WriteMemory</div>`;
                }

                // Show sends and receives with directional indicators
                Object.entries(state.sends).forEach(([dir, data]) => {
                    const dirArrow = {
                        'north': '⬆',
                        'south': '⬇',
                        'east': '⮕',
                        'west': '⬅'
                    }[dir.toLowerCase()] || '?';
                    
                    let dirLabel = dir;
                    if (state.dataColor !== null && state.dataColor !== undefined) {
                        const colorMap = {0: 'Red', 1: 'Green', 2: 'Blue', 3: 'Yellow', 4: 'Purple', 5: 'Cyan', 6: 'Orange'};
                        const colorName = colorMap[state.dataColor % 7] || `C${state.dataColor}`;
                        dirLabel = colorName;
                    }
                    html += `<div class="data-flow direction-${dir.toLowerCase()}" style="margin-left: 5px;">SEND ${dirArrow} ${dirLabel}</div>`;
                });

                Object.entries(state.receives).forEach(([dir, data]) => {
                    const dirArrow = {
                        'north': '⬆',
                        'south': '⬇',
                        'east': '⮕',
                        'west': '⬅'
                    }[dir.toLowerCase()] || '?';
                    
                    let dirLabel = dir;
                    if (state.dataColor !== null && state.dataColor !== undefined) {
                        const colorMap = {0: 'Red', 1: 'Green', 2: 'Blue', 3: 'Yellow', 4: 'Purple', 5: 'Cyan', 6: 'Orange'};
                        const colorName = colorMap[state.dataColor % 7] || `C${state.dataColor}`;
                        dirLabel = colorName;
                    }
                    html += `<div class="data-flow direction-${dir.toLowerCase()}" style="margin-left: 5px;">RECV ${dirArrow} ${dirLabel}</div>`;
                });

                if (state.backpressure) {
                    html += `<div class="backpressure-info" style="font-size: 10px; margin-top: 5px;">
                        ⚠️ BLOCKED<br/>${state.backpressure.substring(0, 40)}
                    </div>`;
                }

                html += `</div>`;
                pe.innerHTML = html;

                pe.onclick = (e) => {
                    e.stopPropagation();
                    selectedPE = key;
                    updateGrid();
                    updateDetail(x, y);
                };

                gridContainer.appendChild(pe);
            });

            // Draw arrows for data flows (delay to ensure layout calculations are valid)
            setTimeout(() => drawDataFlowArrows(svgOverlay, dataFlows, gridCols, gridRows), 0);
            
            wrapper.appendChild(svgOverlay);
            wrapper.appendChild(gridContainer);
            grid.appendChild(wrapper);
            
            document.getElementById('cycleDisplay').textContent = `Cycle ${currentCycle}`;
            document.getElementById('cycleSlider').value = currentCycle;
            document.getElementById('cycleInput').value = currentCycle;
        }
        
        function drawDataFlowArrows(svg, dataFlows, gridCols, gridRows) {
            if (!svg) return;
            
            // Define arrow markers for both Send (blue) and Recv (green)
            if (!svg.querySelector('#arrowhead-send')) {
                const defs = document.createElementNS('http://www.w3.org/2000/svg', 'defs');
                
                // Marker for Send arrows (blue)
                const markerSend = document.createElementNS('http://www.w3.org/2000/svg', 'marker');
                markerSend.setAttribute('id', 'arrowhead-send');
                markerSend.setAttribute('markerWidth', '14');
                markerSend.setAttribute('markerHeight', '14');
                markerSend.setAttribute('refX', '11');
                markerSend.setAttribute('refY', '7');
                markerSend.setAttribute('orient', 'auto');
                const polygonSend = document.createElementNS('http://www.w3.org/2000/svg', 'polygon');
                polygonSend.setAttribute('points', '0 0, 14 7, 0 14');
                polygonSend.setAttribute('fill', '#4fc3f7');  // Blue for Send
                markerSend.appendChild(polygonSend);
                defs.appendChild(markerSend);
                
                // Marker for Recv arrows (green)
                const markerRecv = document.createElementNS('http://www.w3.org/2000/svg', 'marker');
                markerRecv.setAttribute('id', 'arrowhead-recv');
                markerRecv.setAttribute('markerWidth', '14');
                markerRecv.setAttribute('markerHeight', '14');
                markerRecv.setAttribute('refX', '11');
                markerRecv.setAttribute('refY', '7');
                markerRecv.setAttribute('orient', 'auto');
                const polygonRecv = document.createElementNS('http://www.w3.org/2000/svg', 'polygon');
                polygonRecv.setAttribute('points', '0 0, 14 7, 0 14');
                polygonRecv.setAttribute('fill', '#66bb6a');  // Green for Recv
                markerRecv.appendChild(polygonRecv);
                defs.appendChild(markerRecv);
                
                svg.appendChild(defs);
            }

            // Defer drawing until layout is stable
            requestAnimationFrame(() => {
                // Clear previous arrows (keep defs)
                Array.from(svg.querySelectorAll('line, path, polyline, circle')).forEach(n => n.remove());

                const peElements = document.querySelectorAll('.pe-core[data-x][data-y]');
                if (!peElements || peElements.length === 0) return;
                const svgRect = svg.getBoundingClientRect();

                dataFlows.forEach(flow => {
                    try {
                        // Use direction instead of tile positions to determine arrow direction
                        const direction = (flow.direction || '').toLowerCase();
                        const isRecv = flow.type === 'recv';
                        const arrowColor = isRecv ? '#66bb6a' : '#4fc3f7';  // Green for Recv, Blue for Send
                        const markerUrl = isRecv ? 'url(#arrowhead-recv)' : 'url(#arrowhead-send)';
                        
                        // Get source PE element
                        const sourceX = flow.x;
                        const sourceY = flow.y;
                        let sourceElement = document.querySelector(`.pe-core[data-x='${sourceX}'][data-y='${sourceY}']`);
                        if (!sourceElement) return;
                        
                        const sourceRect = sourceElement.getBoundingClientRect();
                        const sourceCenterX = (sourceRect.left - svgRect.left + sourceRect.width / 2) / svgRect.width * 1000;
                        const sourceCenterY = (sourceRect.top - svgRect.top + sourceRect.height / 2) / svgRect.height * 1000;
                        
                        let targetX, targetY;
                        
                        // Calculate target position based on direction
                        if (direction === 'north') {
                            targetX = sourceCenterX;
                            targetY = sourceCenterY - 120;  // Arrow goes up
                        } else if (direction === 'south') {
                            targetX = sourceCenterX;
                            targetY = sourceCenterY + 120;  // Arrow goes down
                        } else if (direction === 'east') {
                            targetX = sourceCenterX + 120;  // Arrow goes right
                            targetY = sourceCenterY;
                        } else if (direction === 'west') {
                            targetX = sourceCenterX - 120;  // Arrow goes left
                            targetY = sourceCenterY;
                        } else {
                            // Fallback: use tile position logic (original behavior)
                            const fromMatch = flow.from.match(/Tile\[(\d+)\]\[(\d+)\]/);
                            const toMatch = flow.to.match(/Tile\[(\d+)\]\[(\d+)\]/);
                            if (!fromMatch || !toMatch) return;
                            const fromX = parseInt(fromMatch[1]);
                            const fromY = parseInt(fromMatch[2]);
                            const toX = parseInt(toMatch[1]);
                            const toY = parseInt(toMatch[2]);
                            
                            const toElement = document.querySelector(`.pe-core[data-x='${toX}'][data-y='${toY}']`);
                            if (!toElement) return;
                            
                            const toRect = toElement.getBoundingClientRect();
                            targetX = (toRect.left - svgRect.left + toRect.width / 2) / svgRect.width * 1000;
                            targetY = (toRect.top - svgRect.top + toRect.height / 2) / svgRect.height * 1000;
                        }
                        
                        // Draw arrow
                        const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
                        line.setAttribute('x1', sourceCenterX);
                        line.setAttribute('y1', sourceCenterY);
                        line.setAttribute('x2', targetX);
                        line.setAttribute('y2', targetY);
                        line.setAttribute('stroke', arrowColor);
                        line.setAttribute('stroke-width', 3);
                        line.setAttribute('marker-end', markerUrl);
                        line.setAttribute('opacity', '0.8');
                        line.setAttribute('class', 'data-flow-arrow');
                        svg.appendChild(line);
                    } catch (e) {
                        // ignore malformed
                    }
                });
            });
        }
        
        function updateDetail(x, y) {
            const key = `${x},${y}`;
            const cycleEvents = eventsByTime[currentCycle] || [];
            const peEvents = cycleEvents.filter(e => e.X === x && e.Y === y);
            
            let html = `<div class="detail-title">PE(${x},${y}) - Cycle ${currentCycle}</div>`;
            
            if (peEvents.length === 0) {
                html += `<div class="detail-item">No activity</div>`;
            } else {
                peEvents.forEach(event => {
                    const msg = event.msg || 'Unknown';
                    
                    if (msg === 'DataFlow') {
                        html += `<div class="detail-item">`;
                        html += `<div class="detail-key">Data Flow</div>`;
                        html += `<div><span class="detail-key">Behavior:</span><span class="detail-value">${event.Behavior}</span></div>`;
                        html += `<div><span class="detail-key">Direction:</span><span class="detail-value">${event.Direction}</span></div>`;
                        html += `<div><span class="detail-key">Data:</span><span class="detail-value">${event.Data}</span></div>`;
                        html += `<div><span class="detail-key">Predicate:</span><span class="detail-value">${event.Pred}</span></div>`;
                        html += `<div><span class="detail-key">Color:</span><span class="detail-value">${event.Color}</span></div>`;
                        html += `</div>`;
                    } else if (msg === 'Backpressure') {
                        html += `<div class="detail-item" style="border-left-color: #f44747;">`;
                        html += `<div class="detail-key" style="color: #f44747;">🔴 BACKPRESSURE</div>`;
                        html += `<div><span class="detail-key">Type:</span><span class="detail-value">${event.Type}</span></div>`;
                        html += `<div><span class="detail-key">OpCode:</span><span class="detail-value">${event.OpCode}</span></div>`;
                        html += `<div><span class="detail-key">Direction:</span><span class="detail-value">${event.Direction}</span></div>`;
                        html += `<div><span class="detail-key">Color:</span><span class="detail-value">${event.Color || '-'}</span></div>`;
                        html += `<div><span class="detail-key">Reason:</span><span class="detail-value">${event.Reason}</span></div>`;
                        
                        // Show buffer status
                        if (event.RecvBufHeadReady !== undefined) {
                            const readyClass = event.RecvBufHeadReady ? 'recv-ready-true' : 'recv-ready-false';
                            html += `<div class="recv-buffer">`;
                            html += `<span class="detail-key">📊 RecvBufHeadReady:</span>`;
                            html += `<span class="${readyClass}">${event.RecvBufHeadReady ? '✓ TRUE' : '✗ FALSE'}</span>`;
                            html += `</div>`;
                        }
                        html += `</div>`;
                    } else if (msg === 'InstExec') {
                        html += `<div class="detail-item" style="border-left-color: #4ec9b0;">`;
                        html += `<div class="detail-key">Instruction</div>`;
                        html += `<div><span class="detail-key">OpCode:</span><span class="detail-value">${event.OpCode}</span></div>`;
                        html += `</div>`;
                    }
                });
            }
            
            // Show all neighboring PEs
            html += `<div class="detail-title" style="margin-top: 20px;">Neighbors</div>`;
            const neighbors = {
                'North': [x, y-1],
                'South': [x, y+1],
                'East': [x+1, y],
                'West': [x-1, y]
            };
            
            Object.entries(neighbors).forEach(([dir, [nx, ny]]) => {
                html += `<div class="detail-item" style="font-size: 11px;">`;
                html += `<span class="detail-key">${dir}:</span> PE(${nx},${ny})`;
                html += `</div>`;
            });
            
            document.getElementById('detailPanel').innerHTML = html;
        }
        
        function previousCycle() {
            if (currentCycle > 0) {
                currentCycle--;
                updateGrid();
                if (selectedPE) {
                    const [x, y] = selectedPE.split(',').map(Number);
                    updateDetail(x, y);
                }
            }
        }
        
        function nextCycle() {
            if (currentCycle < maxCycle) {
                currentCycle++;
                updateGrid();
                if (selectedPE) {
                    const [x, y] = selectedPE.split(',').map(Number);
                    updateDetail(x, y);
                }
            }
        }
        
        function jumpToCycle() {
            const input = document.getElementById('cycleInput').value;
            const cycle = parseInt(input);
            if (!isNaN(cycle) && cycle >= 0 && cycle <= maxCycle) {
                currentCycle = cycle;
                updateGrid();
                if (selectedPE) {
                    const [x, y] = selectedPE.split(',').map(Number);
                    updateDetail(x, y);
                }
            }
        }
        
        function resetSelection() {
            selectedPE = null;
            document.getElementById('detailPanel').innerHTML = 
                '<div style="color: #858585; text-align: center; padding: 20px;">Click on a PE to view details</div>';
            updateGrid();
        }
        
        document.getElementById('cycleSlider').oninput = (e) => {
            currentCycle = parseInt(e.target.value);
            updateGrid();
            if (selectedPE) {
                const [x, y] = selectedPE.split(',').map(Number);
                updateDetail(x, y);
            }
        };
        
        // Timeline modal functions
        function openTimelineModal() {
            document.getElementById('timelineModal').classList.add('show');
            renderTimeline();
        }
        
        function closeTimelineModal() {
            document.getElementById('timelineModal').classList.remove('show');
        }
        
        function renderTimeline() {
            const timelineContainer = document.getElementById('timelineContainer');
            timelineContainer.innerHTML = '';
            
            // Create timeline wrapper
            const timeline = document.createElement('div');
            timeline.style.display = 'flex';
            timeline.style.overflowX = 'auto';
            timeline.style.border = '2px solid #333';
            timeline.style.backgroundColor = 'white';
            timeline.style.borderRadius = '5px';
            
            // Build each cycle column
            for (let cycle = 0; cycle <= maxCycle; cycle++) {
                const column = document.createElement('div');
                column.style.display = 'flex';
                column.style.flexDirection = 'column';
                column.style.borderRight = '1px solid #ccc';
                column.style.minWidth = '180px';
                
                // Cycle header
                const header = document.createElement('div');
                header.style.backgroundColor = '#2196F3';
                header.style.color = 'white';
                header.style.padding = '10px';
                header.style.fontWeight = 'bold';
                header.style.textAlign = 'center';
                header.style.fontSize = '14px';
                header.style.borderBottom = '1px solid #999';
                header.textContent = `Cycle ${cycle}`;
                column.appendChild(header);
                
                // Add PE blocks for this cycle
                coresList.forEach(([x, y]) => {
                    const cycleEvents = eventsByTime[cycle] || [];
                    let status = 'idle';
                    let sends = [];
                    let receives = [];
                    let reason = '';
                    let hasActivity = false;
                    let hasBackpressure = false;
                    
                    // Process events
                    cycleEvents.forEach(event => {
                        if (event.X === x && event.Y === y) {
                            hasActivity = true;
                            const msg = event.msg || '';
                            
                            if (msg === 'DataFlow') {
                                const dir = event.Direction || '?';
                                const data = event.Data || '?';
                                const behavior = event.Behavior || '';
                                
                                if (behavior === 'Send') {
                                    sends.push(`${dir}:${data}`);
                                } else {
                                    receives.push(`${dir}:${data}`);
                                }
                            } else if (msg === 'Backpressure') {
                                hasBackpressure = true;
                                reason = event.Reason || 'Unknown';
                            } else if (msg === 'InstExec' || msg === 'Memory') {
                                // Any execution activity
                            }
                        }
                    });
                    
                    // Determine status: Backpressure = RED, Activity but no backpressure = GREEN, No activity = GRAY
                    if (hasBackpressure) {
                        status = 'blocked';
                    } else if (hasActivity) {
                        status = 'active';
                    }
                    
                    // Create PE block
                    const block = document.createElement('div');
                    block.style.padding = '8px';
                    block.style.borderBottom = '1px solid #eee';
                    block.style.fontSize = '11px';
                    block.style.minHeight = '60px';
                    block.style.cursor = 'pointer';
                    block.style.transition = 'all 0.15s';
                    block.style.overflow = 'hidden';
                    
                    // Set style based on status - GREEN for ACTIVE (any activity without backpressure)
                    if (status === 'blocked') {
                        block.style.backgroundColor = '#ffcccc';
                        block.style.borderLeft = '4px solid #f44336';
                    } else if (status === 'active') {
                        block.style.backgroundColor = '#ccffcc';
                        block.style.borderLeft = '4px solid #4caf50';
                    } else {
                        block.style.backgroundColor = '#f0f0f0';
                        block.style.borderLeft = '4px solid #ccc';
                    }
                    
                    // Add content
                    let html = `<div style="font-weight: bold; color: #333;">PE(${x},${y})</div>`;
                    
                    if (sends.length > 0) {
                        html += `<div style="color: #2196F3; font-size: 10px;">Send: ${sends.join(', ')}</div>`;
                    }
                    if (receives.length > 0) {
                        html += `<div style="color: #ff9800; font-size: 10px;">Recv: ${receives.join(', ')}</div>`;
                    }
                    if (status === 'blocked') {
                        html += `<div style="color: #f44336; font-size: 10px; font-weight: bold;">⚠️ BLOCKED</div>`;
                        if (reason) {
                            html += `<div style="color: #f44336; font-size: 9px;">${reason.substring(0, 30)}</div>`;
                        }
                    } else if (status === 'active') {
                        html += `<div style="color: #4caf50; font-size: 10px; font-weight: bold;">✓ RUNNING</div>`;
                    }
                    
                    block.innerHTML = html;
                    
                    // Hover effect
                    block.onmouseover = () => {
                        block.style.boxShadow = '0 2px 8px rgba(0,0,0,0.2)';
                        block.style.zIndex = '10';
                    };
                    
                    block.onmouseout = () => {
                        block.style.boxShadow = 'none';
                    };
                    
                    // Click to jump to cycle
                    block.onclick = (e) => {
                        e.stopPropagation();
                        currentCycle = cycle;
                        closeTimelineModal();
                        updateGrid();
                        document.getElementById('cycleSlider').value = cycle;
                        document.getElementById('cycleDisplay').textContent = `Cycle ${cycle}`;
                    };
                    
                    block.title = `Click to go to Cycle ${cycle}, PE(${x},${y})`;
                    
                    column.appendChild(block);
                });
                
                timeline.appendChild(column);
            }
            
            timelineContainer.appendChild(timeline);
            
            // Calculate and display stats
            let totalActive = 0;
            let totalBlocked = 0;
            
            for (let cycle = 0; cycle <= maxCycle; cycle++) {
                coresList.forEach(([x, y]) => {
                    const cycleEvents = eventsByTime[cycle] || [];
                    let isActive = false;
                    let isBlocked = false;
                    
                    cycleEvents.forEach(event => {
                        if (event.X === x && event.Y === y) {
                            if (event.msg === 'InstExec') isActive = true;
                            if (event.msg === 'Backpressure') isBlocked = true;
                        }
                    });
                    
                    if (isActive) totalActive++;
                    if (isBlocked) totalBlocked++;
                });
            }
            
            // Calculate PE and Link utilization similar to analyze_all.py
            let peUtilizationList = [];
            let linkUtilizationList = [];
            
            // Calculate PE utilization: active cycles per PE / total cycles
            for (let i = 0; i < coresList.length; i++) {
                const [x, y] = coresList[i];
                const peKey = `${x},${y}`;
                let activeCyclesSet = new Set();
                
                // Collect all cycles where this PE has activity
                for (let cycle = 0; cycle <= maxCycle; cycle++) {
                    const cycleEvents = eventsByTime[cycle] || [];
                    for (const event of cycleEvents) {
                        if (event.X === x && event.Y === y) {
                            const msg = event.msg || '';
                            // Count activity events (same as analyze_all.py)
                            if (msg === 'DataFlow' && ['Send', 'Recv'].includes(event.Behavior)) {
                                activeCyclesSet.add(cycle);
                            } else if (msg === 'Memory' && ['WriteMemory', 'ReadMemory'].includes(event.Behavior)) {
                                activeCyclesSet.add(cycle);
                            } else if (msg === 'InstExec') {
                                activeCyclesSet.add(cycle);
                            }
                        }
                    }
                }
                
                const activeCycles = activeCyclesSet.size;
                const peUtil = ((maxCycle + 1) > 0) ? (activeCycles / (maxCycle + 1) * 100) : 0;
                peUtilizationList.push({
                    pe: peKey,
                    utilization: peUtil.toFixed(1),
                    activeCycles: activeCycles,
                    totalCycles: maxCycle + 1
                });
            }
            
            // Calculate Link utilization: data flow events per link / total cycles
            let linkMap = new Map();
            for (let cycle = 0; cycle <= maxCycle; cycle++) {
                const cycleEvents = eventsByTime[cycle] || [];
                for (const event of cycleEvents) {
                    if (event.msg === 'DataFlow') {
                        const x = event.X, y = event.Y;
                        const direction = event.Direction || '?';
                        const behavior = event.Behavior || '';
                        
                        let linkKey;
                        if (behavior === 'Send') {
                            linkKey = `PE(${x},${y})-${direction}`;
                        } else if (behavior === 'Recv') {
                            linkKey = `${direction}->PE(${x},${y})`;
                        }
                        
                        if (linkKey) {
                            if (!linkMap.has(linkKey)) {
                                linkMap.set(linkKey, { count: 0, cycles: new Set() });
                            }
                            const linkData = linkMap.get(linkKey);
                            linkData.count++;
                            linkData.cycles.add(cycle);
                        }
                    }
                }
            }
            
            // Convert link map to list and calculate utilization
            for (const [linkKey, linkData] of linkMap.entries()) {
                const linkUtil = ((maxCycle + 1) > 0) ? (linkData.cycles.size / (maxCycle + 1) * 100) : 0;
                linkUtilizationList.push({
                    link: linkKey,
                    utilization: linkUtil.toFixed(1),
                    activeEvents: linkData.count,
                    activeCycles: linkData.cycles.size,
                    totalCycles: maxCycle + 1
                });
            }
            
            // Calculate average utilization
            const avgPEUtil = peUtilizationList.length > 0 
                ? (peUtilizationList.reduce((sum, p) => sum + parseFloat(p.utilization), 0) / peUtilizationList.length).toFixed(1)
                : 0;
            const avgLinkUtil = linkUtilizationList.length > 0 
                ? (linkUtilizationList.reduce((sum, l) => sum + parseFloat(l.utilization), 0) / linkUtilizationList.length).toFixed(1)
                : 0;
            
            // Build info table
            let infoHTML = `
                <div style="color: #333; font-size: 12px;">
                    <div><span class="detail-key">Kernel:</span> <span style="color: #4fc3f7;">${document.getElementById('kernelName').textContent}</span></div>
                    <div><span class="detail-key">Total Cycles:</span> <span>${maxCycle + 1}</span></div>
                    <div><span class="detail-key">Total PEs:</span> <span>${coresList.length}</span></div>
                    <div><span class="detail-key">Total Links:</span> <span>${linkUtilizationList.length}</span></div>
                    <hr style="border: 1px solid #ddd; margin: 5px 0;">
                    <div style="color: #4caf50; font-weight: bold;">PE Utilization: ${avgPEUtil}%</div>
                    <div style="color: #2196F3; font-weight: bold;">Link Utilization: ${avgLinkUtil}%</div>
                    <hr style="border: 1px solid #ddd; margin: 5px 0;">
                    <div><span class="detail-key">Total States:</span> <span style="color: #4caf50;">${totalActive}</span> ACTIVE, <span style="color: #f44336;">${totalBlocked}</span> BLOCKED</div>
                </div>
            `;
            
            document.getElementById('timelineInfo').innerHTML = infoHTML;
        }
        
        // Expected Schedule modal functions
        function openExpectedScheduleModal() {
            if (Object.keys(expectedSchedule).length === 0) {
                alert('No expected schedule available. YAML was not found or could not be parsed.');
                return;
            }
            document.getElementById('expectedScheduleModal').classList.add('show');
            renderExpectedSchedule();
        }
        
        function closeExpectedScheduleModal() {
            document.getElementById('expectedScheduleModal').classList.remove('show');
        }
        
        function renderExpectedSchedule() {
            const container = document.getElementById('expectedScheduleContainer');
            container.innerHTML = '';
            
            // Build a table showing expected instructions by timestep
            // Format: Timestep | PE(X,Y) | Instruction
            
            // First, collect all timesteps across all PEs
            const allTimesteps = new Set();
            const peInstructions = {};  // {peKey: {timestep: opcode}}
            
            for (const [peKey, schedule] of Object.entries(expectedSchedule)) {
                peInstructions[peKey] = {};
                for (const [timestep, opcode] of Object.entries(schedule)) {
                    allTimesteps.add(parseInt(timestep));
                    peInstructions[peKey][timestep] = opcode;
                }
            }
            
            const sortedTimesteps = Array.from(allTimesteps).sort((a, b) => a - b);
            
            // Create a table
            let html = '<table style="width: 100%; border-collapse: collapse; font-size: 12px;">';
            html += '<tr style="background-color: #333; position: sticky; top: 0;">';
            html += '<th style="border: 1px solid #666; padding: 8px; text-align: left; color: #4ec9b0;">Timestep</th>';
            html += '<th style="border: 1px solid #666; padding: 8px; text-align: left; color: #4ec9b0;">PE(X,Y)</th>';
            html += '<th style="border: 1px solid #666; padding: 8px; text-align: left; color: #4ec9b0;">Instruction</th>';
            html += '</tr>';
            
            // For each timestep, list all PEs that have instructions
            for (const timestep of sortedTimesteps) {
                const pesAtThisTimestep = [];
                
                for (const [peKey, schedule] of Object.entries(peInstructions)) {
                    if (schedule[timestep]) {
                        pesAtThisTimestep.push({peKey, opcode: schedule[timestep]});
                    }
                }
                
                if (pesAtThisTimestep.length > 0) {
                    for (let i = 0; i < pesAtThisTimestep.length; i++) {
                        const {peKey, opcode} = pesAtThisTimestep[i];
                        const bgColor = i === 0 ? '#2a2a2a' : '#1e1e1e';
                        html += `<tr style="background-color: ${bgColor}; cursor: pointer;" onclick="jumpToCycle(${timestep})">`;
                        
                        if (i === 0) {
                            html += `<td style="border: 1px solid #444; padding: 8px; color: #ffeb3b; font-weight: bold; vertical-align: top;" rowspan="${pesAtThisTimestep.length}">T${timestep}</td>`;
                        }
                        
                        html += `<td style="border: 1px solid #444; padding: 8px; color: #569cd6;">${peKey}</td>`;
                        html += `<td style="border: 1px solid #444; padding: 8px; color: #ce9178; font-weight: bold;">${opcode}</td>`;
                        html += `</tr>`;
                    }
                }
            }
            
            html += '</table>';
            
            // Add summary
            let summary = `<div style="margin-top: 15px; padding: 10px; background-color: #252526; border-radius: 5px; font-size: 12px;">`;
            summary += `<div><span class="detail-key">Total Expected Instructions:</span> ${Object.values(peInstructions).reduce((sum, schedule) => sum + Object.keys(schedule).length, 0)}</div>`;
            summary += `<div><span class="detail-key">Number of Timesteps:</span> ${sortedTimesteps.length}</div>`;
            summary += `<div><span class="detail-key">PEs with Instructions:</span> ${Object.keys(peInstructions).length}</div>`;
            summary += `</div>`;
            
            container.innerHTML = html + summary;
        }
        
        // Close modal when clicking outside
        window.onclick = (event) => {
            const timelineModal = document.getElementById('timelineModal');
            const expectedModal = document.getElementById('expectedScheduleModal');
            
            if (event.target === timelineModal) {
                closeTimelineModal();
            }
            if (event.target === expectedModal) {
                closeExpectedScheduleModal();
            }
        };
        
        // Initial render
        updateGrid();
        initWaveform();
        if (hasPEState) {
            updateWaveform();
        }
    </script>
</body>
</html>
"""
    
    # Build events data
    events_json = json.dumps(dict(events_by_cycle))
    cores_json = json.dumps(list(cores))
    
    # Build PEState data - convert to JSON-serializable format
    pestate_json_dict = {}
    for cycle in pestate_by_cycle:
        pestate_json_dict[str(cycle)] = {
            f"{x},{y}": {
                'opcode': state.get('opcode', '-'),
                'status': state.get('status', 'Idle'),
                'pc': state.get('pc', -1),
                'block_reason': state.get('block_reason'),
                'inputs': state.get('inputs', []),
                'outputs': state.get('outputs', []),
                'memory_ops': state.get('memory_ops', []),
            }
            for (x, y), state in pestate_by_cycle[cycle].items()
        }
    pestate_json = json.dumps(pestate_json_dict)
    
    # Convert expected_schedule keys from tuples to strings for JSON
    expected_schedule_json_dict = {f"{x},{y}": schedule for (x, y), schedule in expected_schedule.items()}
    expected_schedule_json = json.dumps(expected_schedule_json_dict)
    
    html = html.replace('EVENTS_DATA', events_json)
    html = html.replace('CORES_LIST', cores_json)
    html = html.replace('PESTATE_DATA', pestate_json)
    html = html.replace('HAS_PESTATE', 'true' if has_pestate else 'false')
    html = html.replace('EXPECTED_SCHEDULE_DATA', expected_schedule_json)
    html = html.replace('MAXCYCLE', str(int(max_cycle)))
    html = html.replace('KERNEL', kernel_name)
    
    with open(output_file, 'w') as f:
        f.write(html)
    
    print(f"[+] Generated {output_file}")
    print(f"    Max cycle: {max_cycle}")
    print(f"    Total cores: {len(cores)}")
    print(f"    Kernel: {kernel_name}")

if __name__ == '__main__':
    if len(sys.argv) < 2:
        print("Usage: python3 generate_interactive_html.py <log_file> [output.html]")
        sys.exit(1)
    
    log_file = sys.argv[1]
    output = sys.argv[2] if len(sys.argv) > 2 else "cgra_debug_interactive.html"
    
    generate_interactive_html(log_file, output)
