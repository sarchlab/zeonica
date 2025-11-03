package core

// Opcode represents the operation code for an instruction
type Opcode string

// Instruction represents a structured IR instruction
type Instruction struct {
	Opcode   Opcode   // The operation to perform
	Operands []string // Operands as strings (for gradual migration)
	Label    string   // Non-empty for label lines (e.g., "IF"), empty for regular instructions
	Raw      string   // Raw string representation for fallback to old logic
}
