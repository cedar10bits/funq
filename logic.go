package funq

// True is a predicate that always reports true.
func True[T any](T) bool { return true }

// False is a predicate that always reports false.
func False[T any](T) bool { return false }

// Not returns a predicate that negates pred.
func Not[T any](pred Fp[T, bool]) Fp[T, bool] {
	return func(t T) bool { return !pred(t) }
}

// And returns a predicate that is true only when every preds is true.
// It short-circuits on the first false. It returns true when preds is empty.
func And[T any](preds ...Fp[T, bool]) Fp[T, bool] {
	return func(t T) bool {
		for _, f := range preds {
			if !f(t) {
				return false
			}
		}
		return true
	}
}

// Or returns a predicate that is true when any preds is true.
// It short-circuits on the first true. It returns false when preds is empty.
func Or[T any](preds ...Fp[T, bool]) Fp[T, bool] {
	return func(t T) bool {
		for _, f := range preds {
			if f(t) {
				return true
			}
		}
		return false
	}
}

// Xor returns a predicate that is true when exactly one of l or r is true.
func Xor[T any](l, r Fp[T, bool]) Fp[T, bool] {
	return func(t T) bool { return l(t) != r(t) }
}

// Nand returns a predicate that is true unless both l and r are true.
func Nand[T any](l, r Fp[T, bool]) Fp[T, bool] {
	return func(t T) bool { return !l(t) || !r(t) }
}

// Nor returns a predicate that is true only when both l and r are false.
func Nor[T any](l, r Fp[T, bool]) Fp[T, bool] {
	return func(t T) bool { return !l(t) && !r(t) }
}

// Xnor returns a predicate that is true when l and r agree.
func Xnor[T any](l, r Fp[T, bool]) Fp[T, bool] {
	return func(t T) bool { return l(t) == r(t) }
}

// Implies returns a predicate that is true unless l is true and r is false.
func Implies[T any](l, r Fp[T, bool]) Fp[T, bool] {
	return func(t T) bool { return !l(t) || r(t) }
}
