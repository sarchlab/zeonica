package instr

import "fmt"

type Inst struct {
	// The raw text of the instruction.
	rawtxt      string
	behavior    interface{} // to store the behavior of the instruction
	DstOperands []Operand
	SrcOperands []Operand
	// others....
	// not add predicate !!!
}

// how to cope with load and store? mem as an special operand?

func (i Inst) Execute(args ...interface{}) {
	switch behavior := i.behavior.(type) {
	case func(int, int) int:
		if len(args) != 2 {
			panic("RUNTIME ERROR: Inst" + i.rawtxt + " expects 2 arguments, but " + fmt.Sprintf("%d", len(args)) + " arguments are provided.")
		}
		var res = behavior(i.SrcOperands[0].Impl.Retrieve().(int), i.SrcOperands[1].Impl.Retrieve().(int))
		i.DstOperands[0].Impl.Push(res)
	default:
		println("Unknown behavior type.")
	}
}

type CombinedInst struct {
	Insts []Inst
}

func (c CombinedInst) Run() {
	for _, inst := range c.Insts {
		inst.Execute()
	}
}

func (c CombinedInst) String() string {
	return fmt.Sprintf("CombinedInst{insts: %v}", c.Insts)
}
