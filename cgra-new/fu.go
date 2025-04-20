package cgra

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/zeonica/confignew"
	operand_impl "github.com/sarchlab/zeonica/operand-impl"
	"github.com/sarchlab/zeonica/program"
)

type FuncUnit struct {
	*sim.TickingComponent

	//ports map[cgra.Side]*portPair

	internalInfo map[int]func()
	// a map to store internal information, like the coordinates of the core in mesh.

	freq    sim.Freq
	binding confignew.IDImplBinding
	// This is a binding that store the mapping between the unique operand representation (i.e. STATIC ID in Zeonica)
	// and the actual operand implementation (i.e. the buffer, the register, etc.)

	program program.Program
	regfile []operand_impl.URegister

	//state coreState
	//emu   in
}

func (fu *FuncUnit) SetProgram(program program.Program) {
	fu.program = program
}

func (fu *FuncUnit) SetBinding(binding confignew.IDImplBinding) {
	fu.binding = binding
}

func (fu *FuncUnit) AddRegister() {
	fu.regfile = append(fu.regfile, operand_impl.URegister{})
}

func (fu *FuncUnit) GetRegister(id int) *operand_impl.URegister {
	return &fu.regfile[id]
}

func (fu *FuncUnit) GetRegisterCount() int {
	return len(fu.regfile)
}
