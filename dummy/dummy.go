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

func (d DummyBuffer) Retrieve([]int) interface{} {
	return nil
}

func (d DummyBuffer) Push(dyn []int, data interface{}) {
}

func (d DummyBuffer) AddressWrite(addr int) {

}

func (d DummyBuffer) AddressRead(addr int) interface{} {
	return nil
}

func (d DummyBuffer) ReadyRead([]int) bool {
	return false
}

func (d DummyBuffer) ReadyWrite([]int) bool {
	return false
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

func (d DummyRegister) Retrieve([]int) interface{} {
	return nil
}

func (d DummyRegister) Push(interface{}) {

}

func (d DummyRegister) AddressWrite(int) {

}

func (d DummyRegister) AddressRead(int) interface{} {
	return nil
}

// This is a protection impl of AsOperand, which will arise panic when it is called.
type NonExist struct {
}

func (n NonExist) Retrieve([]int) interface{} {
	debug.PrintStack()
	panic("FATAL: NonExist impl retrieve() is called, please check your code.")
}

func (n NonExist) Push([]int, interface{}) {
	debug.PrintStack()
	panic("FATAL: NonExist impl push() is called, please check your code.")
}

func (n NonExist) AddressWrite(int) {
	debug.PrintStack()
	panic("FATAL: NonExist impl addressWrite() is called, please check your code.")
}

func (n NonExist) AddressRead(int) interface{} {
	debug.PrintStack()
	panic("FATAL: NonExist impl addressRead() is called, please check your code.")
}

func (n NonExist) ReadyRead([]int) bool {
	debug.PrintStack()
	panic("FATAL: NonExist impl readyRead() is called, please check your code.")
}

func (n NonExist) ReadyWrite([]int) bool {
	debug.PrintStack()
	panic("FATAL: NonExist impl readyWrite() is called, please check your code.")
}

var NonExistComponent = NonExist{}
