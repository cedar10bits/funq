# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Until v1.0.0, the API may change between minor versions. Breaking changes are
called out here.

## [Unreleased]

### Changed

- `Flow.Cache` and `Flow.Partition` no longer keep a buffer more than twice
  the size of their elements after a `Filter` that dropped most of them.
- `And` and `Or` copy their predicate slice, so changing it afterward no
  longer changes the predicate.

### Fixed

- `Chunk`: panicked when `n` far exceeded the Flow's length (e.g.
  `Chunk(math.MaxInt)`); the first chunk is now sized from the Flow.

## [0.2.0] - 2026-09-13

### Added

- `Flow.Accumulate`: the running counterpart of `Fold`, yielding the seed and
  every intermediate accumulator state (Haskell's `scanl`, Kotlin's
  `runningFold`). Lazy, and one element longer than its input.
- `Track.OnBreak`: registers a per-stage rollback compensation, run LIFO on a
  later stage's failure or panic. See the README or `Track.Play`'s
  documentation for the contract.

### Changed

- Reduced allocations across several `Flow` operations, including `SortBy`,
  `FlatMap`, `Map`, `MapIndexed`, `Zip`, and `Chunk`. See
  [docs/performance.md](docs/performance.md) for figures.

### Fixed

- `Concat`: panicked when every input Flow was empty; now returns empty instead.

## [0.1.0] - 2026-08-23

Initial release. Requires Go 1.27 or later: the type-changing chains below
rest on parameterized methods.

### Added

- `Flow[T]`: an immutable sequence with type-changing `Map`/`FlatMap`/`Fold`,
  plus the operations whose signatures cannot be expressed as methods and so
  ship as free functions (`Zip`, `Chunk`, `Distinct`, `Contains`), and a
  `FromSeq` constructor to bridge standard iterators into lazy pipelines.
- `Optional[T]`: a null-safe container with type-changing `Map`/`FlatMap`,
  bridging to Flow, to `(value, error)`, and to JSON. Implements
  `fmt.Stringer`, rendering as `Some(v)` or `None`.
- `Compose`/`Chain.Then`/`Chain.Run` and `Groove`/`Track.Jam`/`Track.Play`:
  arity-free builders for composing plain and error-returning functions one
  stage at a time, the latter short-circuiting on the first error and
  reporting which stage failed.
- Predicates and function helpers to pass to the above: `And`, `Between`,
  `OneOf`, `Identity`, `NilError`, `ErrOnNone`, and friends.

For the complete API, see
[pkg.go.dev](https://pkg.go.dev/github.com/cedar10bits/funq).

[Unreleased]: https://github.com/cedar10bits/funq/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/cedar10bits/funq/releases/tag/v0.2.0
[0.1.0]: https://github.com/cedar10bits/funq/releases/tag/v0.1.0
