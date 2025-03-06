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

func (i Inst) Execute(args ...interface{}) { // OPTIONAL： move the execute function to the Core
	switch behavior := i.behavior.(type) {
	case func(int, int) int:
		if len(args) != 2 {
			panic("RUNTIME ERROR: Inst" + i.rawtxt + " expects 2 arguments, but " + fmt.Sprintf("%d", len(args)) + " arguments are provided.")
		}
		// WRONG HERE! Data are fetched repeatedly.
		var res = behavior(i.SrcOperands[0].Impl.Retrieve(i.SrcOperands[0].DynamicID).(int), i.SrcOperands[1].Impl.Retrieve(i.SrcOperands[1].DynamicID).(int))
		// if i.DstOperands[0].DynamicID == "SelfX" ... User-define behaviour
		i.DstOperands[0].Impl.Push(i.DstOperands[0].DynamicID, res)
	default:
		println("FATAL: Unsupported behavior type.")
	}
}

type CombinedInst struct {
	Insts   []Inst
	IsEntry bool // If the core is datadriven, then represent if the instruction is an entry instruction.
}

func (c CombinedInst) Run() {
	if !c.CheckReady() {
		return
	}
	for _, inst := range c.Insts {
		inst.Execute()
	}
}

func (c CombinedInst) String() string {
	return fmt.Sprintf("CombinedInst{insts: %v}", c.Insts)
}

func (c CombinedInst) CheckReady() bool {
	for _, i := range c.Insts {
		for _, src := range i.SrcOperands {
			if src.Predicate && !src.Impl.ReadyRead(src.DynamicID) {
				return false
			}
		}
		for _, dst := range i.DstOperands {
			if dst.Predicate && !dst.Impl.ReadyWrite(dst.DynamicID) {
				return false
			}
		}
	}
	return true
}

/*
	{
	    MAC, 1, [RED, EAST_PORT], [BLUE, SOUTH_PORT]!, [$0], [$1]
	    MOV, [BLUE, SOUTH_PORT], [BLUE, NORTH_PORT]!
	}

	STATIC ID:
	$0 = 0
	$1 = 1
	EAST_PORT = 35
	SOUTH_PORT = 33
	NORTH_PORT = 32

	DYNAMIC ID:
	RED = 0
	YELLOW = 1
	BLUE = 2
	GREEN = 3

*/

/*
func instMAC(src0 float32, src1 float32, src2 float32) float32 {
	return src0*src1 + src2
}

var testInstMac = Inst{
	rawtxt:   "MAC, 1, [EAST_PORT], [SOUTH_PORT]!, [$0], [$1]",
	behavior: instMAC,
	DstOperands: []Operand{
		Operand{
			StaticID:  35,
			DynamicID: []int{0},
			Impl:      nil,
			Predicate: false,
		},
	},
	SrcOperands: []Operand{
		Operand{
			StaticID:  33,
			DynamicID: []int{2},
			Impl:      nil,
			Predicate: true,
		},
		Operand{
			StaticID:  0,
			DynamicID: []int{0}, // Just placeholder
			Impl:      nil,
			Predicate: false,
		},
		Operand{
			StaticID:  1,
			DynamicID: []int{1}, // Just placeholder
			Impl:      nil,
			Predicate: false,
		},
	},
}

var testInstMov = Inst{
	rawtxt: "MOV, [BLUE, SOUTH_PORT], [BLUE, NORTH_PORT]!",
	behavior: func(src float32) float32 {
		return src
	},
	DstOperands: []Operand{
		Operand{
			StaticID:  32,
			DynamicID: []int{2},
			Impl:      nil,
			Predicate: true,
		},
	},
	SrcOperands: []Operand{
		Operand{
			StaticID:  33,
			DynamicID: []int{2},
			Impl:      nil,
			Predicate: true,
		},
	},
}

var testCombinedInst = CombinedInst{
	Insts:   []Inst{testInstMac, testInstMov},
	IsEntry: true,
}
*/
