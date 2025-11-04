package core

import (
	"fmt"
	"strings"
)

// RunInstIR executes a structured IR instruction
func (i instEmulator) RunInstIR(inst Instruction, state *coreState) {
	// If this is a label, just increment PC and return
	if inst.Label != "" {
		state.PC++
		return
	}

	// Switch on opcode for IR-handled instructions
	switch inst.Opcode {
	case "PHI":
		i.runPhiIR(inst, state)
	default:
		// Fallback to old string-based execution
		i.RunInst(inst.Raw, state)
	}
}

// runPhiIR implements PHI instruction based on predecessor blocks
// Syntax: PHI, $dst, $v1@BlockA, $v2@BlockB, ...
func (i instEmulator) runPhiIR(inst Instruction, state *coreState) {
	if len(inst.Operands) < 2 {
		panic(fmt.Sprintf("PHI instruction requires at least 2 operands (dst and one incoming), got %d", len(inst.Operands)))
	}

	dst := inst.Operands[0]
	if !strings.HasPrefix(dst, "$") {
		panic(fmt.Sprintf("PHI destination must be a register (e.g., $0), got %s", dst))
	}

	// Get the predecessor block that led to this PHI
	predBlock := state.LastPredBlock

	// Parse each incoming value and find the one matching the predecessor
	var selectedValue uint32
	foundMatch := false

	for idx := 1; idx < len(inst.Operands); idx++ {
		incoming := strings.TrimSpace(inst.Operands[idx])

		// Parse format: $reg@BlockLabel
		parts := strings.Split(incoming, "@")
		if len(parts) != 2 {
			panic(fmt.Sprintf("PHI incoming operand must be in format $reg@BlockLabel, got %s", incoming))
		}

		srcReg := strings.TrimSpace(parts[0])
		blockLabel := strings.TrimSpace(parts[1])

		if !strings.HasPrefix(srcReg, "$") {
			panic(fmt.Sprintf("PHI incoming register must start with $, got %s", srcReg))
		}

		// Check if this incoming matches our predecessor block
		if blockLabel == predBlock {
			// Read the value from the source register
			selectedValue = i.readOperand(srcReg, state)
			foundMatch = true
			break
		}
	}

	if !foundMatch {
		// No matching predecessor found for PHI; this indicates a control flow or PHI construction error.
		// Collect available block labels from the PHI operands for clearer debugging.
		var blockLabels []string
		for idx := 1; idx < len(inst.Operands); idx++ {
			parts := strings.Split(strings.TrimSpace(inst.Operands[idx]), "@")
			if len(parts) == 2 {
				blockLabels = append(blockLabels, strings.TrimSpace(parts[1]))
			}
		}
		panic(fmt.Sprintf("PHI: no incoming value matches predecessor block '%s' at PC %d; available block labels: %v",
			predBlock, state.PC, blockLabels))
	}

	// Write the selected value to destination
	i.writeOperand(dst, selectedValue, state)
	state.PC++
}
