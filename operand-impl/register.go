package operand_impl

// implement operand interface

type URegister struct {
	value uint32
}

func (r *URegister) Retrieve(dynID []int) interface{} {
	return r.value
}

func (r *URegister) Peek(addr []int) interface{} {
	return r.value
}

func (r *URegister) Push(dynID []int, value interface{}) {
	r.value = value.(uint32)
}

func (r *URegister) AddressRead(addr int) interface{} {
	return r.value
}

func (r *URegister) AddressWrite(addr int, value interface{}) {
	r.value = value.(uint32)
}

func (r *URegister) ReadyRead(dynID []int) bool {
	return true
}

func (r *URegister) ReadyWrite(dynID []int) bool {
	return true
}

type IRegister struct {
	value int
}

// Implement Operand interface
func (r *IRegister) Retrieve(dynID []int) interface{} {
	// Register ignore dynID
	// Register's retrieve and peek are the same
	return r.value
}

func (r *IRegister) Peek(addr []int) interface{} {
	return r.value
}

func (r *IRegister) Push(dynID []int, value interface{}) {
	r.value = value.(int)
}

func (r *IRegister) AddressRead(addr int) interface{} {
	return r.value
}

func (r *IRegister) AddressWrite(addr int, value interface{}) {
	r.value = value.(int)
}

func (r *IRegister) ReadyRead(dynID []int) bool {
	// Register is ready to read
	return true
}

func (r *IRegister) ReadyWrite(dynID []int) bool {
	// Register is ready to write
	return true
}

type FRegister struct {
	value float32
}

func (r *FRegister) Retrieve(dynID []int) interface{} {
	// Register ignore dynID
	// Register's retrieve and peek are the same
	return r.value
}

func (r *FRegister) Peek(addr []int) interface{} {
	return r.value
}

func (r *FRegister) Push(dynID []int, value interface{}) {
	r.value = value.(float32)
}

func (r *FRegister) AddressRead(addr int) interface{} {
	return r.value
}

func (r *FRegister) AddressWrite(addr int, value interface{}) {
	r.value = value.(float32)
}

func (r *FRegister) ReadyRead(dynID []int) bool {
	// Register is ready to read
	return true
}

func (r *FRegister) ReadyWrite(dynID []int) bool {
	// Register is ready to write
	return true
}
