package instr

type Operand struct {
	DynamicID []int         // dynamicID
	StaticID  int           // staticID
	Predicate bool          // predicate
	Impl      AsOperandImpl // an implementation of the operand
}

type AsOperandImpl interface {
	Retrieve([]int) interface{}  // retrieve value from the source, type is not defined.
	Push([]int, interface{})     // push value to the source
	AddressWrite(int)            // write the address to the source, if supported.
	AddressRead(int) interface{} // read the address from the source, if supported.
	// THE SIGNATURE OF THE FUNCTION ABOVE NEED SOME MORE ADJUSTMENT
	// Should Pass DynamicID IN
	ReadyRead([]int) bool // check if the operand is ready
	ReadyWrite([]int) bool
}

// Example： Colored Buffer

/*

type ColoredBuffer struct {
	red      []interface{} // 0
	yellow   []interface{} // 1
	blue     []interface{} // 2
	green    []interface{} // 3
	capacity int
}


func NewColoredBuffer(capacity int) ColoredBuffer {
	return ColoredBuffer{
		red:      make([]interface{}, 0),
		yellow:   make([]interface{}, 0),
		blue:     make([]interface{}, 0),
		green:    make([]interface{}, 0),
		capacity: capacity,
	}
}

func (c ColoredBuffer) Retrieve(dynamicID []int) interface{} {
	if len(dynamicID) != 1 { // TODO: encapsulate the check, with GlobalConfig.dynamicIDLength
		panic("The length of dynamicID is not correct.")
	}
	if dynamicID == nil { // do not require the color
		if len(c.red) > 0 {
			res := c.red[0]
			c.red = c.red[1:]
			return res
		}
		if len(c.yellow) > 0 {
			res := c.yellow[0]
			c.yellow = c.yellow[1:]
			return res
		}
		if len(c.blue) > 0 {
			res := c.blue[0]
			c.blue = c.blue[1:]
			return res
		}
		if len(c.green) > 0 {
			res := c.green[0]
			c.green = c.green[1:]
			return res
		}
	} else {
		switch dynamicID[0] {
		case 0:
			if len(c.red) > 0 {
				res := c.red[0]
				c.red = c.red[1:]
				return res
			} else {
				return 0
			}
		case 1:
			if len(c.yellow) > 0 {
				res := c.yellow[0]
				c.yellow = c.yellow[1:]
				return res
			} else {
				return 0
			}
		case 2:
			if len(c.blue) > 0 {
				res := c.blue[0]
				c.blue = c.blue[1:]
				return res
			} else {
				return 0
			}
		case 3:
			if len(c.green) > 0 {
				res := c.green[0]
				c.green = c.green[1:]
				return res
			} else {
				return 0
			}
		}
	}
	return 0
}

func (c ColoredBuffer) Push(dynamicID []int, value interface{}) {
	if len(dynamicID) != 1 {
		panic("The length of dynamicID is not correct.")
	}
	if dynamicID == nil {
		if len(c.red) < c.capacity {
			c.red = append(c.red, value)

		} else if len(c.yellow) < c.capacity {
			c.yellow = append(c.yellow, value)
		} else if len(c.blue) < c.capacity {
			c.blue = append(c.blue, value)
		} else if len(c.green) < c.capacity {
			c.green = append(c.green, value)
		} else {
			c.red = append(c.red[1:], value)
		}
		return
	}
	switch dynamicID[0] {
	case 0:
		if len(c.red) < c.capacity {
			c.red = append(c.red, value)
		} else {
			c.red = append(c.red[1:], value)
		}
	case 1:
		if len(c.yellow) < c.capacity {
			c.yellow = append(c.yellow, value)
		} else {
			c.yellow = append(c.yellow[1:], value)
		}
	case 2:
		if len(c.blue) < c.capacity {
			c.blue = append(c.blue, value)
		} else {
			c.blue = append(c.blue[1:], value)
		}
	case 3:
		if len(c.green) < c.capacity {
			c.green = append(c.green, value)
		} else {
			c.green = append(c.green[1:], value)
		}
	}
}

func (c ColoredBuffer) AddressWrite(dynamicID []int) {
}

func (c ColoredBuffer) AddressRead(dynamicID []int) interface{} {
	return nil
}

func (c ColoredBuffer) ReadyRead(dynamicID []int) bool {
	if len(dynamicID) != 1 {
		panic("The length of dynamicID is not correct.")
	}
	if dynamicID == nil {
		return len(c.red) > 0 || len(c.yellow) > 0 || len(c.blue) > 0 || len(c.green) > 0
	}
	switch dynamicID[0] {
	case 0:
		return len(c.red) > 0
	case 1:
		return len(c.yellow) > 0
	case 2:
		return len(c.blue) > 0
	case 3:
		return len(c.green) > 0
	}
	return false
}

func (c ColoredBuffer) ReadyWrite(dynamicID []int) bool {
	if len(dynamicID) != 1 {
		panic("The length of dynamicID is not correct.")
	}
	if dynamicID == nil {
		return len(c.red) < c.capacity || len(c.yellow) < c.capacity || len(c.blue) < c.capacity || len(c.green) < c.capacity
	}
	switch dynamicID[0] {
	case 0:
		return len(c.red) < c.capacity
	case 1:
		return len(c.yellow) < c.capacity
	case 2:
		return len(c.blue) < c.capacity
	case 3:
		return len(c.green) < c.capacity
	}
	return false
}

*/
