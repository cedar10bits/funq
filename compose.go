package funq

import "fmt"

// Fp is a plain function func(T) U that does not return an error.
// The p suffix means "plain". It is the counterpart of [Fe], whose e suffix
// means "error". (Plain does not imply purity: an Fp may still have side
// effects.)
type Fp[T, U any] = func(T) U

// Fe is a function func(T) (U, error) that may fail.
// It is the error-returning counterpart of [Fp].
type Fe[T, U any] = func(T) (U, error)

// Identity returns its argument unchanged.
func Identity[T any](t T) T {
	return t
}

// Const returns a function that ignores its argument and always returns u.
//
// T, the argument type of the returned function, is not determined by u and
// is never inferred, not even from the context the result is used in. It
// comes first so that supplying it alone suffices: Const[int]("x") is a
// func(int) string, with U inferred from u.
func Const[T, U any](u U) Fp[T, U] {
	return func(T) U { return u }
}

// NilError converts an [Fp] to an [Fe] whose error is always nil.
func NilError[T, U any](f Fp[T, U]) Fe[T, U] {
	return func(t T) (U, error) { return f(t), nil }
}

// PanicOnError converts an [Fe] to an [Fp], panicking if f returns an error.
//
// The panic value is an error wrapping the original err with %w, so a
// recover call can inspect it with errors.Is/errors.As to reach the
// original error.
func PanicOnError[T, U any](f Fe[T, U]) Fp[T, U] {
	return func(x T) U {
		u, err := f(x)
		if err != nil {
			panic(fmt.Errorf("PanicOnError: %w", err))
		}
		return u
	}
}

// IgnoreError converts an [Fe] to an [Fp] that returns orElse when f fails.
func IgnoreError[T, U any](f Fe[T, U], orElse U) Fp[T, U] {
	return func(t T) U {
		u, err := f(t)
		if err != nil {
			return orElse
		}
		return u
	}
}

// ErrOnNone converts an [Fp] returning an [Optional] to an [Fe] that fails
// with err when the Optional is None, so a lookup or search that reports
// absence rather than failure can join a [Groove] pipeline. It is the
// [Optional] counterpart of [NilError], [PanicOnError] and [IgnoreError].
//
// The body is just [Optional.OrErr] applied to f's result. The wrapper is
// point-free, so it drops straight into a Jam call without a closure:
//
//	Groove(ErrOnNone(lookup, ErrNotFound)).Jam(save)
func ErrOnNone[T, U any](f Fp[T, Optional[U]], err error) Fe[T, U] {
	return func(t T) (U, error) { return f(t).OrErr(err) }
}

// Chain is a plain-function pipeline under construction. It has no useful
// zero value: build one with [Compose] and extend it with [Chain.Then].
//
// Chain works on plain ([Fp]) stages that cannot fail. When a stage can
// return an error, use [Track], the error-aware (railway-oriented)
// counterpart that short-circuits on the first failure.
type Chain[T0, T1 any] struct {
	run Fp[T0, T1]
}

// Compose cuts a Chain's first stage from f: T0 -> T1. A chain has no fixed
// arity: add further stages by calling [Chain.Then] once per stage.
func Compose[T0, T1 any](f Fp[T0, T1]) Chain[T0, T1] {
	return Chain[T0, T1]{run: f}
}

// Then appends one stage, g: T1 -> T2, to the chain.
func (c Chain[T0, T1]) Then[T2 any](g Fp[T1, T2]) Chain[T0, T2] {
	return Chain[T0, T2]{run: func(t0 T0) T2 { return g(c.run(t0)) }}
}

// Run executes the chain from t0 through every stage in order and returns
// the final result.
//
// Run is itself a method value of type Fp[T0, T1]: a built Chain drops
// straight into anything that takes a plain function, for example
// flow.Map(chain.Run), or as a stage of another Chain via inner.Run.
func (c Chain[T0, T1]) Run(t0 T0) T1 {
	return c.run(t0)
}

// Track is an error-returning function pipeline under construction. It has
// no useful zero value: build one with [Groove] and extend it with
// [Track.Jam].
//
// In brief:
//
//   - Groove cuts a track's first groove
//   - Jam adds a stage to it
//   - Play runs it
//
// It is the railway-oriented-programming counterpart of [Chain]: on failure
// a Track stops at the stage that failed and reports which one, rather than
// running the rest of the pipeline on a zero value.
type Track[T0, T1 any] struct {
	run    func(t0 T0, of int) (T1, error)
	stages int
}

// Groove cuts a Track's first stage from f: T0 -> (T1, error). Add further
// stages with [Track.Jam], and run the finished pipeline with [Track.Play].
func Groove[T0, T1 any](f Fe[T0, T1]) Track[T0, T1] {
	return Track[T0, T1]{stages: 1, run: func(t0 T0, of int) (T1, error) {
		t1, err := f(t0)
		if err != nil {
			var zero T1
			return zero, &grooveError{stage: 1, of: of, err: err}
		}
		return t1, nil
	}}
}

// Jam appends exactly one stage, g: T1 -> (T2, error), to the track. It does
// not repeat or retry the stage it appends.
func (t Track[T0, T1]) Jam[T2 any](g Fe[T1, T2]) Track[T0, T2] {
	n := t.stages + 1
	return Track[T0, T2]{stages: n, run: func(t0 T0, of int) (T2, error) {
		var zero T2
		t1, err := t.run(t0, of)
		if err != nil {
			return zero, err
		}
		t2, err := g(t1)
		if err != nil {
			return zero, &grooveError{stage: n, of: of, err: err}
		}
		return t2, nil
	}}
}

// Play executes the track from t0 through every stage in order. If a stage
// returns an error, Play stops there and returns an error reporting the
// failing stage's position and the track's total stage count, for example
// "funq: Groove pipeline failed at stage 3 of 5: <underlying error>".
//
// Play is itself a method value of type Fe[T0, T1]: a built Track drops
// straight into anything that takes a plain error-returning function, for
// example as a stage of another Track via inner.Play. When it is used that
// way, the inner track's own stage numbering is preserved in its error: a
// failure inside inner.Play still reports "stage k of <inner's own stage
// count>", wrapped by the outer track's error for the stage that called it.
func (t Track[T0, T1]) Play(t0 T0) (T1, error) { return t.run(t0, t.stages) }

// grooveError reports which stage of a Track pipeline failed.
type grooveError struct {
	stage, of int
	err       error
}

var (
	_ error                       = (*grooveError)(nil)
	_ interface{ Unwrap() error } = (*grooveError)(nil)
)

// errPrefix leads every [grooveError] message. It names the package first so
// the message is greppable back to funq, then the API within it.
const errPrefix = "funq: Groove pipeline failed"

// Error formats errPrefix followed by the stage that failed, how many stages
// the pipeline has, and the underlying error.
func (e *grooveError) Error() string {
	return fmt.Sprintf("%s at stage %d of %d: %v", errPrefix, e.stage, e.of, e.err)
}

// Unwrap exposes the immediate cause, per the standard library's error-chain
// contract: errors.Is/errors.As call it repeatedly, so unwrapping more than
// one layer here would hide intermediate grooveError frames from that
// traversal.
func (e *grooveError) Unwrap() error {
	return e.err
}
