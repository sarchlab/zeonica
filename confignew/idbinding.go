package confignew

import (
	"github.com/sarchlab/zeonica/instr"
)

// ID assume the array is homogeneous

// NameIDBinding is a struct that binds a name with an ID. Definiton of ID please refer to http://www.google.com
type NameIDBinding struct {
	// distributed is the number of IDs that have been distributed
	distributed int
	// nameToID is a map that maps a name to an ID
	nameToID map[string]int
	// IDToName is a map that maps an ID to a name
	IDToName map[int]string
}

// Constructor for NameIDBinding
func NewNameIDBinding() *NameIDBinding {
	return &NameIDBinding{
		distributed: 0,
		nameToID:    make(map[string]int),
		IDToName:    make(map[int]string),
	}
}

// registerName registers a name and returns the ID of the name
func (n NameIDBinding) RegisterName(name string) int {
	n.nameToID[name] = n.distributed
	n.IDToName[n.distributed] = name
	n.distributed++
	return n.distributed - 1
}

// bindNameAndID binds a name with an arbitrary ID. Users basically should not use this function.
func (n NameIDBinding) bindNameAndID(name string, id int) {
	n.nameToID[name] = id
	n.IDToName[id] = name
}

// bindRegisterFile binds a name with a register file. The first register is $0.
func (binding NameIDBinding) BindRegisterFile(sum int) int {
	first := 0
	first = binding.RegisterName("$0")
	for i := 0; i < sum; i++ {
		binding.RegisterName("$" + string(i))
	}
	return first
}

// IDImplBinding is a struct that binds an ID with an implementation. Every core has its own IDImplBinding.
type IDImplBinding struct {
	IDToImpl map[int](instr.AsOperandImpl)
	ImplToID map[instr.AsOperandImpl]int
}

func (i IDImplBinding) BindIDAndImpl(id int, impl instr.AsOperandImpl) {
	i.IDToImpl[id] = impl
	i.ImplToID[impl] = id
}

// Constructor for IDImplBinding
func NewIDImplBinding() *IDImplBinding {
	return &IDImplBinding{
		IDToImpl: make(map[int](instr.AsOperandImpl)),
		ImplToID: make(map[instr.AsOperandImpl]int),
	}
}

func (i IDImplBinding) Lookup(id int) instr.AsOperandImpl {
	return i.IDToImpl[id]
}
