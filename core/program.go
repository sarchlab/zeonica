package core

type Program struct {
	EntryBlocks []EntryBlock
}

type EntryBlock struct {
	EntryCond     OperandList
	CombinedInsts []CombinedInst
	Label         map[string]int
}

type CombinedInst struct {
	Insts []Inst
}

type Inst struct {
	// The raw text of the instruction.
	OpCode      string
	DstOperands OperandList
	SrcOperands OperandList
}

type OperandList struct {
	Operands []Operand
}

type Operand struct {
	Flag  bool
	Color string
	Impl  string
}
