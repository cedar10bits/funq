// Package funq provides type-safe, composable utilities for functional programming in Go.
//
// The package rests on three pillars:
//
//   - [Flow] is an immutable sequence with Map / Filter / Take and other
//     transformations. It is eager or lazy depending on its source (values,
//     generator functions, or iter.Seq), not on how many times a pipeline
//     built on it runs. A pipeline cannot be relied on to run only once: a
//     terminal operation that needs the elements may re-run it. See the
//     [Flow] documentation for the re-computation and concurrency caveats
//     and for Cache, which materializes the result.
//   - [Optional] models a value that may be absent, with Map / FlatMap / OrElse
//     and conversions to and from Flow, (value, error), and JSON.
//   - [Compose] and [Groove] build function pipelines one stage at a time via
//     [Chain.Then] and [Track.Jam]. Compose is for plain ([Fp]) steps that
//     cannot fail. Groove is for error-returning ([Fe]) steps. It
//     short-circuits on the first error (railway-oriented programming).
//
// A few operations — [Chunk], [Contains], [Distinct], and [Zip] — are free
// functions rather than Flow methods because their signatures cannot be
// expressed as methods. See each function's documentation for why. Those
// that reduce to a func(Flow[T]) U — [Distinct] as it stands, [Chunk] once
// given its size — drop back into a chain through [Flow.To]:
// f.To(Distinct), f.To(Chunk[int](2)).
package funq
