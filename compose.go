package funq

import (
	"errors"
	"fmt"
	"slices"
)

// Fp is a plain function func(T) U that does not return an error. It is the
// counterpart of [Fe]: the p suffix means "plain", the e means "error".
// (Plain does not imply purity: an Fp may still have side effects.)
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
// recover call can inspect it with errors.Is/errors.AsType.
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
// The wrapper is point-free, so it drops straight into a Jam call:
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

// Run executes the chain from t0 through every stage in order.
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
//   - Groove cuts a track's first stage
//   - Jam adds a stage to it
//   - OnBreak registers a rollback for what the track holds so far
//   - Play runs it
//
// It is the railway-oriented-programming counterpart of [Chain]: on failure
// a Track stops at the stage that failed and reports which one, rather than
// running the rest of the pipeline on a zero value.
type Track[T0, T1 any] struct {
	run       func(T0, trackState) (T1, error)
	stages    int
	rollbacks int
}

// trackState is what a stage needs from the [Track.Play] call running it: the
// track's total stage count for error reporting, and where to register a
// compensation. undoes is a pointer because a panicking stage discards return
// values, yet Play must still reach what was registered before the panic. A Track
// stays a pure value: the state belongs to Play, which builds a fresh one per
// call and leaves undoes nil when the track has no [Track.OnBreak].
type trackState struct {
	of     int
	undoes *[]func() error
}

// Groove cuts a Track's first stage from f: T0 -> (T1, error). Add further
// stages with [Track.Jam], and run the finished pipeline with [Track.Play].
func Groove[T0, T1 any](f Fe[T0, T1]) Track[T0, T1] {
	return Track[T0, T1]{
		stages: 1,
		run: func(t0 T0, st trackState) (T1, error) {
			t1, err := f(t0)
			if err != nil {
				var zero T1
				return zero, &grooveError{stage: 1, of: st.of, err: err}
			}
			return t1, nil
		},
	}
}

// Jam appends exactly one stage, g: T1 -> (T2, error), to the track. It does
// not repeat or retry the stage it appends.
func (t Track[T0, T1]) Jam[T2 any](g Fe[T1, T2]) Track[T0, T2] {
	n := t.stages + 1
	return Track[T0, T2]{
		stages:    n,
		rollbacks: t.rollbacks,
		run: func(t0 T0, st trackState) (T2, error) {
			var zero T2
			t1, err := t.run(t0, st)
			if err != nil {
				return zero, err
			}
			t2, err := g(t1)
			if err != nil {
				return zero, &grooveError{stage: n, of: st.of, err: err}
			}
			return t2, nil
		},
	}
}

// OnBreak registers compensate as a rollback for the value the track holds
// at this point, to undo what the stages so far produced — closing a
// transaction they opened, say — if a later stage fails.
//
// It adds no stage of its own: it neither runs compensate nor changes the
// track's stage count or output type, so the positions [Track.Play] reports
// are unaffected. compensate runs only when the pipeline fails after this
// point — whether a later stage returns an error or panics — and never on
// success.
//
// Rollback is per-track: register a compensation on the outer track when it
// has to cover the whole pipeline. See [Track.Play] for the rollback order,
// how a compensation's own failure is handled, and how nesting scopes
// rollback.
func (t Track[T0, T1]) OnBreak(compensate func(T1) error) Track[T0, T1] {
	stage := t.stages
	return Track[T0, T1]{
		stages:    stage,
		rollbacks: t.rollbacks + 1,
		run: func(t0 T0, st trackState) (T1, error) {
			t1, err := t.run(t0, st)
			if err != nil {
				var zero T1
				return zero, err
			}
			*st.undoes = append(*st.undoes, func() error { return compensation(stage, func() error { return compensate(t1) }) })
			return t1, nil
		},
	}
}

// Play executes the track from t0 through every stage in order. If a stage
// returns an error, Play stops there and returns an error naming the
// failing stage and the track's total stage count, for example
// "funq: Groove pipeline failed at stage 3 of 5: <underlying error>".
//
// It then rolls back, running the compensations registered by [Track.OnBreak]
// before the failing stage in LIFO order, regardless of whether an earlier
// one failed. A compensation that fails or panics is contained and joined
// onto the pipeline's own error with errors.Join, so Play returns rather
// than panics on this path.
//
// Play is itself a method value of type Fe[T0, T1]: a built Track drops
// straight into anything that takes a plain error-returning function, for
// example as a stage of another Track via inner.Play. Nested this way, the
// inner track's own stage numbering survives in its error — a failure
// inside inner.Play still reports "stage k of <inner's own stage count>" —
// wrapped by the outer track's error for the stage that called it. Rollback
// is likewise per-track: inner.Play resolves its own compensations before
// returning, independently of the outer track's.
//
// A stage that panics rolls back the same way, from a defer that never
// intercepts the panic, so a caller's recover sees exactly what was raised.
// A compensation's own panic is contained here too, but with no error
// return to attach it to, whatever it reports is discarded.
func (t Track[T0, T1]) Play(t0 T0) (T1, error) {
	if t.rollbacks == 0 {
		return t.run(t0, trackState{of: t.stages})
	}

	// A flag rather than a recover: recover would re-raise the panic from a
	// new frame, and would miss an exit that raises none (runtime.Goexit).
	stagesDone := false
	undoes := make([]func() error, 0, t.rollbacks)
	defer func() {
		if !stagesDone {
			_ = rollback(undoes)
		}
	}()

	t1, err := t.run(t0, trackState{of: t.stages, undoes: &undoes})
	stagesDone = true
	if err == nil {
		return t1, nil
	}

	if cerr := rollback(undoes); cerr != nil {
		err = errors.Join(err, cerr)
	}
	var zero T1
	return zero, err
}

func rollback(undoes []func() error) error {
	var errs []error
	for _, u := range slices.Backward(undoes) {
		if err := u(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// compensation runs a stage's compensation, reporting what it returns — or
// what it panicked with — as an error naming stage. Containing the panic
// here keeps one compensation from cutting the rollback short; see
// [Track.Play] for why it must not displace a stage's own panic.
func compensation(stage int, run func() error) (err error) {
	defer func() {
		switch r := recover().(type) {
		case nil:
		case error:
			err = fmt.Errorf("funq: rollback for stage %d panicked: %w", stage, r)
		default:
			err = fmt.Errorf("funq: rollback for stage %d panicked: %v", stage, r)
		}
	}()
	if cerr := run(); cerr != nil {
		return fmt.Errorf("funq: rollback for stage %d failed: %w", stage, cerr)
	}
	return nil
}

// grooveError reports which stage of a Track pipeline failed.
type grooveError struct {
	stage, of int
	err       error
}

var (
	_ error                       = (*grooveError)(nil)
	_ interface{ Unwrap() error } = (*grooveError)(nil)
)

// errPrefix leads every [grooveError] message, naming the package first so
// the message is greppable back to funq.
const errPrefix = "funq: Groove pipeline failed"

func (e *grooveError) Error() string {
	return fmt.Sprintf("%s at stage %d of %d: %v", errPrefix, e.stage, e.of, e.err)
}

// Unwrap exposes the immediate cause, per the standard library's error-chain
// contract: errors.Is/errors.AsType call it repeatedly, so unwrapping more
// than one layer here would hide intermediate grooveError frames from that
// traversal.
func (e *grooveError) Unwrap() error {
	return e.err
}
