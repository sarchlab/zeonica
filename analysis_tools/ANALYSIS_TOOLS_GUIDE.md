# CGRA Analysis Tools - Comprehensive Guide

This document provides a complete guide to the CGRA kernel debugging and analysis tools. These tools enable you to analyze execution logs, visualize data flow, diagnose performance issues, and understand kernel behavior at the cycle-by-cycle level.

## Table of Contents

1. [Quick Start (3 Steps)](#quick-start)
2. [Tools Overview](#tools-overview)
3. [Detailed Tool Usage](#detailed-tool-usage)
4. [Interactive HTML Debugger](#interactive-html-debugger)
5. [Common Debugging Workflows](#debugging-workflows)
6. [FAQ & Troubleshooting](#faq)
7. [Tips & Best Practices](#tips)

---

## Quick Start

### Step 1: Run a Test and Generate Log

```bash
cd /workspaces/zeonica/test/axpy
go build && ./axpy
# This generates: axpy_run.log
```

### Step 2: Generate All Reports (One Command)

```bash
cd /workspaces/zeonica
./quick_debug.sh test/axpy/axpy_run.log /tmp
```

This creates 6 files:
- `core_activity.csv` - PE state matrix (open in Excel)
- `dataflow_trace.csv` - All data flow events
- `backpressure_analysis.csv` - All blocking events
- `debug_report.txt` - Human-readable text summary
- `backpressure_diagnosis.txt` - Detailed blocking reasons
- `cgra_timeline.html` - Interactive visualization

### Step 3: View Reports

```bash
# Text summary (quick overview)
cat /tmp/debug_report.txt | head -50

# Open in Excel for detailed analysis
# /tmp/core_activity.csv

# Interactive visualization
# Open /tmp/cgra_timeline.html in browser
```

---

## Tools Overview

### 1. export_cgra_csv.py
**Purpose**: Convert JSON logs to CSV tables for Excel analysis

**Generates**:
- `core_activity.csv` - State matrix (rows=cycles, columns=PE attributes)
- `dataflow_trace.csv` - All data flow events with from/to/direction/data
- `backpressure_analysis.csv` - All blocking events with reasons

**Usage**:
```bash
python3 export_cgra_csv.py <log_file> <output_dir>
```

**Best for**: Finding patterns, analyzing trends, filtering by PE or cycle

---

### 2. generate_debug_report.py
**Purpose**: Create human-readable text reports

**Output**:
- PE Timeline: Complete timeline for each PE showing opcodes, sends, receives, blocking
- Data Flow Sequence: All data movements by cycle
- Backpressure Summary: Statistics on blocking events

**Usage**:
```bash
python3 generate_debug_report.py <core_activity.csv>
```

**Example output**:
```
PE(2,2) Timeline
================================================
Cycle    Status     Opcode    →Dir   →Data    ←Dir   ←Data    BackP
0        IDLE       -         -      -        -      -        -
3        BLOCKED    MOV       East   0x5      -      -        YES
4        IDLE       -         -      -        -      -        -
```

---

### 3. diagnose_backpressure.py
**Purpose**: Find exact causes of backpressure (blocking)

**Key Information**:
- Which PE was blocked
- What instruction tried to execute
- Which direction (North/South/East/West)
- Specific reason (e.g., `RecvBufHeadReady=False`)

**Usage**:
```bash
python3 diagnose_backpressure.py <log_file>
```

**Example output**:
```
Cycle 3:
  [CheckFlagsFailed] MOV to East: RecvBufHeadReady=False
  → PE(3,2)'s receive buffer is full
```

---

### 4. generate_interactive_html.py
**Purpose**: Create interactive HTML debugger with cycle-by-cycle visualization

**Features**:
- Navigate through cycles with slider, buttons, or direct input
- View PE grid with status colors (green=executing, red=blocked, gray=idle)
- See data flow arrows between PEs
- Click PE to view detailed information
- Timeline overview showing all PEs across all cycles
- Expected instruction schedule from YAML
- Waveform visualization of PE states over time

**Usage**:
```bash
python3 generate_interactive_html.py <log_file> [output.html]
```

**Example**:
```bash
python3 generate_interactive_html.py test/axpy/axpy_run.log test/axpy/cgra_debug.html
# Open cgra_debug.html in browser
```

---

### 5. debug_console.py
**Purpose**: Interactive console for cycle-by-cycle queries

**Commands**:
```
cycle <N>           - Show all activity in cycle N
core <X> <Y> <C1> <C2> - Show PE(X,Y) from cycle C1 to C2
flow all            - Show all data flows
flow <X> <Y>        - Show flows involving PE(X,Y)
blocked <X> <Y>     - Show blocking events for PE(X,Y)
```

**Usage**:
```bash
python3 debug_console.py <log_file>
cgra> cycle 5
cgra> core 2 2 0 10
cgra> flow all
```

---

### 6. quick_debug.sh
**Purpose**: One-command script to generate all 6 analysis files

**Usage**:
```bash
./quick_debug.sh <log_file> <output_dir>
```

**Generates**:
1. core_activity.csv
2. dataflow_trace.csv
3. backpressure_analysis.csv
4. debug_report.txt
5. backpressure_diagnosis.txt
6. cgra_timeline.html

---

## Detailed Tool Usage

### CSV Analysis in Excel

After running `export_cgra_csv.py`, open `core_activity.csv` in Excel:

**Key Columns**:
- `Cycle` - Cycle number (0 to max)
- `PE(X,Y)-Status` - IDLE/EXEC/BLOCKED
- `PE(X,Y)-Opcode` - Instruction being executed
- `PE(X,Y)-SendDir` - Direction of data being sent (north/south/east/west)
- `PE(X,Y)-SendData` - Data value being sent
- `PE(X,Y)-RecvDir` - Direction of data being received
- `PE(X,Y)-RecvData` - Data value being received
- `PE(X,Y)-BackP` - Is PE currently blocked?

**Excel Tips**:
1. Sort by `PE(X,Y)-Status` to find blocked cycles
2. Filter by `PE(X,Y)-BackP = YES` to see blocking events
3. Use Ctrl+F to search for specific data values
4. Create pivot tables to analyze PE utilization
5. Compare multiple runs side-by-side

---

### Understanding PE States

| State | Color | Meaning |
|-------|-------|---------|
| **IDLE** | Gray | No activity this cycle |
| **EXEC** | Green | PE is executing (has events but not blocked) |
| **BLOCKED** | Red | PE cannot proceed due to backpressure |

**State Transitions**:
```
IDLE (no events)
  ↓
EXEC (has DataFlow/Memory/Instruction events)
  ↓
BLOCKED (has Backpressure event - highest priority)
```

---

### Understanding Data Flow

**Send** (PE sending data):
- Direction indicates where data goes (N/S/E/W)
- Blue arrow in visualization
- Shown as `→ Direction: Data` in detail panel

**Receive** (PE receiving data):
- Direction indicates where data comes from (N/S/E/W)
- Green arrow in visualization
- Shown as `← Direction: Data` in detail panel

**Flow Example**:
```
Cycle 5:
  PE(2,2) → SEND East   Data=0x42
  PE(3,2) ← RECV West   Data=0x42
```
The data flows from PE(2,2) to PE(3,2) through the East/West ports.

---

### Understanding Backpressure

**What it means**: A PE tried to send data but couldn't (buffer full)

**Common Reasons**:
- `RecvBufHeadReady=False` - Receiver's buffer is full
- `SendBufHeadBusy=True` - Sender's buffer is busy
- `old data not consumed` - Receiver hasn't consumed previous data

**How to diagnose**:
1. Find the blocked PE in CSV (Status = BLOCKED)
2. Check SendDir and SendData columns - what was it trying to send?
3. Find the receiving PE in the same direction
4. Check if receiver's status is also BLOCKED
5. Use `diagnose_backpressure.py` for detailed reason

---

## Interactive HTML Debugger

### Features

#### 1. Cycle Navigation
- **Slider**: Drag to browse cycles
- **Prev/Next Buttons**: Move one cycle at a time
- **Direct Input**: Type cycle number and click "Go"
- **Current Cycle Display**: Shows cycle number in top-right

#### 2. PE Grid Visualization
- **Colors**: Green (executing), Red (blocked), Gray (idle)
- **Information per PE**:
  - Coordinates PE(x,y)
  - Current status
  - Opcode (instruction being executed)
  - Data send/receive information
  - Blocking reason (if blocked)

#### 3. Data Flow Arrows
- **Blue arrows** (→): Data being sent
- **Green arrows** (←): Data being received
- **Direction**: Arrow points to receiving PE
- **Animation**: Pulsing animation for visibility

#### 4. Detail Panel
Click any PE to see detailed information:
- **DataFlow Events**: All sends and receives
- **Backpressure Events**: Why PE was blocked
- **Buffer Status**: RecvBufHeadReady state
- **Neighbor Info**: Coordinates of adjacent PEs

#### 5. Timeline Overview
Click "📊 Timeline Overview" button:
- **Grid view**: Each row = PE, each column = cycle
- **Colors**: Green (executing), Red (blocked), Gray (idle)
- **Statistics**:
  - Total cycles
  - Total PEs
  - PE utilization (%)
  - Link utilization (%)
- **Click to jump**: Click any cell to jump to that cycle

#### 6. Expected Schedule
Click "📋 Expected Schedule" button:
- **View**: Instructions that should execute at each timestep
- **Table**: Timestep | PE(X,Y) | Instruction
- **Interactive**: Click row to jump to that cycle
- **Compare**: Check actual vs expected execution

#### 7. Waveform View (Optional)
If PEState data available:
- **Signal Display**: Show opcode, status, or PC over time
- **PE Selection**: View single PE or all PEs
- **Formats**: Auto/Hex/Decimal
- **Zoom Controls**: Zoom in/out with buttons or Ctrl+Scroll
- **Play Animation**: Watch waveform update as simulation progresses

---

## Debugging Workflows

### Workflow 1: "Why is my result wrong?"

1. **Run test and generate reports**
   ```bash
   ./quick_debug.sh test/axpy/axpy_run.log /tmp
   ```

2. **Check result size in CSV**
   ```bash
   # Open /tmp/core_activity.csv in Excel
   # Look at PE(3,2) loop counter column
   # Should see: 0, 1, 2, 3, ... (incrementing)
   ```

3. **If counter is stuck at 0**:
   - Check if PE(2,2) sends loop-back data to PE(3,2)
   - Check if data appears in PE(3,2) receive column
   - Check if backpressure is blocking PE(2,2)

4. **If data not flowing**:
   ```bash
   python3 diagnose_backpressure.py test/axpy/axpy_run.log
   ```
   - Look for PE(2,2) being blocked in many cycles
   - See what reason is given

---

### Workflow 2: "Why is PE X frequently blocked?"

1. **Generate backpressure diagnosis**
   ```bash
   python3 diagnose_backpressure.py test/axpy/axpy_run.log | grep "PE(X,Y)"
   ```

2. **Understand the reason**:
   - `RecvBufHeadReady=False` → Receiver's buffer full
   - Look at which direction: North/South/East/West
   - Find receiving PE in that direction

3. **Check receiver status**:
   ```bash
   # Open CSV, look at receiving PE's columns
   # Is it also blocked? Is it consuming data?
   ```

4. **Track data flow backwards**:
   - From blocked PE, find what data it's trying to send
   - Find where that data comes from (upstream PE)
   - Check if upstream PE is sending correctly

---

### Workflow 3: "Data stopped flowing after cycle N"

1. **Open HTML debugger**:
   ```bash
   python3 generate_interactive_html.py test/axpy/axpy_run.log test/axpy/debug.html
   # Open in browser
   ```

2. **Navigate to problem cycle**:
   - Go to cycle N-1 (last working cycle)
   - See what data flowed
   - Identify sending and receiving PEs

3. **Go to cycle N**:
   - See if data flow continues
   - Check which PE has become BLOCKED
   - Click PE to see detailed reason

4. **Diagnose the blockage**:
   - Use Detail Panel to see exact reason
   - Check RecvBufHeadReady status
   - Look for buffer overflow

---

### Workflow 4: "Compare two different runs"

1. **Generate CSVs for both runs**:
   ```bash
   export_cgra_csv.py test/axpy/run1.log /tmp/run1
   export_cgra_csv.py test/axpy/run2.log /tmp/run2
   ```

2. **Open in Excel side-by-side**:
   - /tmp/run1/core_activity.csv (left)
   - /tmp/run2/core_activity.csv (right)

3. **Compare cycle-by-cycle**:
   - Look for differences in PE states
   - Check where execution diverges
   - Identify which PE behaves differently

4. **Create difference report**:
   - Highlight differences
   - Track data flow differences
   - Identify root cause of divergence

---

## FAQ

### Q: What do the colors in the HTML debugger mean?

**A**: 
- 🟢 **Green (EXEC)**: PE is executing (has events, not blocked)
- 🔴 **Red (BLOCKED)**: PE is blocked by backpressure
- ⚪ **Gray (IDLE)**: PE has no activity

---

### Q: Why does an arrow not point to the expected PE?

**A**: The visualization shows actual data flow from the events. If you see a Send from PE(2,2) to East, but expect it to go to a specific PE, check:
1. Is the receiving PE really to the East of PE(2,2)?
2. Is the receiving PE connected in the network?
3. Check the log for "From" and "To" fields in events

---

### Q: What is "RecvBufHeadReady"?

**A**: It indicates whether a receiving buffer is ready to accept new data:
- **TRUE**: Buffer is ready, can accept new data
- **FALSE**: Buffer is busy (old data not consumed), cannot accept new data

When FALSE, backpressure occurs because the PE cannot send data.

---

### Q: How is PE Utilization calculated?

**A**: 
```
PE Utilization = (Cycles with activity / Total cycles) × 100%
```

Activity includes:
- DataFlow (Send/Recv events)
- Memory operations (WriteMemory/ReadMemory)
- Instruction execution

---

### Q: What's the difference between "Actual" and "Expected" schedule?

**A**:
- **Expected**: From YAML config - what should happen
- **Actual**: From simulator log - what actually happened
- **Use**: Compare to find where simulation diverges from specification

---

### Q: Can I zoom the waveform visualization?

**A**: Yes! Multiple ways:
- Click **Zoom +** / **Zoom −** buttons
- Use **Ctrl + Mouse Wheel** (on browser)
- Click **Reset** to return to default zoom

Cycle labels adjust automatically when zoomed.

---

### Q: How do I find data leaving a specific PE?

**A**:
1. In CSV: Look for row with `PE(X,Y)-SendData` containing data
2. In HTML: Click PE and check "SEND" entries in Detail Panel
3. Trace where data goes by looking at timestamp and receiving PE's "RECV" entries

---

## Tips & Best Practices

### 1. Start with Small Datasets
- Modify `main.go` to use N=2 or N=3
- Shorter logs are easier to analyze
- Faster to test iterations

### 2. Focus on Key Cycles
- **Cycle 0**: Initialization/preload
- **Cycle 1-2**: First computation
- **Cycle 3-10**: First iteration completion and second iteration start
- **Later cycles**: Look for patterns or issues

### 3. Identify "Good" vs "Bad" Reference
- Keep a working version of your code
- Compare logs side-by-side in Excel
- Track where execution diverges

### 4. Use Multiple Tools Together
- Start with **CSV** for overview
- Then use **HTML debugger** for details
- Finally use **diagnosis tool** for root cause

### 5. Follow Data Flow Backwards
- Start at result consumer (STORE)
- Track where data comes from
- Trace backwards through PEs
- Find root cause at source

### 6. Check Initialization
- Always check Cycle 0
- Look for WriteMemory preload events
- Verify initial data is correct

### 7. Monitor Backpressure
- Frequent blocking = bottleneck
- Check if receiver can consume data
- Look for buffer overflow patterns

### 8. Document Your Findings
- Keep notes on what works/doesn't work
- Record cycle numbers of interest
- Track which PEs have issues
- Share findings with team

### 9. Use Regular Expressions in Excel
- Filter by pattern: `.*BLOCKED.*`
- Find cycles with specific opcodes
- Combine filters to narrow down issues

### 10. Export for Sharing
- CSV files are portable
- HTML debugger standalone
- Can send to others for review
- No special tools needed to view

---

## File Organization

All tools are located in `/workspaces/zeonica/`:

```
/workspaces/zeonica/
├── quick_debug.sh                    # One-command tool
├── export_cgra_csv.py               # CSV export
├── generate_debug_report.py          # Text reports
├── diagnose_backpressure.py         # Backpressure analysis
├── generate_interactive_html.py     # HTML debugger
├── debug_console.py                 # Interactive console
├── debug_cycle_trace.py             # Advanced cycle tracing
└── ANALYSIS_TOOLS_GUIDE.md          # This file
```

---

## Summary

These tools provide a complete debugging environment for CGRA kernels:

1. **CSV exports** → Data analysis in Excel
2. **Text reports** → Human-readable summaries
3. **HTML debugger** → Interactive visualization
4. **Backpressure diagnosis** → Root cause analysis
5. **Interactive console** → Query-based debugging

**Core principle**: Don't trust simulator output. Trace every data byte through the log.

Start with `./quick_debug.sh`, then dive deeper with specific tools as needed.

Good luck debugging! 🚀
