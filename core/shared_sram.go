package core

// SharedSRAMAccessor provides direct timing and storage access for banked
// shared SRAM scratchpads.
type SharedSRAMAccessor interface {
	ScheduleCycleForAddress(addr uint64, issueCycle int64) int64
	ReadStorage(addr uint32, size uint64) ([]byte, error)
	WriteStorage(addr uint32, data []byte) error
}
