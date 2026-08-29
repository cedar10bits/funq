package funq

import (
	"cmp"
)

// LessThan returns a predicate that is true for values strictly less than r.
func LessThan[T cmp.Ordered](r T) Fp[T, bool] {
	return func(l T) bool { return l < r }
}

// GreaterThan returns a predicate that is true for values strictly greater than r.
func GreaterThan[T cmp.Ordered](r T) Fp[T, bool] {
	return func(l T) bool { return l > r }
}

// AtMost returns a predicate that is true for values less than or equal to r.
func AtMost[T cmp.Ordered](r T) Fp[T, bool] {
	return func(l T) bool { return l <= r }
}

// AtLeast returns a predicate that is true for values greater than or equal to r.
func AtLeast[T cmp.Ordered](r T) Fp[T, bool] {
	return func(l T) bool { return l >= r }
}

// Equal returns a predicate that is true for values equal to x.
func Equal[T comparable](x T) Fp[T, bool] {
	return func(y T) bool { return x == y }
}

// Between returns a predicate that is true for values within the inclusive
// range bounded by x and y. The bounds may be given in either order.
//
// For floating-point types the predicate reports false whenever NaN is
// involved: a NaN argument compares false against any bound, and a NaN bound
// propagates through min/max, making both comparisons false.
func Between[T cmp.Ordered](x, y T) Fp[T, bool] {
	// A single-point range measured faster as an equality test than as two
	// bound comparisons, clearly so for string and only slightly for float64
	// (BenchmarkBetween; see its doc comment for the mechanism). NaN bounds
	// miss this path on their own, since NaN == NaN is false.
	if x == y {
		return Equal(x)
	}
	lo, hi := min(x, y), max(x, y)
	return func(t T) bool { return lo <= t && t <= hi }
}

// OneOf returns a predicate that is true for values present in vs.
func OneOf[T comparable](vs ...T) Fp[T, bool] {
	// Below two elements the map costs an allocation plus a hash per call
	// without buying any lookup.
	ln := len(vs)
	switch ln {
	case 0:
		return False[T]
	case 1:
		return Equal(vs[0])
	}
	dup := make(map[T]struct{}, ln)
	for _, v := range vs {
		dup[v] = struct{}{}
	}
	return func(t T) bool { _, ok := dup[t]; return ok }
}
