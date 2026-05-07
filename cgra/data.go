package cgra

// Data is the value container transferred between CGRA components.
type Data struct {
	Data []uint32
	Pred bool
}

// NewScalar creates a Data that wraps a single uint32 value with Pred=true by default.
func NewScalar(v uint32) Data {
	return Data{Data: []uint32{v}, Pred: true}
}

// NewScalarWithPred creates a scalar Data value with an explicit predicate.
func NewScalarWithPred(v uint32, pred bool) Data {
	return Data{Data: []uint32{v}, Pred: pred}
}

// First returns the first lane value. If empty, returns 0.
func (d Data) First() uint32 {
	if len(d.Data) == 0 {
		return 0
	}
	return d.Data[0]
}

// LaneCount returns the number of lanes carried by this value.
func (d Data) LaneCount() int {
	return len(d.Data)
}

// IsScalar reports whether the value is a single-lane scalar.
func (d Data) IsScalar() bool {
	return len(d.Data) <= 1
}

// Clone returns a deep copy of the value container.
func (d Data) Clone() Data {
	if len(d.Data) == 0 {
		return Data{Pred: d.Pred}
	}
	cloned := make([]uint32, len(d.Data))
	copy(cloned, d.Data)
	return Data{Data: cloned, Pred: d.Pred}
}

// WithPred returns a copy with the given predicate flag.
func (d Data) WithPred(pred bool) Data {
	d.Pred = pred
	return d
}

// FromSlice constructs a Data from a slice and optional predicate.
func FromSlice(vals []uint32, pred bool) Data {
	if len(vals) == 0 {
		return Data{Pred: pred}
	}
	cloned := make([]uint32, len(vals))
	copy(cloned, vals)
	return Data{Data: cloned, Pred: pred}
}
