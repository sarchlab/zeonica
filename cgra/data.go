package cgra

// TokenID uniquely identifies a data token flowing through the CGRA
// This type is defined in cgra package to avoid circular imports
type TokenID uint64

type Data struct {
	Data    []uint32
	Pred    bool
	TokenID TokenID // Unique identifier for tracking this token through the network
	EOS     bool    // End of Stream flag - indicates this is the last data item in the stream
}

// NewScalar creates a Data that wraps a single uint32 value with Pred=true by default.
func NewScalar(v uint32) Data {
	return Data{Data: []uint32{v}, Pred: true, EOS: false}
}

// NewScalarWithPred creates a Data that wraps a single uint32 value with the given predicate.
func NewScalarWithPred(v uint32, pred bool) Data {
	return Data{Data: []uint32{v}, Pred: pred, EOS: false}
}

// NewScalarWithEOS creates a Data with EOS flag set.
func NewScalarWithEOS(v uint32, pred bool, eos bool) Data {
	return Data{Data: []uint32{v}, Pred: pred, EOS: eos}
}

// First returns the first lane value. If empty, returns 0.
func (d Data) First() uint32 {
	if len(d.Data) == 0 {
		return 0
	}
	return d.Data[0]
}

// WithPred returns a copy with the given predicate flag.
func (d Data) WithPred(pred bool) Data {
	d.Pred = pred
	return d
}

// WithEOS returns a copy with the given EOS flag.
func (d Data) WithEOS(eos bool) Data {
	d.EOS = eos
	return d
}

// FromSlice constructs a Data from a slice and optional predicate.
func FromSlice(vals []uint32, pred bool) Data {
	return Data{Data: vals, Pred: pred, EOS: false}
}
