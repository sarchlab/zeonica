package operand_impl

type Memory struct {
	memory []interface{}
}

func (m *Memory) Retrieve(dynID []int) interface{} {
	return m.memory[dynID[0]]
}

