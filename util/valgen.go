// Some helpers using closures to generate values
package valgen

func MakeConstGen(constant int) func() int {
	return func() int {
		return constant
	}
}

func MakeIncreasingGen(start int) func() int {
	current := start
	return func() int {
		current++
		return current
	}
}
