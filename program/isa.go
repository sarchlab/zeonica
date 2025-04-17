package program



// ISA is a struct that represents an Instruction Set Architecture.
type ISA struct {
	// name of the ISA.
	isaName string
	// map from instruction name to the behavior of the instruction.
	nameToBehavior map[string]interface{}
}

// Constructor for ISA.
func NewISA(name string) *ISA {
	return &ISA{
		isaName:        name,
		nameToBehavior: make(map[string]interface{}),
	}
}

// Register a new instruction to the ISA.
func (isa ISA) registerNewInst(name string, behavior interface{}) {
	isa.nameToBehavior[name] = behavior
}

var defaultISA = NewISA("Zeonica Unified ISA")

// ...

// Initialize the default ISA, call by simulatorBuilder.
func defaultISAinit() {
	defaultISA.registerNewInst("ADD", instADD)
	// ... define all the other instructions.
}
