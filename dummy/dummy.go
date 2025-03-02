// This is a dummy package to co-operate. The dummy impls will be replaced by the real impls in the future.
package dummy

import (
	"runtime/debug"

	"github.com/sarchlab/zeonica/confignew"
	"github.com/sarchlab/zeonica/instr"
)

func DummyCreateAndRegiTile() *DummyTile {
	return new(DummyTile)
}

func DummyCreateAndRegiBuffer() *DummyBuffer {
	return new(DummyBuffer)
}

type DummyBuffer struct {
}

func (d DummyBuffer) Retrieve() interface{} {
	return nil
}

func (d DummyBuffer) Push(interface{}) {
}

func (d DummyBuffer) AddressWrite(addr int) {

}

func NewDummyBuffer() DummyBuffer {
	return DummyBuffer{}
}

type DummyTile struct {
	IDbinding confignew.IDImplBinding
	instList  []instr.CombinedInst
}

func NewDummyTile() DummyTile {
	return DummyTile{
		IDbinding: *confignew.NewIDImplBinding(),
	}
}

func (t DummyTile) FindImpl(config confignew.IDImplBinding) {
	for _, cInst := range t.instList {
		for _, inst := range cInst.Insts {
			for _, operand := range inst.DstOperands {
				operand.Impl = t.IDbinding.Lookup(operand.StaticID)
			}
			for _, operand := range inst.SrcOperands {
				operand.Impl = t.IDbinding.Lookup(operand.StaticID)
			}
		}
	}
}

type DummyRegister struct {
}

func (d DummyRegister) retrieve() interface{} {
	return nil
}

func (d DummyRegister) push(interface{}) {

}

func (d DummyRegister) addressWrite(int) {

}

// This is a protection impl of AsOperand, which will arise panic when it is called.
type NonExist struct {
}

func (n NonExist) Retrieve() interface{} {
	debug.PrintStack()
	panic("FATAL: NonExist impl retrieve() is called, please check your code.")
}

func (n NonExist) Push(interface{}) {
	debug.PrintStack()
	panic("FATAL: NonExist impl push() is called, please check your code.")
}

func (n NonExist) AddressWrite(int) {
	debug.PrintStack()
	panic("FATAL: NonExist impl addressWrite() is called, please check your code.")
}

var NonExistComponent = NonExist{}
