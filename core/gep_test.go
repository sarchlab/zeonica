package core

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/zeonica/cgra"
)

var _ = Describe("GEP", func() {
	var (
		ie instEmulator
		s  coreState
	)

	reg := func(name string) Operand { return Operand{Color: "RED", Impl: name} }

	gep := func(srcs ...string) Operation {
		ops := make([]Operand, 0, len(srcs))
		for _, src := range srcs {
			ops = append(ops, reg(src))
		}
		return Operation{
			OpCode:      "GEP",
			ID:          7,
			SrcOperands: OperandList{Operands: ops},
			DstOperands: OperandList{Operands: []Operand{reg("$5")}},
		}
	}

	BeforeEach(func() {
		ie = instEmulator{CareFlags: true}
		exit := false
		retVal := uint32(0)
		s = coreState{
			exit:      &exit,
			retVal:    &retVal,
			Registers: make([]cgra.Data, 8),
			States:    make(map[string]interface{}),
		}
		s.Registers[0] = cgra.NewScalarWithPred(16, true) // base
		s.Registers[1] = cgra.NewScalarWithPred(3, true)  // index0 (row)
		s.Registers[2] = cgra.NewScalarWithPred(2, true)  // index1 (column)
	})

	It("adds base and index for two operands", func() {
		out := ie.runGep(gep("$0", "$1"), &s)
		Expect(out[reg("$5")].First()).To(Equal(uint32(19)))
		Expect(out[reg("$5")].Pred).To(BeTrue())
	})

	It("applies the configured row stride for three operands", func() {
		s.GepStride = 4
		out := ie.runGep(gep("$0", "$1", "$2"), &s)
		Expect(out[reg("$5")].First()).To(Equal(uint32(16 + 3*4 + 2)))
		Expect(out[reg("$5")].Pred).To(BeTrue())
	})

	It("propagates a false predicate from the third operand", func() {
		s.GepStride = 4
		s.Registers[2] = cgra.NewScalarWithPred(2, false)
		out := ie.runGep(gep("$0", "$1", "$2"), &s)
		Expect(out[reg("$5")].Pred).To(BeFalse())
	})

	It("panics when a three-operand GEP has no stride configured", func() {
		Expect(func() { ie.runGep(gep("$0", "$1", "$2"), &s) }).To(Panic())
	})
})
