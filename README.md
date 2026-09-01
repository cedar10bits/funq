# funq

[![Go](https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Go Reference](https://pkg.go.dev/badge/github.com/cedar10bits/funq.svg)](https://pkg.go.dev/github.com/cedar10bits/funq)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![CI](https://github.com/cedar10bits/funq/actions/workflows/ci.yml/badge.svg)](https://github.com/cedar10bits/funq/actions/workflows/ci.yml)

Type-safe, composable utilities for functional programming in Go.

## Overview

funq provides type-safe functional programming utilities built on generics:
fluent, value-typed sequences, null-safe value handling, and composition of
fallible functions (`Groove`). The standard library's `iter`, `slices`, and
`maps` do not address that last one at all.

`Flow[T]` and `Optional[T]` are concrete value types, so their `Map`/`FlatMap`
methods (and `Flow.Fold`) can each carry a type parameter — a chain changes
element type and stays fluent. That is what funq needs Go 1.27's parameterized
methods for.

The names lean into the pun: data finds its `Flow`, and fallible steps find their
`Groove` — a `Track` you `Jam` stages onto, then `Play`.

```go
// One direction to read, top to bottom, instead of nested calls inside-out.
squares := funq.From(1, 2, 3, 4, 5, 6).
	Filter(func(n int) bool { return n%2 == 0 }).
	Map(func(n int) int { return n * n }).
	Slice() // [4 16 36]

// Groove chains fallible steps (any func(T) (U, error)); the first failure
// short-circuits the rest.
checkout := funq.Groove(findCart).  // userID -> (Cart, error)     SELECT ...
	Jam(reserveStock).              // Cart   -> (Cart, error)     UPDATE ...
	Jam(chargeAndRecord)            // Cart   -> (Receipt, error)  INSERT ...

_, err := checkout.Play(userID)
// err is nil, or e.g. "funq: Groove pipeline failed at stage 2 of 3: SKU-42 out of stock"
```

## Installation

```bash
go get github.com/cedar10bits/funq
```

Requires Go 1.27 or later; there is no build for earlier versions.

## When to reach for funq (vs the standard library)

Go 1.23+'s `iter`, `slices`, and `maps` handle one-off transforms well. funq
targets what they leave open.

Stay with the standard library or a hand-written loop when the transform is one
or two steps, or on a hot path that scans every element — a Flow pipeline is
roughly an order of magnitude slower than a hand-written loop (see the
[Performance](#performance) section).

Reach for funq when:

- a chain has several steps or changes element type mid-chain — it reads
  top-to-bottom instead of inside-out;
- you want something the standard library has no equivalent for: the `GroupBy` /
  `Zip` operations, an `Optional` type, or error-aware composition (`Groove`);
- a lazy source lets a short-circuiting terminal stop early instead of
  materializing the whole sequence;
- honestly, it just feels good to write — a top-to-bottom chain is nicer to
  compose and to revisit than nested calls or a scratch slice and a loop.

It is not all-or-nothing: `Flow.Seq` yields a standard `iter.Seq[T]` and
`FromSeq` consumes one, so funq chains and standard-library iterators compose in
either direction.

## The types

funq has three: `Flow` for sequences, `Optional` for a value that may be absent,
and the `Groove` / `Compose` builders for function pipelines.

pkg.go.dev has runnable, compiler-verified examples for more operations than
shown below, including `SortBy`, `GroupBy`, `Zip`, `Partition`, and `Chunk`:
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

Optional is decoupled from Flow, with a small focused API; `AsFlow()` / `Seq()`
bridge into the full Flow API. It also bridges Go's `(value, error)` idiom both
ways: `FromResult` takes an error-returning call and keeps just presence, while
`OrErr` / `ErrOnNone` turn a `None` back into an error — so an `Optional`-returning
stage slots into a [Groove](#groove---error-aware-composition-railway-oriented)
pipeline.

It implements `json.Marshaler` / `json.Unmarshaler` and is `omitzero`-aware
(`Some(v)` ↔ `v`, `None` ↔ `null` or an absent field). It targets a value
serialized in passing, not a persisted struct field; the type docs cover why
`sql.Scanner` / `driver.Valuer` are deliberately absent.

### Groove - Error-Aware Composition (Railway-Oriented)

The funk in `funq`: the first error short-circuits the rest — railway-oriented
programming.

- Build it with `Groove(first).Jam(next)...` — one `Jam` per stage; stages may
  change type.
- Non-`func(T) (U, error)` stages come in through an adapter: `NilError` for a
  plain function, `ErrOnNone` for one returning an `Optional`.
- `Compose` / `Then` / `Run` mirror this for steps that cannot fail —
  `Compose(f).Then(g).Run(x)`.

`Play` runs the pipeline. On failure, the returned error names the failing
stage's position and the pipeline's length:

```
funq: Groove pipeline failed at stage 3 of 5: <underlying error>
```

## Predicates

Build logic for `Flow.Filter` and validation:
```go
result := funq.From(-5, -2, 0, 3, 4, 7, 8, 12).
	Filter(funq.And(funq.GreaterThan(0), func(v int) bool { return v%2 == 0 })).
	Slice()
// Output: [4 8 12]
```

- Comparisons: `LessThan`, `GreaterThan`, `AtMost`, `AtLeast`, `Equal`,
  `Between`, `OneOf`
- Combining operators: `Not`, `And`, `Or`, `Xor`, `Nand`, `Nor`, `Xnor`,
  `Implies`
- Constants: `True`, `False`

## API Reference

For the complete, up-to-date list of methods and functions, see
pkg.go.dev: https://pkg.go.dev/github.com/cedar10bits/funq

What that index will not tell you:
- `Cache()` materializes a Flow to run the upstream pipeline exactly once, then
  reuse the result across multiple terminal operations
- `Const`'s argument type is not determined by its value and is never inferred,
  so name it: `Const[int]("x")` is a `func(int) string`
- `Flow.To(fn)` applies a `func(Flow[T]) U` to the Flow itself, keeping the
  free functions in a fluent chain: `f.To(funq.Chunk[int](2))`,
  `f.To(funq.Distinct)`
- There is no `Min`/`Max`/`Contains` method on `Flow` — a parameterized
  method cannot constrain the receiver's `T` (see the `Distinct` doc comment).
  Membership is the free function `funq.Contains(f, v)`. Element-wise min/max
  are spelled `f.MinBy(funq.Identity)` / `f.MaxBy(funq.Identity)`

## Performance

Flow is roughly an order of magnitude slower than a hand-written loop on a full
traversal — its value is readability and composition, not raw throughput. Most of
that gap is the cost of composing over `iter.Seq` at all (~6x); funq itself adds
only ~1.3x, a fixed ~7 ns per element that disappears once the per-element work is
non-trivial.

Laziness pays off on early exit from a lazy source: short-circuiting terminals
like `First`, `Find`, `Any`, or `Take` (without ending the chain) turn an O(N)
computation into O(k).

A terminal operation may re-run the whole pipeline from the source each time;
`Cache()` avoids that. `Count()` and `IsEmpty()` are the exception — when the
element count is statically known they answer from it without traversing at all.

See [docs/performance.md](docs/performance.md) for the full benchmark breakdown,
methodology, and reproduce commands.

## Versioning

funq follows [Semantic Versioning](https://semver.org/). It stays on `0.x`
for now, so the API may change between minor versions. Breaking changes are
called out in [CHANGELOG.md](CHANGELOG.md). `v1.0.0` will happen once the API
has settled, not on a fixed schedule.

## License

MIT License - see [LICENSE](LICENSE) for details.