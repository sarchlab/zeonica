package verify

import (
	"fmt"
	"strings"

	"github.com/sarchlab/zeonica/core"
)

// RunLint performs static lint checks on a kernel program.
// It validates structure (STRUCT) and simple timing constraints (TIMING).
// For kernels with modulo scheduling (ii > 0), it uses a D∈{0,1} iteration
// distance model to reduce false positives on loop-carried dependencies.
// Returns a list of issues found, or empty list if no issues.
func RunLint(programs map[string]core.Program, arch *ArchInfo) []Issue {
	var issues []Issue

	// Extract CompiledII from the first program that has it
	// (All programs should have the same II since they're from the same kernel)
	ii := 0
	for _, prog := range programs {
		if prog.CompiledII > 0 {
			ii = prog.CompiledII
			break
		}
	}

	// STRUCT: Validate PE coordinates
	for coordStr := range programs {
		x, y, err := parseCoordinate(coordStr)
		if err != nil {
			issues = append(issues, Issue{
				Type:    IssueStruct,
				Message: fmt.Sprintf("Invalid coordinate format: %s (%v)", coordStr, err),
				OpID:    -1,
				PEX:     -1,
				PEY:     -1,
				Details: map[string]interface{}{"coord": coordStr},
			})
			continue
		}

		// Check bounds
		if x < 0 || x >= arch.Columns || y < 0 || y >= arch.Rows {
			issues = append(issues, Issue{
				Type:    IssueStruct,
				PEX:     x,
				PEY:     y,
				Message: fmt.Sprintf("PE coordinate out of bounds: (%d,%d) in %dx%d CGRA", x, y, arch.Columns, arch.Rows),
				OpID:    -1,
				Details: map[string]interface{}{
					"coord":  coordStr,
					"bounds": fmt.Sprintf("%d x %d", arch.Columns, arch.Rows),
				},
			})
		}
	}

	// STRUCT: Check for port write conflicts within same (PE, timestep)
	for coordStr, prog := range programs {
		x, y, err := parseCoordinate(coordStr)
		if err != nil || x < 0 || x >= arch.Columns || y < 0 || y >= arch.Rows {
			continue // Skip invalid coordinates
		}

		for entryIdx, entry := range prog.EntryBlocks {
			for timestepIdx, ig := range entry.InstructionGroups {
				// Track output ports being written in this timestep
				portWrites := make(map[string]int) // port key → operation index

				for opIdx, op := range ig.Operations {
					for _, dst := range op.DstOperands.Operands {
						// Check if this is a port operand
						if isPortOperand(dst.Impl) {
							portKey := dst.Impl + ":" + dst.Color
							if prevOpIdx, exists := portWrites[portKey]; exists {
								issues = append(issues, Issue{
									Type: IssueStruct,
									PEX:  x,
									PEY:  y,
									Time: timestepIdx,
									OpID: opIdx,
									Message: fmt.Sprintf(
										"Port write conflict: PE(%d,%d) at t=%d, port [%s,%s] written by op %d and op %d",
										x, y, timestepIdx, dst.Impl, dst.Color, prevOpIdx, opIdx,
									),
									Details: map[string]interface{}{
										"port":     dst.Impl,
										"color":    dst.Color,
										"opIdx":    opIdx,
										"prevOp":   prevOpIdx,
										"entry":    entryIdx,
										"timestep": timestepIdx,
									},
								})
							}
							portWrites[portKey] = opIdx
						}
					}
				}
			}
		}
	}

	// TIMING: Build dependency graph and check latencies with modulo scheduling support
	issues = append(issues, checkTimingConstraints(programs, arch, ii)...)

	return issues
}

// checkTimingConstraints performs STA on data dependencies with modulo scheduling support.
// For kernels with ii > 0 (modulo scheduled), uses a D∈{0,1} iteration distance model:
// - D=0: same iteration (t_cons - t_prod >= hopLatency)
// - D=1: next iteration (t_cons - t_prod + ii >= hopLatency)
// If either interpretation is valid, no error is reported (reduces false positives).
//
// CRITICAL FIX: This now correctly:
//  1. Uses real timesteps from InstructionGroup indices (not operation indices)
//  2. Distinguishes producer (dst operands) from consumer (src operands)
//  3. Matches producer/consumer ports via mesh neighbor + opposite direction:
//     - If consumer reads NORTH, producer is at (x, y-1) and writes SOUTH
//     - If consumer reads SOUTH, producer is at (x, y+1) and writes NORTH
//     - Similarly for EAST/WEST
func checkTimingConstraints(programs map[string]core.Program, arch *ArchInfo, ii int) []Issue {
	var issues []Issue

	// Build a map of all operations by (PE, timestep)
	// Key: "x,y" → list of (timestep, operations)
	// This ensures we use REAL timesteps from InstructionGroup indices
	prodConsEdges := make(map[string]*producerInfo) // key: "prodX,prodY,prodT,port,color"

	// Step 1: Collect all PRODUCER operations (those with dst ports)
	for coordStr, prog := range programs {
		prodX, prodY, err := parseCoordinate(coordStr)
		if err != nil || prodX < 0 || prodX >= arch.Columns || prodY < 0 || prodY >= arch.Rows {
			continue
		}

		for entryIdx, entry := range prog.EntryBlocks {
			// CRITICAL: t is the real timestep index from InstructionGroups slice
			for t, ig := range entry.InstructionGroups {
				for opIdx, op := range ig.Operations {
					// Scan dst_operands for port writes
					for _, dst := range op.DstOperands.Operands {
						if isPortOperand(dst.Impl) {
							// This operation WRITES to a port at time t
							key := fmt.Sprintf("%d,%d,%d,%s,%s", prodX, prodY, t, strings.ToUpper(dst.Impl), dst.Color)
							prodConsEdges[key] = &producerInfo{
								x:     prodX,
								y:     prodY,
								t:     t, // Real timestep from InstructionGroup index
								port:  strings.ToUpper(dst.Impl),
								color: dst.Color,
								opIdx: opIdx,
								entry: entryIdx,
								op:    op,
							}
						}
					}
				}
			}
		}
	}

	// Step 2: For each CONSUMER operation (those with src ports), find matching producer
	for coordStr, prog := range programs {
		consX, consY, err := parseCoordinate(coordStr)
		if err != nil || consX < 0 || consX >= arch.Columns || consY < 0 || consY >= arch.Rows {
			continue
		}

		for _, entry := range prog.EntryBlocks {
			// CRITICAL: t is the real timestep index from InstructionGroups slice
			for consT, ig := range entry.InstructionGroups {
				for opIdx, op := range ig.Operations {
					// Scan src_operands for port reads (consumers only read from src)
					for _, src := range op.SrcOperands.Operands {
						if isPortOperand(src.Impl) {
							// This operation READS from a port at time consT
							// Find the producer at the neighbor PE with opposite port direction

							consPort := strings.ToUpper(src.Impl)
							prodPort := oppositePort(consPort)
							prodX, prodY := neighborCoord(consX, consY, consPort)

							// Check bounds
							if prodX < 0 || prodX >= arch.Columns || prodY < 0 || prodY >= arch.Rows {
								continue // Neighbor is out of bounds
							}

							// Look for a producer at (prodX, prodY) that writes prodPort with same color
							// We search across ALL timesteps in the producer's program
							for prodT := 0; prodT < 1000; prodT++ { // arbitrary large number
								prodKey := fmt.Sprintf("%d,%d,%d,%s,%s", prodX, prodY, prodT, prodPort, src.Color)
								prodInfo, exists := prodConsEdges[prodKey]
								if !exists {
									continue // No producer at this timestep
								}

								// Found a producer! Apply D∈{0,1} timing model
								reqLatency := arch.HopLatency
								tProd := prodInfo.t // Real timestep from producer's InstructionGroup index
								tCons := consT      // Real timestep from consumer's InstructionGroup index

								// D∈{0,1} model: two interpretations of data dependence
								// D=0: same iteration
								delta0 := tCons - tProd
								ok0 := delta0 >= reqLatency

								// D=1: next iteration (if ii > 0, i.e., modulo scheduled)
								ok1 := false
								delta1 := 0
								if ii > 0 {
									delta1 = tCons - tProd + ii
									ok1 = delta1 >= reqLatency
								}

								// Only report violation if BOTH interpretations fail
								// (both D=0 and D=1 are invalid, meaning this is a real violation)
								if !ok0 && !ok1 {
									issues = append(issues, Issue{
										Type: IssueTiming,
										PEX:  consX,
										PEY:  consY,
										Time: consT,
										OpID: opIdx,
										Message: fmt.Sprintf(
											"Insufficient latency under both D=0 and D=1 assumptions: "+
												"producer PE(%d,%d) t=%d, consumer PE(%d,%d) t=%d, "+
												"hop=%d, ii=%d (delta0=%d, delta1=%d)",
											prodX, prodY, tProd, consX, consY, tCons,
											reqLatency, ii, delta0, delta1,
										),
										Details: map[string]interface{}{
											"consumer_x":       consX,
											"consumer_y":       consY,
											"consumer_t":       tCons,
											"producer_x":       prodX,
											"producer_y":       prodY,
											"producer_t":       tProd,
											"consumer_port":    consPort,
											"producer_port":    prodPort,
											"required_latency": reqLatency,
											"delta0":           delta0, // D=0: same iteration
											"delta1":           delta1, // D=1: next iteration
											"ii":               ii,
											"ok0":              ok0,
											"ok1":              ok1,
											"color":            src.Color,
										},
									})
								}
							}
						}
					}
				}
			}
		}
	}

	return issues
}

// isPortOperand checks if an operand is a port (direction name)
func isPortOperand(impl string) bool {
	dirNames := map[string]bool{
		"NORTH": true,
		"SOUTH": true,
		"EAST":  true,
		"WEST":  true,
		"North": true,
		"South": true,
		"East":  true,
		"West":  true,
	}
	return dirNames[impl]
}

// oppositePort returns the opposite port direction
func oppositePort(dir string) string {
	switch strings.ToUpper(dir) {
	case "NORTH":
		return "SOUTH"
	case "SOUTH":
		return "NORTH"
	case "EAST":
		return "WEST"
	case "WEST":
		return "EAST"
	default:
		return ""
	}
}

// neighborCoord calculates the neighbor PE coordinate when reading from a port.
// For 2D mesh topology:
// - Reading NORTH means producer is at (x, y-1) writing SOUTH
// - Reading SOUTH means producer is at (x, y+1) writing NORTH
// - Reading EAST means producer is at (x+1, y) writing WEST
// - Reading WEST means producer is at (x-1, y) writing EAST
func neighborCoord(x, y int, readDir string) (nx, ny int) {
	switch strings.ToUpper(readDir) {
	case "NORTH":
		// Reading from north means the producer is north of us
		return x, y - 1
	case "SOUTH":
		// Reading from south means the producer is south of us
		return x, y + 1
	case "EAST":
		// Reading from east means the producer is east of us
		return x + 1, y
	case "WEST":
		// Reading from west means the producer is west of us
		return x - 1, y
	default:
		return -1, -1
	}
}

// producerInfo tracks metadata about a producer operation that writes to a port
type producerInfo struct {
	x     int
	y     int
	t     int // Real timestep from InstructionGroup index
	port  string
	color string
	opIdx int
	entry int
	op    core.Operation
}
