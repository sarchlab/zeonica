package core

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/zeonica/cgra"
)

var _ = Describe("REM", func() {
	var (
		ie instEmulator
		s  coreState
	)

	reg := func(name string) Operand { return Operand{Color: "RED", Impl: name} }

	rem := Operation{
		OpCode:      "REM",
		SrcOperands: OperandList{Operands: []Operand{reg("$0"), reg("$1")}},
		DstOperands: OperandList{Operands: []Operand{reg("$2")}},
	}

	set := func(a, b int32) {
		s.Registers[0] = cgra.NewScalarWithPred(uint32(a), true)
		s.Registers[1] = cgra.NewScalarWithPred(uint32(b), true)
	}

	BeforeEach(func() {
		ie = instEmulator{CareFlags: true}
		exit := false
		retVal := uint32(0)
		s = coreState{
			exit:      &exit,
			retVal:    &retVal,
			Registers: make([]cgra.Data, 4),
			States:    make(map[string]interface{}),
		}
	})

	It("computes the signed remainder", func() {
		set(4199, 70)
		Expect(ie.runRem(rem, &s)[reg("$2")].First()).To(Equal(uint32(69)))
	})

	It("keeps the sign of the dividend like LLVM srem", func() {
		set(-7, 3)
		Expect(int32(ie.runRem(rem, &s)[reg("$2")].First())).To(Equal(int32(-1)))
	})

	It("panics on remainder by zero", func() {
		set(5, 0)
		Expect(func() { ie.runRem(rem, &s) }).To(Panic())
	})
})
