package funq

import (
	"encoding/json"
	"fmt"
	"iter"
)

// Optional is a null-safe container holding zero or one value of type T.
//
// Like [Flow], it is a concrete value type, so [Optional.Map] and
// [Optional.FlatMap] can change the value type while keeping the chain
// fluent and the result an Optional.
//
// Optional is intentionally decoupled from Flow: it offers a small, focused
// API (presence, mapping, fallback) rather than the full sequence interface.
// Use [Optional.AsFlow] or [Optional.Seq] to bridge into Flow operations.
//
// JSON support is scoped to the Optional's role as a fluent chain / return
// value that may be serialized (e.g. in an HTTP response), not as a persisted
// struct-field type. For database round-trips use database/sql's Null[T].
// Optional intentionally does not implement sql.Scanner / driver.Valuer.
//
// The zero value is None.
type Optional[T any] struct {
	v  T
	ok bool
}

// Some creates an Optional containing v.
func Some[T any](v T) Optional[T] { return Optional[T]{v: v, ok: true} }

// None creates an empty Optional.
func None[T any]() Optional[T] { return Optional[T]{} }

// FromPtr converts a pointer to an Optional: None if p is nil, otherwise
// Some with the dereferenced value.
func FromPtr[T any](p *T) Optional[T] {
	if p == nil {
		return None[T]()
	}
	return Some(*p)
}

// FromResult converts a (value, error) pair to an Optional: None if err is
// non-nil, otherwise Some(v). Its parameters match the results of an
// error-returning call, so such a call can be passed directly:
// FromResult(strconv.Atoi(s)).
//
// The error is discarded, so reach for FromResult only when presence is all
// that matters downstream. It is the inverse of [Optional.OrErr], but only for
// presence: a round trip replaces the original error with whichever one OrErr
// was handed.
func FromResult[T any](v T, err error) Optional[T] {
	if err != nil {
		return None[T]()
	}
	return Some(v)
}

// IsPresent reports whether a value is present.
func (o Optional[T]) IsPresent() bool { return o.ok }

// IsEmpty reports whether the Optional is empty.
func (o Optional[T]) IsEmpty() bool { return !o.ok }

// Get returns the value and whether it is present, in the Go comma-ok style.
func (o Optional[T]) Get() (T, bool) { return o.v, o.ok }

// OrElse returns the value if present, otherwise fallback.
func (o Optional[T]) OrElse(fallback T) T {
	if o.ok {
		return o.v
	}
	return fallback
}

// OrElseGet returns the value if present, otherwise the result of fn.
func (o Optional[T]) OrElseGet(fn func() T) T {
	if o.ok {
		return o.v
	}
	return fn()
}

// OrErr returns the value and a nil error if present, otherwise the zero value
// and err. It is the exit from a fluent chain back into Go's (value, error)
// idiom, so an Optional-returning terminal such as [Flow.Find] can be returned
// straight from an error-returning function.
//
// It is the inverse of [FromResult]; see there for why a round trip loses the
// original error. [ErrOnNone] wraps this for use as a Groove pipeline stage.
//
// Pass a non-nil err. A nil one makes None report success with the zero value,
// indistinguishable from Some of that value.
func (o Optional[T]) OrErr(err error) (T, error) {
	if o.ok {
		return o.v, nil
	}
	var zero T
	return zero, err
}

// MustGet returns the value if present, otherwise panics.
func (o Optional[T]) MustGet() T {
	if !o.ok {
		panic("funq: MustGet called on None")
	}
	return o.v
}

// Or returns o if present, otherwise other.
func (o Optional[T]) Or(other Optional[T]) Optional[T] {
	if o.ok {
		return o
	}
	return other
}

// Filter keeps the value only if it satisfies pred.
func (o Optional[T]) Filter(pred func(T) bool) Optional[T] {
	if o.ok && pred(o.v) {
		return o
	}
	return None[T]()
}

// Map transforms the value with fn, propagating None unchanged and possibly
// changing the value type; see the [Optional] documentation for why methods
// like this can carry their own type parameter.
func (o Optional[T]) Map[U any](fn func(T) U) Optional[U] {
	if o.ok {
		return Some(fn(o.v))
	}
	return None[U]()
}

// FlatMap transforms the value into another Optional, propagating None
// unchanged.
func (o Optional[T]) FlatMap[U any](fn func(T) Optional[U]) Optional[U] {
	if o.ok {
		return fn(o.v)
	}
	return None[U]()
}

// ForEach applies fn to the value if present.
func (o Optional[T]) ForEach(fn func(T)) {
	if o.ok {
		fn(o.v)
	}
}

// Seq returns an iterator yielding the value if present, otherwise nothing.
func (o Optional[T]) Seq() iter.Seq[T] {
	return func(yield func(T) bool) {
		if o.ok {
			yield(o.v)
		}
	}
}

// AsFlow bridges the Optional into a Flow of zero or one element.
func (o Optional[T]) AsFlow() Flow[T] {
	if o.ok {
		return fromSlice([]T{o.v})
	}
	return empty[T]()
}

// Ptr returns a pointer to the value, or nil for None. It is the
// inverse of [FromPtr], for interoperating with APIs that represent an
// optional value as a pointer (e.g. the AWS SDK or generated protobuf code).
//
// The pointer refers to a copy of the value, so writing through it does not
// change the Optional. Each call returns a fresh pointer.
func (o Optional[T]) Ptr() *T {
	if !o.ok {
		return nil
	}
	v := o.v
	return &v
}

// String renders Some as "Some(v)" and None as "None", so %v and %s (e.g. in
// a log line) show a readable form instead of the struct's unexported
// fields.
func (o Optional[T]) String() string {
	if !o.ok {
		return "None"
	}
	return fmt.Sprintf("Some(%v)", o.v)
}

// MarshalJSON encodes Some as its value and None as JSON null.
func (o Optional[T]) MarshalJSON() ([]byte, error) {
	if !o.ok {
		return []byte("null"), nil
	}
	return json.Marshal(o.v)
}

// UnmarshalJSON decodes JSON into the Optional. A JSON null yields None; any
// other value is decoded into T and yields Some. When the Optional is a value-
// type struct field, an absent field leaves it at its zero value (None)
// because UnmarshalJSON is never called. Absent and null therefore both map
// to None.
func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*o = None[T]()
		return nil
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*o = Some(v)
	return nil
}

// IsZero reports whether the Optional is None, matching the zero value being
// None. It lets encoding/json's ",omitzero" option drop a None field.
func (o Optional[T]) IsZero() bool { return !o.ok }
