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
func checkTimingConstraints(programs map[string]core.Program, arch *ArchInfo, ii int) []Issue {
	var issues []Issue

	// Build a map of all operations by (PE, timestep, opIdx)
	opMap := make(map[string]*operationInfo)        // key: "x,y,t,opIdx"
	coordToOps := make(map[string][]*operationInfo) // key: "x,y"

	for coordStr, prog := range programs {
		x, y, err := parseCoordinate(coordStr)
		if err != nil || x < 0 || x >= arch.Columns || y < 0 || y >= arch.Rows {
			continue
		}

		for entryIdx, entry := range prog.EntryBlocks {
			for t, ig := range entry.InstructionGroups {
				for opIdx, op := range ig.Operations {
					info := &operationInfo{
						x:     x,
						y:     y,
						t:     t,
						opIdx: opIdx,
						entry: entryIdx,
						op:    op,
					}
					key := fmt.Sprintf("%d,%d,%d,%d", x, y, t, opIdx)
					opMap[key] = info

					coordKey := fmt.Sprintf("%d,%d", x, y)
					coordToOps[coordKey] = append(coordToOps[coordKey], info)
				}
			}
		}
	}

	// For each operation, check its source operands
	for _, opInfo := range opMap {
		for _, src := range opInfo.op.SrcOperands.Operands {
			// Check if this is a port operand (cross-PE data dependency)
			if isPortOperand(src.Impl) {
				// This is a port read. Find the corresponding writer.
				// For mesh topology, infer the neighbor.
				dirUpper := strings.ToUpper(src.Impl)

				// Infer source PE (producer)
				var srcX, srcY int
				switch dirUpper {
				case "NORTH":
					// We're reading from NORTH, so producer is to our north
					srcX, srcY = opInfo.x, opInfo.y+1
				case "SOUTH":
					srcX, srcY = opInfo.x, opInfo.y-1
				case "EAST":
					srcX, srcY = opInfo.x+1, opInfo.y
				case "WEST":
					srcX, srcY = opInfo.x-1, opInfo.y
				default:
					continue
				}

				// Check bounds
				if srcX < 0 || srcX >= arch.Columns || srcY < 0 || srcY >= arch.Rows {
					continue // Out of bounds, skip
				}

				// The producer writes to the opposite port direction
				producerPort := oppositePort(dirUpper)
				producerCoordKey := fmt.Sprintf("%d,%d", srcX, srcY)

				// Find producer operation (scan all operations on producer PE)
				for _, producerOp := range coordToOps[producerCoordKey] {
					// Check if this producer writes to the port we're reading from
					for _, dst := range producerOp.op.DstOperands.Operands {
						if strings.ToUpper(dst.Impl) == producerPort && dst.Color == src.Color {
							// Found the producer! Check timing with modulo scheduling model.
							reqLatency := arch.HopLatency
							tProd := producerOp.t
							tCons := opInfo.t

							// Compute deltas for D=0 (same iteration) and D=1 (next iteration)
							delta0 := tCons - tProd
							delta1 := tCons - tProd + ii

							// Check if either interpretation satisfies the latency constraint
							ok0 := delta0 >= reqLatency
							ok1 := delta1 >= reqLatency

							// Only report an issue if BOTH interpretations fail
							if !ok0 && !ok1 {
								issues = append(issues, Issue{
									Type: IssueTiming,
									PEX:  opInfo.x,
									PEY:  opInfo.y,
									Time: opInfo.t,
									OpID: opInfo.opIdx,
									Message: fmt.Sprintf(
										"Insufficient latency under both D=0 and D=1 assumptions: "+
											"producer PE(%d,%d) t=%d, consumer PE(%d,%d) t=%d, "+
											"hop=%d, ii=%d (delta0=%d, delta1=%d)",
										srcX, srcY, tProd, opInfo.x, opInfo.y, tCons,
										reqLatency, ii, delta0, delta1,
									),
									Details: map[string]interface{}{
										"consumer_x":       opInfo.x,
										"consumer_y":       opInfo.y,
										"consumer_t":       tCons,
										"producer_x":       srcX,
										"producer_y":       srcY,
										"producer_t":       tProd,
										"required_latency": reqLatency,
										"delta0":           delta0, // Assuming D=0
										"delta1":           delta1, // Assuming D=1
										"ii":               ii,
										"ok0":              ok0,
										"ok1":              ok1,
										"port":             producerPort,
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

	return issues
}

// operationInfo tracks metadata about an operation for dependency analysis
type operationInfo struct {
	x     int
	y     int
	t     int
	opIdx int
	entry int
	op    core.Operation
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
