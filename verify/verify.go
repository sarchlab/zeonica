// Package verify provides internal debugging tools for Zeonica kernel verification.
//
// This package implements two complementary verification stages:
//
// 1. Static Lint (lint.go): Fast structural and basic timing checks
//   - STRUCT checks: PE coordinate validity, port write conflicts, resource constraints
//   - TIMING checks: Simple STA for data-flow dependencies
//
// 2. Functional Simulator (funcsim.go): Lightweight dataflow-only interpreter
//   - Executes kernel without cycle-accurate timing or backpressure
//   - Checks semantic correctness of kernel computation
//   - Useful for isolating compiler bugs from simulator bugs
//
// # IR Structure
//
// A core.Program represents a single PE's kernel:
//
//	core.Program
//	  └── EntryBlock (one or more)
//	      └── InstructionGroup (one or more per entry)
//	          └── Operation (one or more, can execute in parallel)
//	              ├── OpCode (string: "MOV", "ADD", "PHI", etc.)
//	              ├── SrcOperands (OperandList)
//	              └── DstOperands (OperandList)
//
// Operands are either:
//   - Register: "$0", "$1", ... (variables stored per PE)
//   - Port: "[NORTH, RED]", "[EAST, YELLOW]", ... (network I/O)
//   - Immediate: "#42" (constants)
//
// Programs are loaded from YAML/ASM and indexed by coordinate key "(x,y)".
//
// # Opcode Semantics
//
// Opcodes are implemented in core/emu.go (see lines ~850 for dispatch).
// This package extracts opcode semantics from that implementation:
//
//   - MOV: Copy data between registers/ports
//   - ADD, SUB, MUL, DIV: Arithmetic operations
//   - FADD, FSUB, FMUL, FDIV: Floating-point arithmetic
//   - ICMP_*: Integer comparisons (EQ, LT, GT, etc.)
//   - PHI: Data merge based on predicate readiness
//   - GPRED: Grant predicate (conditional execution)
//   - LOAD, STORE: Memory operations
//   - GEP: Get element pointer (address arithmetic)
//   - RET, JMP, BEQ, BNE, BLT: Control flow
//   - NOP: No operation
//
// # Architecture Model
//
// ArchInfo captures CGRA topology and constraints:
//
//   - Rows, Columns: Grid dimensions (e.g., 4x4 for histogram)
//   - Topology: "mesh" (4 neighbors per PE: N/S/E/W)
//   - HopLatency: Cycles per network hop (default 1)
//   - MemCapacity: Memory per PE (words or bytes)
//   - CtrlMemItems: Control memory entries (if applicable)
//
// Cross-PE communication assumes mesh topology:
//   - PE(x, y) → [EAST, RED] → PE(x+1, y)
//   - PE(x, y) → [NORTH, RED] → PE(x, y+1)
//   - etc.
//
// # Predicate System
//
// Data carries a predicate (valid/invalid flag):
//
//	type Data struct {
//	    Data []uint32  // Actual value(s)
//	    Pred bool      // true = valid, false = masked/invalid
//	}
//
// Operations check predicates before execution:
//   - Register sources must have Pred=true
//   - Port receives must have Pred=true AND data predicate true
//   - Output predicate depends on operation (some ops propagate, some set false, etc.)
//
// # Usage Example
//
//	arch := verify.LoadArchInfoFromConfig(device)
//	programs := core.LoadProgramFileFromYAML("histogram.yaml")
//
//	// Stage 1: Lint checks
//	issues := verify.RunLint(programs, arch)
//	if len(issues) > 0 {
//	    for _, issue := range issues {
//	        log.Printf("[%s] PE(%d,%d) t=%d: %s", issue.Type, issue.PEX, issue.PEY, issue.Time, issue.Message)
//	    }
//	    panic("Lint found issues; check compiler output")
//	}
//
//	// Stage 2: Functional simulation
//	fs := verify.NewFunctionalSimulator(programs, arch)
//	fs.PreloadMemory(2, 3, 0, 0xFF)  // Example: preload address 0 with value 0xFF
//	if err := fs.Run(10000); err != nil {
//	    panic(err)
//	}
//
//	// Query results
//	val := fs.GetRegisterValue(2, 3, 0)
//	fmt.Printf("Core (2,3) $0 = 0x%x\n", val)
//
//	// Stage 3: If needed, run cycle-accurate simulator
//	// (existing code unchanged)
//
// # Limitations
//
// - Modulo-scheduled kernels (II > 1): Not supported yet
// - Network delays: Simplified (immediate in funcsim)
// - Backpressure: Ignored (no buffer stalls in funcsim)
// - Floating-point: Uses Go native float32/float64 (no special rounding)
// - Memory coherence: None (per-PE memory only)
//
// # Future Work
//
// - Extended STA with pipeline/multicycle dependency analysis
// - Support for modulo-scheduled kernels
// - Configurable network delay model
// - Memory hierarchy modeling
// - Performance profiling & optimization
package verify

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/sarchlab/zeonica/core"
)

// IssueType categorizes lint issues
type IssueType string

const (
	IssueStruct IssueType = "STRUCT" // Mapping/structure error (illegal PE, port conflict)
	IssueTiming IssueType = "TIMING" // Dependency/timing error (insufficient latency)
)

// Issue represents a single lint issue
type Issue struct {
	Type    IssueType              // STRUCT or TIMING
	PEX     int                    // PE X coordinate (-1 if not applicable)
	PEY     int                    // PE Y coordinate (-1 if not applicable)
	Time    int                    // Timestep (-1 if not applicable)
	OpID    int                    // Operation index or -1
	Message string                 // Human-readable description
	Details map[string]interface{} // Additional structured data
}

// ArchInfo describes CGRA topology and constraints
type ArchInfo struct {
	Rows         int    // Number of rows in CGRA grid
	Columns      int    // Number of columns in CGRA grid
	Topology     string // Topology type (e.g., "mesh")
	HopLatency   int    // Cycles per network hop (default 1)
	MemCapacity  int    // Memory capacity per PE (in words or bytes)
	CtrlMemItems int    // Control memory entries per PE
}

// PEState captures the runtime state of a single PE (for functional simulator)
type PEState struct {
	Registers  map[int]core.Data      // Register file: register index → Data
	Memory     map[uint32]core.Data   // Local memory: address → Data
	LocalState map[string]interface{} // For advanced state (predicates, etc.)
}

// NewPEState creates a new PE state with empty registers and memory
func NewPEState() *PEState {
	return &PEState{
		Registers:  make(map[int]core.Data),
		Memory:     make(map[uint32]core.Data),
		LocalState: make(map[string]interface{}),
	}
}

// WriteReg writes a value to a register
func (ps *PEState) WriteReg(regIndex int, data core.Data) {
	ps.Registers[regIndex] = data
}

// ReadReg reads a value from a register (returns Data{Pred: false} if not found)
func (ps *PEState) ReadReg(regIndex int) core.Data {
	if data, ok := ps.Registers[regIndex]; ok {
		return data
	}
	return core.Data{Data: []uint32{0}, Pred: false}
}

// WriteMemory writes a value to memory at an address
func (ps *PEState) WriteMemory(address uint32, data core.Data) {
	ps.Memory[address] = data
}

// ReadMemory reads a value from memory at an address
func (ps *PEState) ReadMemory(address uint32) core.Data {
	if data, ok := ps.Memory[address]; ok {
		return data
	}
	return core.Data{Data: []uint32{0}, Pred: false}
}

// FunctionalSimulator executes a kernel program without cycle-accurate timing
type FunctionalSimulator struct {
	programs map[string]core.Program
	arch     *ArchInfo
	peStates [][]*PEState

	currentT  int
	TraceOpPre  func(x, y, t int, op *core.Operation)
	TraceOpPost func(x, y, t int, op *core.Operation)
	TraceStore func(x, y, t int, addr uint32, value core.Data, op *core.Operation)
}

// NewFunctionalSimulator creates a new functional simulator
func NewFunctionalSimulator(programs map[string]core.Program, arch *ArchInfo) *FunctionalSimulator {
	fs := &FunctionalSimulator{
		programs: programs,
		arch:     arch,
		peStates: make([][]*PEState, arch.Rows),
	}

	// Initialize PE states grid
	for y := 0; y < arch.Rows; y++ {
		fs.peStates[y] = make([]*PEState, arch.Columns)
		for x := 0; x < arch.Columns; x++ {
			fs.peStates[y][x] = NewPEState()
		}
	}

	return fs
}

// PreloadMemory preloads a memory location with a value
func (fs *FunctionalSimulator) PreloadMemory(x, y int, value uint32, address uint32) error {
	if y < 0 || y >= fs.arch.Rows || x < 0 || x >= fs.arch.Columns {
		return fmt.Errorf("invalid PE coordinate (%d, %d)", x, y)
	}
	fs.peStates[y][x].WriteMemory(address, core.NewScalar(value))
	return nil
}

// GetRegisterValue retrieves a register value from a PE
func (fs *FunctionalSimulator) GetRegisterValue(x, y int, regIndex int) uint32 {
	if y < 0 || y >= fs.arch.Rows || x < 0 || x >= fs.arch.Columns {
		return 0
	}
	return fs.peStates[y][x].ReadReg(regIndex).First()
}

// GetRegisterData retrieves a register value with predicate
func (fs *FunctionalSimulator) GetRegisterData(x, y int, regIndex int) core.Data {
	if y < 0 || y >= fs.arch.Rows || x < 0 || x >= fs.arch.Columns {
		return core.Data{Data: []uint32{0}, Pred: false}
	}
	return fs.peStates[y][x].ReadReg(regIndex)
}

// GetMemoryValue retrieves a memory value from a PE
func (fs *FunctionalSimulator) GetMemoryValue(x, y int, address uint32) uint32 {
	if y < 0 || y >= fs.arch.Rows || x < 0 || x >= fs.arch.Columns {
		return 0
	}
	return fs.peStates[y][x].ReadMemory(address).First()
}

// DebugReadOperand reads an operand value for tracing
func (fs *FunctionalSimulator) DebugReadOperand(x, y int, operand core.Operand) core.Data {
	return fs.readOperand(x, y, &operand)
}

// DebugGetPortBuffer returns a snapshot of port tokens at a PE.
func (fs *FunctionalSimulator) DebugGetPortBuffer(x, y int) map[string]core.Data {
	if y < 0 || y >= fs.arch.Rows || x < 0 || x >= fs.arch.Columns {
		return map[string]core.Data{}
	}
	ports := getPortBuffer(fs.peStates[y][x])
	out := make(map[string]core.Data, len(ports))
	for k, v := range ports {
		out[k] = v
	}
	return out
}

// GetMemoryRange retrieves a range of memory values
func (fs *FunctionalSimulator) GetMemoryRange(x, y int, addrStart, addrEnd uint32) []uint32 {
	if y < 0 || y >= fs.arch.Rows || x < 0 || x >= fs.arch.Columns {
		return nil
	}
	var result []uint32
	for addr := addrStart; addr <= addrEnd; addr++ {
		result = append(result, fs.peStates[y][x].ReadMemory(addr).First())
	}
	return result
}

// LoadArchInfoFromConfig creates ArchInfo from a device configuration
// For now, we assume a simple 4x4 mesh; this can be extended to parse actual config
func LoadArchInfoFromConfig(rows, cols int) *ArchInfo {
	return &ArchInfo{
		Rows:         rows,
		Columns:      cols,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  1024, // Default 1KB per PE
		CtrlMemItems: 256,  // Default 256 control memory items
	}
}

// parseCoordinate parses a coordinate string "(x,y)" into (x, y)
func parseCoordinate(coordStr string) (int, int, error) {
	// Expected format: "(x,y)" or "(x, y)" with optional space
	re := regexp.MustCompile(`\(\s*(\d+)\s*,\s*(\d+)\s*\)`)
	matches := re.FindStringSubmatch(coordStr)
	if len(matches) != 3 {
		return -1, -1, fmt.Errorf("invalid coordinate format: %s", coordStr)
	}

	x, err1 := strconv.Atoi(matches[1])
	y, err2 := strconv.Atoi(matches[2])
	if err1 != nil || err2 != nil {
		return -1, -1, fmt.Errorf("failed to parse coordinate: %s", coordStr)
	}

	return x, y, nil
}

// getNeighbor returns the neighbor PE coordinates given a direction
// Returns (nx, ny, valid) where valid is false if neighbor is out of bounds
func (arch *ArchInfo) getNeighbor(x, y int, dir string) (int, int, bool) {
	dirUpper := strings.ToUpper(dir)
	switch dirUpper {
	case "NORTH":
		ny := y + 1
		if ny >= arch.Rows {
			return -1, -1, false
		}
		return x, ny, true
	case "SOUTH":
		ny := y - 1
		if ny < 0 {
			return -1, -1, false
		}
		return x, ny, true
	case "EAST":
		nx := x + 1
		if nx >= arch.Columns {
			return -1, -1, false
		}
		return nx, y, true
	case "WEST":
		nx := x - 1
		if nx < 0 {
			return -1, -1, false
		}
		return nx, y, true
	default:
		return -1, -1, false
	}
}
