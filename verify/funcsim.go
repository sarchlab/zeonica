package verify

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/sarchlab/zeonica/core"
)

// Run executes the functional simulator for up to maxSteps iterations.
// Returns an error if execution fails.
func (fs *FunctionalSimulator) Run(maxSteps int) error {
	if fs.programs == nil || fs.arch == nil {
		return fmt.Errorf("FunctionalSimulator not properly initialized")
	}

	// Build operation list with all operations sorted by timestep
	type OpExecRecord struct {
		x        int
		y        int
		t        int
		opIdx    int
		entry    int
		op       core.Operation
		executed bool
	}
	var allOps []*OpExecRecord

	// Collect all operations
	for coordStr, prog := range fs.programs {
		x, y, err := parseCoordinate(coordStr)
		if err != nil || x < 0 || x >= fs.arch.Columns || y < 0 || y >= fs.arch.Rows {
			continue
		}

		for entryIdx, entry := range prog.EntryBlocks {
			for t, ig := range entry.InstructionGroups {
				for opIdx, op := range ig.Operations {
					allOps = append(allOps, &OpExecRecord{
						x:        x,
						y:        y,
						t:        t,
						opIdx:    opIdx,
						entry:    entryIdx,
						op:       op,
						executed: false,
					})
				}
			}
		}
	}

	// Execute in topological order (by timestep)
	for step := 0; step < maxSteps; step++ {
		progress := false

		for _, rec := range allOps {
			if rec.executed {
				continue
			}

			// Try to execute this operation
			if fs.canExecuteOp(rec.x, rec.y, &rec.op) {
				fs.executeOp(rec.x, rec.y, &rec.op)
				rec.executed = true
				progress = true
			}
		}

		if !progress {
			break // No more operations can execute
		}
	}

	return nil
}

// canExecuteOp checks if all source operands are ready
func (fs *FunctionalSimulator) canExecuteOp(x, y int, op *core.Operation) bool {
	for _, src := range op.SrcOperands.Operands {
		if !fs.isOperandReady(x, y, &src) {
			return false
		}
	}
	return true
}

// isOperandReady checks if a single operand is ready for reading
func (fs *FunctionalSimulator) isOperandReady(x, y int, operand *core.Operand) bool {
	if strings.HasPrefix(operand.Impl, "$") {
		// Register operand: check if predicate is true
		regIdx, err := strconv.Atoi(strings.TrimPrefix(operand.Impl, "$"))
		if err != nil {
			return false
		}
		return fs.peStates[y][x].ReadReg(regIdx).Pred
	}

	if strings.HasPrefix(operand.Impl, "#") {
		// Immediate: always ready
		return true
	}

	if isPortOperand(operand.Impl) {
		// Port operand: data available from neighbor (in funcsim, we assume ports are always ready)
		// In functional model, we don't model backpressure
		return true
	}

	return false
}

// executeOp executes a single operation
func (fs *FunctionalSimulator) executeOp(x, y int, op *core.Operation) {
	switch strings.ToUpper(op.OpCode) {
	case "MOV":
		fs.runMov(x, y, op)
	case "DATA_MOV":
		// Newer kernels use DATA_MOV as an alias of MOV.
		fs.runMov(x, y, op)
	case "ADD":
		fs.runAdd(x, y, op)
	case "SUB":
		fs.runSub(x, y, op)
	case "MUL":
		fs.runMul(x, y, op)
	case "DIV":
		fs.runDiv(x, y, op)
	case "FADD":
		fs.runFAdd(x, y, op)
	case "FSUB":
		fs.runFSub(x, y, op)
	case "FMUL":
		fs.runFMul(x, y, op)
	case "FDIV":
		fs.runFDiv(x, y, op)
	case "PHI":
		fs.runPhi(x, y, op)
	case "GPRED":
		fs.runGpred(x, y, op)
	case "GRANT_PREDICATE":
		// Alias used by newer kernels (e.g., FIR): same semantics as GPRED.
		fs.runGpred(x, y, op)
	case "LOAD":
		fs.runLoad(x, y, op)
	case "STORE":
		fs.runStore(x, y, op)
	case "GEP":
		fs.runGep(x, y, op)
	case "LLS", "SHL":
		fs.runLLS(x, y, op)
	case "LRS":
		fs.runLRS(x, y, op)
	case "OR":
		fs.runOr(x, y, op)
	case "XOR":
		fs.runXor(x, y, op)
	case "AND":
		fs.runAnd(x, y, op)
	case "NOT":
		fs.runNot(x, y, op)
	case "ICMP_EQ", "ICMP_SGT", "ICMP_SLT", "ICMP_SGE", "ICMP_SLE", "ICMP_SNE":
		fs.runIcmp(x, y, op)
	case "LT_EX":
		fs.runLtEx(x, y, op)
	case "SEXT":
		fs.runSext(x, y, op)
	case "ZEXT":
		fs.runZext(x, y, op)
	case "CAST_FPTOSI":
		fs.runCastFptoSi(x, y, op)
	case "FMUL_FADD":
		fs.runFMulFAdd(x, y, op)
	case "PHI_CONST":
		fs.runPhiConst(x, y, op)
	case "GRANT_ONCE":
		fs.runGrantOnce(x, y, op)
	case "CONSTANT":
		fs.runConstant(x, y, op)
	case "RET", "RETURN":
		// Control-flow termination in cycle-accurate sim; functional sim treats it as a no-op.
		// (Most kernels are acyclic and verification focuses on dataflow semantics.)
		return
	case "NOP":
		// No-op: do nothing
	default:
		// Unknown opcode: silently skip
	}
}

// runMov implements MOV opcode: copy data between registers/ports
// Semantics from core/emu.go runMov handler
func (fs *FunctionalSimulator) runMov(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) == 0 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src := &op.SrcOperands.Operands[0]
	dst := &op.DstOperands.Operands[0]

	// Read source value
	srcData := fs.readOperand(x, y, src)

	// Write to destination
	fs.writeOperand(x, y, dst, srcData)
}

// runAdd implements ADD opcode: integer addition
// Semantics: dst = src0 + src1
func (fs *FunctionalSimulator) runAdd(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	// Compute sum (propagate predicate: false if any input invalid)
	result := core.NewScalarWithPred(src0.First()+src1.First(), src0.Pred && src1.Pred)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runSub implements SUB opcode: integer subtraction
func (fs *FunctionalSimulator) runSub(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	result := core.NewScalarWithPred(src0.First()-src1.First(), src0.Pred && src1.Pred)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runMul implements MUL opcode: integer multiplication
func (fs *FunctionalSimulator) runMul(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	result := core.NewScalarWithPred(src0.First()*src1.First(), src0.Pred && src1.Pred)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runDiv implements DIV opcode: integer division
func (fs *FunctionalSimulator) runDiv(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	var result core.Data
	if src1.First() != 0 {
		result = core.NewScalarWithPred(src0.First()/src1.First(), src0.Pred && src1.Pred)
	} else {
		result = core.NewScalarWithPred(0, false) // Division by zero: invalid result
	}

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runFAdd implements FADD opcode: floating-point addition
func (fs *FunctionalSimulator) runFAdd(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	// Convert to float32
	f0 := math.Float32frombits(src0.First())
	f1 := math.Float32frombits(src1.First())

	result := core.NewScalarWithPred(
		math.Float32bits(f0+f1),
		src0.Pred && src1.Pred,
	)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runFSub implements FSUB opcode: floating-point subtraction
func (fs *FunctionalSimulator) runFSub(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	f0 := math.Float32frombits(src0.First())
	f1 := math.Float32frombits(src1.First())

	result := core.NewScalarWithPred(
		math.Float32bits(f0-f1),
		src0.Pred && src1.Pred,
	)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runFMul implements FMUL opcode: floating-point multiplication
func (fs *FunctionalSimulator) runFMul(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	f0 := math.Float32frombits(src0.First())
	f1 := math.Float32frombits(src1.First())

	result := core.NewScalarWithPred(
		math.Float32bits(f0*f1),
		src0.Pred && src1.Pred,
	)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runFDiv implements FDIV opcode: floating-point division
func (fs *FunctionalSimulator) runFDiv(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	f0 := math.Float32frombits(src0.First())
	f1 := math.Float32frombits(src1.First())

	var result core.Data
	if f1 != 0 {
		result = core.NewScalarWithPred(
			math.Float32bits(f0/f1),
			src0.Pred && src1.Pred,
		)
	} else {
		result = core.NewScalarWithPred(0, false) // Division by zero
	}

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runPhi implements PHI opcode: select one of multiple inputs
// Semantics: PHI selects first ready source (functional model)
func (fs *FunctionalSimulator) runPhi(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) == 0 || len(op.DstOperands.Operands) == 0 {
		return
	}

	// Select the first ready source (predicate=true)
	var selectedData core.Data
	found := false

	for _, src := range op.SrcOperands.Operands {
		data := fs.readOperand(x, y, &src)
		if data.Pred {
			selectedData = data
			found = true
			break
		}
	}

	if !found {
		// No source ready: output is invalid
		selectedData = core.NewScalarWithPred(0, false)
	}

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, selectedData)
}

// runGpred implements GPRED opcode: grant predicate (pass through)
// Semantics: consume predicate, output is valid if input predicate is true
func (fs *FunctionalSimulator) runGpred(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) == 0 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src := fs.readOperand(x, y, &op.SrcOperands.Operands[0])

	// Output propagates the input predicate
	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, src)
}

// runLoad implements LOAD opcode: load from memory
// Semantics: address from src, load from memory, store in dst
func (fs *FunctionalSimulator) runLoad(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 1 || len(op.DstOperands.Operands) < 1 {
		return
	}

	// Source operand is the address
	addrData := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	addr := addrData.First()

	// Load from memory
	value := fs.peStates[y][x].ReadMemory(addr)

	// Write to destination register
	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, value)
}

// runStore implements STORE opcode: store to memory
// Semantics: value from src[0], address from src[1], store to memory
func (fs *FunctionalSimulator) runStore(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 {
		return
	}

	// Source operands: typically [value, address] or [address, value]
	// For simplicity, assume first is value, second is address
	valueData := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	addrData := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	addr := addrData.First()
	fs.peStates[y][x].WriteMemory(addr, valueData)
}

// runGep implements GEP opcode: compute address (base + index)
// Semantics: dst = src[0] + src[1]
func (fs *FunctionalSimulator) runGep(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	base := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	index := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	result := core.NewScalarWithPred(
		base.First()+index.First(),
		base.Pred && index.Pred,
	)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runLLS implements LLS (Logical Left Shift) and SHL (alias)
// Semantics: dst = src0 << src1
func (fs *FunctionalSimulator) runLLS(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	result := core.NewScalarWithPred(
		src0.First()<<src1.First(),
		src0.Pred && src1.Pred,
	)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runLRS implements LRS (Logical Right Shift)
// Semantics: dst = src0 >> src1
func (fs *FunctionalSimulator) runLRS(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	result := core.NewScalarWithPred(
		src0.First()>>src1.First(),
		src0.Pred && src1.Pred,
	)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runOr implements OR (bitwise OR)
// Semantics: dst = src0 | src1
func (fs *FunctionalSimulator) runOr(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	result := core.NewScalarWithPred(
		src0.First()|src1.First(),
		src0.Pred && src1.Pred,
	)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runXor implements XOR (bitwise XOR)
// Semantics: dst = src0 ^ src1
func (fs *FunctionalSimulator) runXor(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	result := core.NewScalarWithPred(
		src0.First()^src1.First(),
		src0.Pred && src1.Pred,
	)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runAnd implements AND (bitwise AND)
// Semantics: dst = src0 & src1
func (fs *FunctionalSimulator) runAnd(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	result := core.NewScalarWithPred(
		src0.First()&src1.First(),
		src0.Pred && src1.Pred,
	)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runNot implements NOT (logical NOT)
// Semantics: dst = src == 0 ? 1 : 0
func (fs *FunctionalSimulator) runNot(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) == 0 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src := fs.readOperand(x, y, &op.SrcOperands.Operands[0])

	var result uint32
	if src.First() == 0 {
		result = 1
	} else {
		result = 0
	}

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, core.NewScalarWithPred(result, src.Pred))
}

// runIcmp implements integer comparison operations
// Semantics: ICMP_EQ, ICMP_SGT, ICMP_SLT, ICMP_SGE, ICMP_SLE, ICMP_SNE
func (fs *FunctionalSimulator) runIcmp(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	val0 := int32(src0.First())
	val1 := int32(src1.First())

	var result uint32
	switch strings.ToUpper(op.OpCode) {
	case "ICMP_EQ":
		if val0 == val1 {
			result = 1
		}
	case "ICMP_SGT":
		if val0 > val1 {
			result = 1
		}
	case "ICMP_SLT":
		if val0 < val1 {
			result = 1
		}
	case "ICMP_SGE":
		if val0 >= val1 {
			result = 1
		}
	case "ICMP_SLE":
		if val0 <= val1 {
			result = 1
		}
	case "ICMP_SNE":
		if val0 != val1 {
			result = 1
		}
	}

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, core.NewScalarWithPred(result, src0.Pred && src1.Pred))
}

// runLtEx implements LT_EX (Less Than Export)
// Semantics: dst = (src0 < src1) ? 1 : 0
func (fs *FunctionalSimulator) runLtEx(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	var result uint32
	if src0.First() < src1.First() {
		result = 1
	}

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, core.NewScalarWithPred(result, src0.Pred && src1.Pred))
}

// runSext implements SEXT (Sign Extend)
// For now, pass-through since we work with 32-bit values
func (fs *FunctionalSimulator) runSext(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) == 0 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src := fs.readOperand(x, y, &op.SrcOperands.Operands[0])

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, src)
}

// runZext implements ZEXT (Zero Extend)
// For now, pass-through since we work with 32-bit values
func (fs *FunctionalSimulator) runZext(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) == 0 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src := fs.readOperand(x, y, &op.SrcOperands.Operands[0])

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, src)
}

// runCastFptoSi implements CAST_FPTOSI (Float to Signed Integer)
// Semantics: dst = (int32)src (convert float32 bits to signed int)
func (fs *FunctionalSimulator) runCastFptoSi(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) == 0 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src := fs.readOperand(x, y, &op.SrcOperands.Operands[0])

	// Convert float32 bits to float32, then to int32
	srcFloat := math.Float32frombits(src.First())
	srcInt := int32(srcFloat)

	result := core.NewScalarWithPred(uint32(srcInt), src.Pred)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runFMulFAdd implements FMUL_FADD (Fused Multiply-Add)
// Semantics: dst = (src0 * src1) + src2 (float operations)
func (fs *FunctionalSimulator) runFMulFAdd(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 3 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])
	src2 := fs.readOperand(x, y, &op.SrcOperands.Operands[2])

	// Convert to float32
	f0 := math.Float32frombits(src0.First())
	f1 := math.Float32frombits(src1.First())
	f2 := math.Float32frombits(src2.First())

	// Fused multiply-add
	resultFloat := f0*f1 + f2

	result := core.NewScalarWithPred(
		math.Float32bits(resultFloat),
		src0.Pred && src1.Pred && src2.Pred,
	)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runPhiConst implements PHI_CONST (Phi with Constant)
// Semantics: First execution selects src0, subsequent executions select src1
func (fs *FunctionalSimulator) runPhiConst(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) < 2 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src0 := fs.readOperand(x, y, &op.SrcOperands.Operands[0])
	src1 := fs.readOperand(x, y, &op.SrcOperands.Operands[1])

	// For simplicity in functional simulation, select first ready source
	var selectedData core.Data
	if src0.Pred {
		selectedData = src0
	} else if src1.Pred {
		selectedData = src1
	} else {
		selectedData = core.NewScalarWithPred(0, false)
	}

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, selectedData)
}

// runGrantOnce implements GRANT_ONCE (Grant Once)
// Semantics: Pass through source value with predicate=true
func (fs *FunctionalSimulator) runGrantOnce(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) == 0 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src := fs.readOperand(x, y, &op.SrcOperands.Operands[0])

	// GRANT_ONCE sets predicate=true (valid mark)
	result := core.NewScalarWithPred(src.First(), true)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// runConstant implements CONSTANT (Constant generation)
// Semantics: Output the constant value with predicate=true
func (fs *FunctionalSimulator) runConstant(x, y int, op *core.Operation) {
	if len(op.SrcOperands.Operands) == 0 || len(op.DstOperands.Operands) == 0 {
		return
	}

	src := fs.readOperand(x, y, &op.SrcOperands.Operands[0])

	// CONSTANT always outputs with predicate=true
	result := core.NewScalarWithPred(src.First(), true)

	dst := &op.DstOperands.Operands[0]
	fs.writeOperand(x, y, dst, result)
}

// readOperand reads a value from an operand (register, immediate, or port)
func (fs *FunctionalSimulator) readOperand(x, y int, operand *core.Operand) core.Data {
	if strings.HasPrefix(operand.Impl, "$") {
		// Register operand
		regIdx, err := strconv.Atoi(strings.TrimPrefix(operand.Impl, "$"))
		if err != nil {
			return core.NewScalarWithPred(0, false)
		}
		return fs.peStates[y][x].ReadReg(regIdx)
	}

	if strings.HasPrefix(operand.Impl, "#") {
		// Immediate operand
		val, err := strconv.ParseUint(strings.TrimPrefix(operand.Impl, "#"), 10, 32)
		if err != nil {
			return core.NewScalarWithPred(0, false)
		}
		return core.NewScalar(uint32(val))
	}

	if isPortOperand(operand.Impl) {
		// Port operand: read from neighbor
		return fs.readFromNeighbor(x, y, operand.Impl)
	}

	return core.NewScalarWithPred(0, false)
}

// writeOperand writes a value to an operand (register or port)
func (fs *FunctionalSimulator) writeOperand(x, y int, operand *core.Operand, data core.Data) {
	if strings.HasPrefix(operand.Impl, "$") {
		// Register operand
		regIdx, err := strconv.Atoi(strings.TrimPrefix(operand.Impl, "$"))
		if err != nil {
			return
		}
		fs.peStates[y][x].WriteReg(regIdx, data)
	}

	// Port operand: write to neighbor (simplified: just track locally)
	// In a real simulator, this would route through network
	// For functional sim, we ignore this for now
}

// readFromNeighbor reads a value from a neighbor PE's port
func (fs *FunctionalSimulator) readFromNeighbor(x, y int, portDir string) core.Data {
	// In functional model, we don't model network delays or buffering
	// For now, return default value (in practice, would fetch from neighbor)
	return core.NewScalarWithPred(0, false)
}
