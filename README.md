# funq

[![Go](https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Go Reference](https://pkg.go.dev/badge/github.com/cedar10bits/funq.svg)](https://pkg.go.dev/github.com/cedar10bits/funq)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![CI](https://github.com/cedar10bits/funq/actions/workflows/ci.yml/badge.svg)](https://github.com/cedar10bits/funq/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/cedar10bits/funq)](https://goreportcard.com/report/github.com/cedar10bits/funq)

Type-safe, composable utilities for functional programming in Go.

## Overview

funq is a Go library that provides type-safe functional programming utilities using generics.
It offers fluent, value-typed sequences, null-safe value handling, and function composition.

`Flow[T]` and `Optional[T]` are concrete value types (not interfaces). This lets
their `Map`/`FlatMap` methods (and, on `Flow`, `Fold`) carry their own type
parameter, so a chain can change the element type and stay fluent — a
capability that requires the parameterized methods added in Go 1.27.

## When to reach for funq (vs the standard library)

Go 1.23+'s `iter`, `slices`, and `maps` packages cover a lot of ground. funq
does not compete with them for one-off transforms — it targets the cases they
leave open.

Prefer the standard library (or a plain loop) when:

- The transform is one or two steps. `slices.Contains`, `slices.Sorted`, or a
  small `for` loop is shorter and has zero overhead.
- The code is a hot path that scans every element — a Flow pipeline is
  roughly an order of magnitude slower than a raw loop (see
  [Performance](#performance)).

Prefer funq when:

- The pipeline has three or more steps, or changes element type mid-chain.
  Nested `slices.X(slices.Y(...))` calls read inside-out. No standard
  library iterator adapter changes element type fluently.
- You need operations the standard library does not provide: `GroupBy`, `Partition`,
  `Zip`, `Optional`, or railway-oriented error handling (`Groove`).
- A lazy source lets a short-circuiting terminal (`First`, `Find`, `Any`) —
  or `Take`, which stops the scan the same way without ending the chain — stop
  early instead of materializing everything (see [Performance](#performance)).

The choice is not all-or-nothing: `Flow.Seq` returns a standard `iter.Seq[T]`,
so a funq chain can feed straight into standard library iterator adapters
that consume one. `FromSeq` goes the other way, building a lazy funq chain from
an existing iterator (e.g. `slices.Values`, `maps.Keys`, or a hand-rolled
generator) without materializing it first.

## Installation

```bash
go get github.com/cedar10bits/funq
```

## Usage Examples

A taste of each type below. pkg.go.dev has runnable, compiler-verified
examples for more operations than shown here, including `SortBy`, `GroupBy`,
`Zip`, `Partition`, and `Chunk`:
https://pkg.go.dev/github.com/cedar10bits/funq#pkg-examples

### Flow - Slice Operations

```go
package main

import (
	"fmt"

	"github.com/cedar10bits/funq"
)

func main() {
	result := funq.FromFn(10, func(i int) int { return i + 1 }).
		Filter(func(v int) bool { return v%2 == 0 }). // Keep even numbers: [2, 4, 6, 8, 10]
		Map(func(v int) int { return v * 3 }). // Multiply by 3: [6, 12, 18, 24, 30]
		Drop(1).                        // Drop first: [12, 18, 24, 30]
		Take(3).                        // Take first 3: [12, 18, 24]
		Slice()

	fmt.Println(result) // Output: [12 18 24]
}
```

### Optional - Null-Safe Values

```go
// The chain stays Optional and can change type along the way
val := funq.Some(42).
	Filter(funq.GreaterThan(10)).
	Map(func(v int) int { return v * 2 }). // Optional[int]
	Map(strconv.Itoa)                     // Optional[string]

fmt.Println(val.OrElse("none")) // Output: 84
```

Optional is decoupled from Flow: it offers a small, focused API
(`Map`, `FlatMap`, `Filter`, `OrElse`, `OrElseGet`, `Get`, `MustGet`, `Or`, `Ptr`,
`String`, ...).
Use `AsFlow()` or `Seq()` to bridge into the full Flow API.

Optional also bridges to Go's `(value, error)` idiom.
`FromResult(strconv.Atoi(s))` takes an error-returning call directly, keeping
presence and dropping the error. `OrErr(err)` goes the other way, turning a `None`
back into a failure so an `Optional`-returning terminal like `Find` can be returned
straight from an error-returning function. `ErrOnNone(f, err)` wraps `OrErr` in
point-free form, so an `Optional`-returning stage drops into a
[Groove](#groove---error-aware-composition-railway-oriented) pipeline's `Jam`
call without a closure.

#### JSON

Because an Optional is often returned from a chain and then serialized (e.g. in
an HTTP response), it implements `json.Marshaler` / `json.Unmarshaler`:

```go
type Profile struct {
	Name  string                `json:"name"`
	Email funq.Optional[string] `json:"email"`           // null when None
	Phone funq.Optional[string] `json:"phone,omitzero"`  // omitted when None
}

// Some(v) -> v, None -> null. On decode, an absent field or null becomes None.
b, _ := json.Marshal(Profile{Name: "Ada", Email: funq.Some("ada@example.com")})
// {"name":"Ada","email":"ada@example.com"}
```

`Optional` also implements `IsZero`, so the `json:",omitzero"` tag (Go 1.24+)
drops a `None` field entirely, matching the zero value being `None`.

> Scope: JSON support reflects Optional's role as a fluent chain / return
> value that may be serialized, not a persisted struct-field type — see the
> `Optional` type documentation for the full rationale, including why
> `sql.Scanner`/`driver.Valuer` aren't implemented instead.

### Groove - Error-Aware Composition (Railway-Oriented)

```go
doubleOrError := func(n int) (int, error) {
	if n > 1000 {
		return 0, fmt.Errorf("number too large: %d", n)
	}
	return n * 2, nil
}

// Groove(f) cuts the first stage. Each Jam appends exactly one more.
// string -> (int, error) -> (int, error) -> (string, error)
parseDoubleStringify := funq.Groove(strconv.Atoi).
	Jam(doubleOrError).
	Jam(funq.NilError(strconv.Itoa))

result, err := parseDoubleStringify.Play("21")
// result: "42", err: nil
```

A `Track` has no fixed length: keep calling `Jam` for as many stages as the
pipeline needs. The built value is reusable across inputs. Since `Play` is itself an
`Fe[T0, T1]` method value, it drops straight into anywhere a plain
error-returning function is expected — including as a stage of another
`Track` via `inner.Play`. `Compose`/`Then`/`Run` are the same shape for
stages that cannot fail (see
[Utilities for Flow and Groove](#utilities-for-flow-and-groove) below).

On failure, `Play` stops at the stage that failed and returns an error
reporting its position and the pipeline's stage count:

```
funq: Groove pipeline failed at stage 3 of 5: <underlying error>
```

A stage that reports absence rather than failure joins the pipeline through `ErrOnNone`;
see [Optional](#optional---null-safe-values).

### Utilities for Flow and Groove

**Predicates** – Build logic for filtering and validation:
```go
// Use predicates with Flow.Filter()
result := funq.From(-5, -2, 0, 3, 4, 7, 8, 12).
	Filter(funq.And(funq.GreaterThan(0), func(v int) bool { return v%2 == 0 })).
	Slice()
// Output: [4 8 12]
```

**Function Composition** – Compose higher-order functions:
```go
addThenFilter := func(n int) funq.Flow[int] {
	return funq.From(n).
		Map(func(v int) int { return v + 10 }).
		Filter(funq.GreaterThan(15))
}

// A no-arg method is already usable as a method expression
pipeline := funq.Compose(addThenFilter).Then(funq.Flow[int].Slice)
result1 := pipeline.Run(5)	// Output: []
result2 := pipeline.Run(10)	// Output: [20]
```

## API Reference

For the complete, up-to-date list of methods and functions, see
pkg.go.dev: https://pkg.go.dev/github.com/cedar10bits/funq

A few entry points, to get oriented:
- `From(values...)`, `FromFn(n, fn)`, `FromSeq(seq)`: create a Flow from values,
  a function, or an iterator. `Flow.Seq` goes the other way, returning an
  `iter.Seq[T]`
- `Cache()`: materialize a Flow to run the upstream pipeline exactly once, then
  reuse the result across multiple terminal operations
- `Some(x)`, `None[int]()`, `FromPtr(ptr)`, `FromResult(v, err)`: create Optionals.
  `Optional.OrErr(err)` and `ErrOnNone(f, err)` go back to `(value, error)`
- `Compose(f).Then(g)...Run(x)`: build a plain-function pipeline one stage at
  a time. `Groove(f).Jam(g)...Play(x)` is the error-returning counterpart,
  short-circuiting on the first error
- `Identity`, `Const(v)`: pass as the key or step argument of `SortBy`/`MinBy`/
  `Map` and friends. `Const`'s argument type is never inferred, so name it:
  `Const[int]("x")` is a `func(int) string`
- Predicates (`And`, `Or`, `LessThan`, `Between`, `OneOf`, ...) are designed to plug directly into `Filter`/`Map`
- `Flow.To(fn)` applies a `func(Flow[T]) U` to the Flow itself, keeping the
  free functions in a fluent chain: `f.To(funq.Chunk[int](2))`,
  `f.To(funq.Distinct)`
- There is no `Min`/`Max`/`Contains` method on `Flow` — a parameterized
  method cannot constrain the receiver's `T` (see the `Distinct` doc comment).
  Membership is the free function `funq.Contains(f, v)`. Element-wise min/max
  are spelled `f.MinBy(funq.Identity)` / `f.MaxBy(funq.Identity)`

## Performance

Flow is roughly an order of magnitude slower than a raw loop on a full traversal —
its value is readability and composition, not raw throughput. Laziness pays off on
early exit from a lazy source (short-circuiting terminals like `First`, `Find`,
`Any`, or `Take` without ending the chain), turning an O(N) computation into O(k).
A terminal operation may re-run the pipeline from the source each time. Use
`Cache()` to materialize an expensive pipeline once and reuse it. `Count()` and
`IsEmpty()` are the exception — when the element count is statically known they
answer from it without traversing at all.
See [docs/performance.md](docs/performance.md) for the full benchmark breakdown,
methodology, and reproduce commands.

## Versioning

funq follows [Semantic Versioning](https://semver.org/). It stays on `0.x`
for now, so the API may change between minor versions. Breaking changes are
called out in [CHANGELOG.md](CHANGELOG.md) rather than silently shipped.
`v1.0.0` is not planned on a fixed schedule — it will happen once the API
has settled, not simply once Go 1.27 reaches GA.

## License

MIT License - see [LICENSE](LICENSE) for details.