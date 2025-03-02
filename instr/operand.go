package instr

type Operand struct {
	DynamicID []int         // dynamicID
	StaticID  int           // staticID
	Impl      AsOperandImpl // an implementation of the operand
}

type AsOperandImpl interface {
	Retrieve() interface{} // retrieve value from the source, type is not defined.
	Push(interface{})      // push value to the source
	AddressWrite(int)      // write the address to the source, if supported.
	// THE SIGNATURE OF THE FUNCTION ABOVE NEED SOME MORE ADJUSTMENT
	// Should Pass DynamicID IN
}
