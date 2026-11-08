package core

func ttBcastElem(kind string) func(r, c int) int {
	switch kind {
	case "h":
		return func(r, c int) int { return ttFaceIndex(0, c) }
	case "w":
		return func(r, c int) int { return ttFaceIndex(r, 0) }
	case "hw":
		return func(r, c int) int { return ttFaceIndex(0, 0) }
	}
	return ttFaceIndex
}
