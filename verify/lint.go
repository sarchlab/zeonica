//nolint:funlen
package verify

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sarchlab/zeonica/core"
)

// RunLint performs static lint checks on a kernel program.
// It validates structure (STRUCT) and simple timing constraints (TIMING).
// For kernels with modulo scheduling (ii > 0), it uses a D∈{0,1} iteration
// distance model to reduce false positives on loop-carried dependencies.
// Optional lint options can be provided to tune predicate analysis behavior.
// Returns a list of issues found, or empty list if no issues.
//
//nolint:gocyclo
func RunLint(programs map[string]core.Program, arch *ArchInfo, opts ...LintOptions) []Issue {
	var issues []Issue
	lintOpts := DefaultLintOptions()
	if len(opts) > 0 {
		lintOpts = normalizeLintOptions(opts[0])
	}

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
	// PREDICATE: Check PHI/PHI_START/GRANT predicate consistency risks.
	issues = append(issues, checkPredicateConstraints(programs, arch, ii, lintOpts)...)

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
//
//nolint:gocyclo
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

type predMask uint8

const (
	predCanTrue predMask = 1 << iota
	predCanFalse
)

const (
	predTrueMask    predMask = predCanTrue
	predFalseMask   predMask = predCanFalse
	predUnknownMask predMask = predCanTrue | predCanFalse
)

func predHasTrue(v predMask) bool {
	return v&predCanTrue != 0
}

func predHasFalse(v predMask) bool {
	return v&predCanFalse != 0
}

func predAnd(values ...predMask) predMask {
	canTrue := true
	canFalse := false
	for _, v := range values {
		canTrue = canTrue && predHasTrue(v)
		canFalse = canFalse || predHasFalse(v)
	}

	var out predMask
	if canTrue {
		out |= predCanTrue
	}
	if canFalse {
		out |= predCanFalse
	}
	if out == 0 {
		return predUnknownMask
	}
	return out
}

func predOr(a, b predMask) predMask {
	out := predMask(0)
	if predHasTrue(a) || predHasTrue(b) {
		out |= predCanTrue
	}
	if predHasFalse(a) || predHasFalse(b) {
		out |= predCanFalse
	}
	if out == 0 {
		return predUnknownMask
	}
	return out
}

func parseRegisterIndex(impl string) (int, bool) {
	if !strings.HasPrefix(impl, "$") {
		return 0, false
	}
	idx, err := strconv.Atoi(strings.TrimPrefix(impl, "$"))
	if err != nil {
		return 0, false
	}
	return idx, true
}

func parseImmediateInt(impl string) (int64, bool) {
	if !strings.HasPrefix(impl, "#") {
		return 0, false
	}
	v := strings.TrimPrefix(impl, "#")
	num, err := strconv.ParseInt(v, 0, 64)
	if err == nil {
		return num, true
	}
	u, err := strconv.ParseUint(v, 0, 64)
	if err != nil {
		return 0, false
	}
	return int64(u), true
}

func operandPredMask(operand core.Operand, regPred map[int]predMask) predMask {
	if idx, ok := parseRegisterIndex(operand.Impl); ok {
		if p, exists := regPred[idx]; exists {
			return p
		}
		return predUnknownMask
	}
	if strings.HasPrefix(operand.Impl, "#") {
		return predTrueMask
	}
	if isPortOperand(operand.Impl) {
		return predUnknownMask
	}
	return predUnknownMask
}

func predicateGateMask(operand core.Operand) predMask {
	v, ok := parseImmediateInt(operand.Impl)
	if !ok {
		return predUnknownMask
	}
	if v == 0 {
		return predFalseMask
	}
	return predTrueMask
}

func writeRegisterDsts(op core.Operation, regPred map[int]predMask, pred predMask) {
	for _, dst := range op.DstOperands.Operands {
		if idx, ok := parseRegisterIndex(dst.Impl); ok {
			regPred[idx] = pred
		}
	}
}

func andFromSrcOperands(op core.Operation, regPred map[int]predMask) predMask {
	if len(op.SrcOperands.Operands) == 0 {
		return predUnknownMask
	}
	preds := make([]predMask, 0, len(op.SrcOperands.Operands))
	for _, src := range op.SrcOperands.Operands {
		preds = append(preds, operandPredMask(src, regPred))
	}
	return predAnd(preds...)
}

type predicateStage string

const (
	predicateStageWarmup predicateStage = "warmup"
	predicateStageSteady predicateStage = "steady"
)

type predicateRiskStat struct {
	x       int
	y       int
	t       int
	opID    int
	message string
	opcode  string

	totalHits    int
	definiteHits int
	stageHits    map[predicateStage]int
}

func newPredicateRiskStat(x, y, t, opID int, message, opcode string) *predicateRiskStat {
	return &predicateRiskStat{
		x:       x,
		y:       y,
		t:       t,
		opID:    opID,
		message: message,
		opcode:  opcode,
		stageHits: map[predicateStage]int{
			predicateStageWarmup: 0,
			predicateStageSteady: 0,
		},
	}
}

func (p *predicateRiskStat) mark(stage predicateStage, definite bool) {
	p.totalHits++
	p.stageHits[stage]++
	if definite {
		p.definiteHits++
	}
}

func (p *predicateRiskStat) certainty() string {
	if p.totalHits > 0 && p.definiteHits == p.totalHits {
		return "definite"
	}
	return "possible"
}

func computePredicatePassWindows(maxInvalid, ii int, opts LintOptions) (int, int) {
	if !opts.EnablePrologueAwarePredicate {
		passCount := maxInvalid + 1
		if passCount < 1 {
			passCount = 1
		}
		if passCount > 4 {
			// Keep lint bounded in legacy mode.
			passCount = 4
		}
		return passCount, 0
	}

	warmupPasses := maxInvalid + 1
	if ii > 0 && ii+1 > warmupPasses {
		warmupPasses = ii + 1
	}
	if warmupPasses < 1 {
		warmupPasses = 1
	}
	if warmupPasses > opts.PredicateWarmupPassCap {
		warmupPasses = opts.PredicateWarmupPassCap
	}

	steadyPasses := opts.PredicateSteadyStatePasses
	if steadyPasses < 0 {
		steadyPasses = 0
	}
	return warmupPasses, steadyPasses
}

func recordPredicateRisk(
	stats map[string]*predicateRiskStat,
	key string,
	x, y, t, opID int,
	message, opcode string,
	stage predicateStage,
	definite bool,
) {
	s, exists := stats[key]
	if !exists {
		s = newPredicateRiskStat(x, y, t, opID, message, opcode)
		stats[key] = s
	}
	s.mark(stage, definite)
}

//nolint:gocyclo
func checkPredicateConstraints(
	programs map[string]core.Program,
	arch *ArchInfo,
	ii int,
	opts LintOptions,
) []Issue {
	var issues []Issue

	type opCursor struct {
		timeIdx    int
		op         core.Operation
		invalidRem int
	}

	for coordStr, prog := range programs {
		x, y, err := parseCoordinate(coordStr)
		if err != nil || x < 0 || x >= arch.Columns || y < 0 || y >= arch.Rows {
			continue
		}

		regPred := make(map[int]predMask)
		phiStartSeen := make(map[int]bool)
		grantOnceSeen := make(map[int]bool)
		riskStats := make(map[string]*predicateRiskStat)

		ops := make([]*opCursor, 0)
		maxInvalid := 0
		for _, entry := range prog.EntryBlocks {
			for t, ig := range entry.InstructionGroups {
				for _, op := range ig.Operations {
					if op.InvalidIterations > maxInvalid {
						maxInvalid = op.InvalidIterations
					}
					ops = append(ops, &opCursor{
						timeIdx:    t,
						op:         op,
						invalidRem: op.InvalidIterations,
					})
				}
			}
		}

		warmupPasses, steadyPasses := computePredicatePassWindows(maxInvalid, ii, opts)
		totalPasses := warmupPasses + steadyPasses
		if totalPasses < 1 {
			totalPasses = 1
		}

		for pass := 0; pass < totalPasses; pass++ {
			stage := predicateStageWarmup
			if pass >= warmupPasses {
				stage = predicateStageSteady
			}
			for _, item := range ops {
				if item.invalidRem > 0 {
					item.invalidRem--
					continue
				}

				op := item.op
				opName := strings.ToUpper(op.OpCode)

				switch opName {
				case "PHI_START":
					if len(op.SrcOperands.Operands) < 2 {
						writeRegisterDsts(op, regPred, predUnknownMask)
						continue
					}
					src1 := operandPredMask(op.SrcOperands.Operands[0], regPred)
					src2 := operandPredMask(op.SrcOperands.Operands[1], regPred)

					if !phiStartSeen[op.ID] {
						if predHasFalse(src1) {
							recordPredicateRisk(
								riskStats,
								fmt.Sprintf("phi_start_first:%d", op.ID),
								x, y, item.timeIdx, op.ID,
								fmt.Sprintf("PHI_START id=%d first source may have pred=false on first execution", op.ID),
								opName,
								stage,
								src1 == predFalseMask,
							)
						}
						phiStartSeen[op.ID] = true
						writeRegisterDsts(op, regPred, src1)
					} else {
						if predHasTrue(src1) && predHasTrue(src2) {
							recordPredicateRisk(
								riskStats,
								fmt.Sprintf("phi_start_both_true:%d", op.ID),
								x, y, item.timeIdx, op.ID,
								fmt.Sprintf("PHI_START id=%d may see both source predicates true", op.ID),
								opName,
								stage,
								src1 == predTrueMask && src2 == predTrueMask,
							)
						}
						writeRegisterDsts(op, regPred, predOr(src1, src2))
					}
				case "PHI":
					if len(op.SrcOperands.Operands) < 2 {
						writeRegisterDsts(op, regPred, predUnknownMask)
						continue
					}
					src1 := operandPredMask(op.SrcOperands.Operands[0], regPred)
					src2 := operandPredMask(op.SrcOperands.Operands[1], regPred)
					if predHasTrue(src1) && predHasTrue(src2) {
						recordPredicateRisk(
							riskStats,
							fmt.Sprintf("phi_both_true:%d", op.ID),
							x, y, item.timeIdx, op.ID,
							fmt.Sprintf("PHI id=%d may have both source predicates true", op.ID),
							opName,
							stage,
							src1 == predTrueMask && src2 == predTrueMask,
						)
					}
					writeRegisterDsts(op, regPred, predOr(src1, src2))
				case "GRANT_PREDICATE", "GPRED":
					if len(op.SrcOperands.Operands) < 2 {
						writeRegisterDsts(op, regPred, predUnknownMask)
						continue
					}
					srcPred := operandPredMask(op.SrcOperands.Operands[0], regPred)
					predPred := operandPredMask(op.SrcOperands.Operands[1], regPred)
					gate := predicateGateMask(op.SrcOperands.Operands[1])
					writeRegisterDsts(op, regPred, predAnd(srcPred, predPred, gate))
				case "GRANT_ONCE":
					if len(op.SrcOperands.Operands) == 0 {
						writeRegisterDsts(op, regPred, predUnknownMask)
						continue
					}
					srcPred := operandPredMask(op.SrcOperands.Operands[0], regPred)
					if !grantOnceSeen[op.ID] {
						grantOnceSeen[op.ID] = true
						writeRegisterDsts(op, regPred, srcPred)
					} else {
						writeRegisterDsts(op, regPred, predFalseMask)
					}
				case "MOV", "DATA_MOV", "CTRL_MOV", "SEXT", "ZEXT", "CAST_FPTOSI", "NOT", "LOAD":
					if len(op.SrcOperands.Operands) == 0 {
						writeRegisterDsts(op, regPred, predUnknownMask)
						continue
					}
					writeRegisterDsts(op, regPred, operandPredMask(op.SrcOperands.Operands[0], regPred))
				case "CONSTANT":
					writeRegisterDsts(op, regPred, predTrueMask)
				case "PHI_CONST":
					if len(op.SrcOperands.Operands) < 2 {
						writeRegisterDsts(op, regPred, predUnknownMask)
						continue
					}
					src1 := operandPredMask(op.SrcOperands.Operands[0], regPred)
					src2 := operandPredMask(op.SrcOperands.Operands[1], regPred)
					writeRegisterDsts(op, regPred, predOr(src1, src2))
				case "ADD", "SUB", "MUL", "DIV", "FADD", "FSUB", "FMUL", "FDIV",
					"OR", "XOR", "AND", "SHL", "LLS", "LRS", "GEP", "MUL_ADD",
					"ICMP_EQ", "ICMP_SLT", "ICMP_SGT", "ICMP_SGE", "ICMP_SLE", "ICMP_SNE", "LT_EX":
					writeRegisterDsts(op, regPred, andFromSrcOperands(op, regPred))
				default:
					// Unknown opcode to lint: keep analysis conservative.
					writeRegisterDsts(op, regPred, predUnknownMask)
				}
			}
		}

		keys := make([]string, 0, len(riskStats))
		for key := range riskStats {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			stat := riskStats[key]
			issues = append(issues, Issue{
				Type:    IssuePredicate,
				PEX:     stat.x,
				PEY:     stat.y,
				Time:    stat.t,
				OpID:    stat.opID,
				Message: stat.message,
				Details: map[string]interface{}{
					"certainty":     stat.certainty(),
					"opcode":        stat.opcode,
					"warmup_hits":   stat.stageHits[predicateStageWarmup],
					"steady_hits":   stat.stageHits[predicateStageSteady],
					"total_hits":    stat.totalHits,
					"definite_hits": stat.definiteHits,
				},
			})
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
