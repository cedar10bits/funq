# Performance

> All figures in this section were measured on `go1.27.0`, darwin/arm64
> (Apple M2). Absolute numbers vary by machine and Go version — the ratios
> are the point. See the reproduce commands at the end of this document.
>
> The evaluation behavior described below is funq's today, not a contract.
> For what the package actually guarantees, see each method's doc comment
> for the exact conditions and the `Flow` doc comment (rule 1 in particular)
> for when transform functions run.

## Who this is for

This document is for callers deciding whether and how to use funq in a
performance-sensitive path. Two things hold at once:

- **The implementation favors the cheaper path wherever it can** without
  changing behavior — see the O(1) `Take`/`Drop` cases below and `SortBy`'s
  permutation-sort choice, both backed by benchmarks.
- **The fluent API still costs something over a hand-written loop** (see
  [Throughput vs a hand-written loop](#throughput-vs-a-hand-written-loop)).

Neither fact alone tells you what to do with a specific pipeline. The
sections below cover the characteristics that decide it:

- which operations stay lazy
- when a pipeline re-runs
- when laziness turns an expensive scan cheap

## When Flow is cheap and when it isn't

Eager vs lazy is a property of how a Flow is constructed, not a separate type:

- **Eager**: `From` wraps concrete data (zero-copy: `From(xs...)` aliases
  `xs`, so don't mutate it afterward).
- **Lazy**: `FromFn` generates elements on demand, ideal for large
  datasets or when you only need a subset (`Take`/`First` stop early).

Intermediate operations (`Map`, `Filter`, ...) are lazy. Terminal operations
(`Slice`, `Reduce`, `Count`, ...) drive the computation.

> `Cache`, `SortFunc`, and `SortBy` always materialize the Flow immediately:
> the upstream pipeline runs once, there, and later terminal calls reuse the
> result instead of re-running it.
>
> Construction cost:
>
> - `Drop`/`Take` on a random-access Flow with a statically known count: O(1),
>   fully lazy.
> - `Drop`/`Take` on a forward-only Flow: lazy, but they skip or stop as they
>   traverse.
> - `Drop`/`Take` on a random-access Flow with unknown count (e.g. after
>   `Filter`): one pipeline scan at construction time.
> - `DropWhile`/`TakeWhile` on any random-access Flow: one scan at
>   construction, since the predicate determines the boundary. When the input's
>   count *was* statically known, they recover it in O(1) from the width of the
>   boundary their scan found, so the result keeps a known count too:
>   `Count`/`IsEmpty` stay O(1) on it, and a following `Take`/`Drop` takes the
>   O(1) path instead of scanning again. After `Filter` (unknown count, and
>   therefore possibly holes), that recovery doesn't apply and the result stays
>   `sizeUnknown`, as before.
>
> Of those, only `Take` materializes: it keeps the elements its scan produced.
> `Drop`, `DropWhile`, and `TakeWhile` keep just the boundary the scan found,
> so the upstream pipeline (predicate included) runs again on every later
> terminal call, unless the scan reached the end, which leaves `Drop` and
> `DropWhile` empty and `TakeWhile` unchanged. `Drop` and `DropWhile`'s
> construction scan covers only the discarded prefix, disjoint from what a
> later terminal call traverses; `TakeWhile`'s construction scan instead
> covers the same range a terminal call re-walks, so an expensive predicate or
> upstream pipeline effectively runs one extra time up front (see
> `Flow.TakeWhile`'s doc comment).
>
> `Reverse` is O(1) on random-access Flows, but materializes forward-only
> Flows (built directly by `FromSeq`, or produced by `FlatMap`, `MapIndexed`,
> `Distinct`, `DistinctBy`, `Zip`, `Chunk`, or `Concat` with unknown-size
> inputs).

Two of those exceptions are worth knowing the cost of directly:

- **`Take(n)`** is O(1) and stays lazy whenever the count is statically
  known (see above), including right after a `TakeWhile`/`DropWhile` that
  recovered one. In its scanning case — unknown count on a still
  random-access Flow — it stops at the n-th surviving element, scanning to
  the end only when fewer than n survive. Because that scan's result is
  materialized, later terminal calls reuse it instead of re-running the
  pipeline.
- **`SortBy`** always sorts a permutation of indices rather than handing
  `key` to the comparator: it calls `key` exactly once per element up
  front and builds two extra slices of length n on top of the output —
  the keys and the permutation — rather than repeatedly evaluating `key`
  and swapping whole elements during the sort.

`SortBy`'s permutation-sort choice is backed by `BenchmarkSortByStrategies`,
which measures it against the alternative it replaced: handing `key` straight
to the comparator instead of sorting an index permutation. At n=1,000,000:

**Time**

| Shape                 | permutation | comparator | comparator/permutation |
| --------------------- | ----------- | ---------- | ---------------------- |
| int, key=identity     | ~489 ms/op  | ~467 ms/op | ~1.0x                  |
| 64B elem, key=field   | ~798 ms/op  | ~1.33 s/op | ~1.7x                  |
| 64B elem, key=tolower | ~915 ms/op  | ~3.80 s/op | ~4.2x                  |

**Memory**

| Shape                 | permutation              | comparator               | permutation/comparator |
| --------------------- | ------------------------ | ------------------------ | ---------------------- |
| int, key=identity     | ~40.0 MB/op, 11 allocs   | ~16.0 MB/op, 8 allocs    | ~2.5x                  |
| 64B elem, key=field   | ~216 MB/op, 12 allocs    | ~128 MB/op, 8 allocs     | ~1.7x                  |
| 64B elem, key=tolower | ~232 MB/op, 1.00M allocs | ~937 MB/op, 50.6M allocs | ~0.25x                 |

The identity/int shape is the one that costs permutation something: the two
strategies draw even in time (~1.0x) there, and permutation pays for that
draw with about 2.5x the memory. That's not just the two extra slices
described above — those alone would be 2.0x. The third is `SortBy`'s own
output slice: the comparator formulation doesn't need one, because
`SortFunc` sorts in place and returns the same slice it was given. Once
either axis — element size or key cost — moves, permutation wins outright.
With a 64-byte element it's ~1.7x faster. With a non-trivial key over
that same element (`strings.ToLower`) it's ~4.2x faster while using a
quarter of the memory, because `key` runs once per element instead of ~50
times — visible in the allocation counts (1.00M vs 50.6M allocs/op).

That identity/int draw is specific to n=1,000,000; permutation is still
measurably ahead of comparator there at smaller n, same as in the other two
shapes:

| n         | int, key=identity | 64B elem, key=field | 64B elem, key=tolower |
| --------- | ----------------- | ------------------- | --------------------- |
| 100       | ~1.2x             | ~1.8x               | ~4.8x                 |
| 10,000    | ~1.0x             | ~1.6x               | ~4.7x                 |
| 1,000,000 | ~1.0x             | ~1.7x               | ~4.2x                 |

(comparator/permutation time ratio; >1x means permutation is faster). The
identity/int column drops to a ~1.0x draw by n=10,000 and holds there. The
other two shapes stay in the same elevated range (~1.6-1.8x for field,
~4.2-4.8x for tolower) at every n measured — permutation's edge there doesn't
depend on scale, only on key cost and element size. Permutation's memory cost
for identity/int holds close to that ~2.5x across n too (~2.3x at n=100,
~2.5x at n=10,000): the three extra slices — keys, index permutation, and
the output — scale with n like everything else.

See `Flow.Take` and `Flow.SortBy` in `flow.go` for why each implementation
was chosen over the alternative it replaced.

> **A pipeline cannot be relied on to run only once.** A terminal call like
> `Slice`, `Reduce`, or `First` may re-run the whole pipeline from the
> source — the `FromFn` generator (if the source is one) and every
> `Map`/`Filter` function along the way may run again each time. `Count`
> and `IsEmpty` are the exception: when the element count is statically
> known, they answer from it directly instead of traversing. This
> matters when:
>
> - **The functions are expensive.** Two terminal calls do the work twice.
>   Call `Cache()` once to materialize the result and reuse it across traversals.
> - **The functions have side effects.** They fire once per terminal call,
>   not once per element overall — usually a surprise. Keep transforms pure, or
>   `Cache()` before the side effect matters.
>
> A Flow is also not safe for concurrent use for the same reason; see the
> `Flow` doc comment.
>
> **When to reach for `Cache()`:** call it if either situation above applies.
> If not (a single terminal call over a pure chain built from `From`/`FromFn`),
> `Cache()` buys nothing and just adds an allocation.

## Throughput vs a hand-written loop

The fluent API wraps every stage in a closure and an `iter.Seq`, so a
full-scan pipeline is meaningfully slower than the equivalent `for` loop. It
also allocates where the loop allocates nothing. A representative pipeline
(keep even, double, sum) over 1,000 ints:

| Approach                | Time       | Allocations     |
| ----------------------- | ---------- | --------------- |
| Hand-written loop       | ~1.0 µs/op | 0 B, 0 allocs   |
| stdlib iter composition | ~6.6 µs/op | 40 B, 3 allocs  |
| Flow pipeline           | ~8.3 µs/op | 184 B, 6 allocs |

So **Flow is roughly an order of magnitude slower than a hand-written loop on
a full traversal.** Its value is *readability and composition*, not raw
throughput. In a hot path that scans every element, write the loop.

Most of that order of magnitude, though, is not a funq-specific cost. The
`stdlib iter composition` row writes the same pipeline against `iter.Seq`,
composing hand-rolled `Filter`/`Map` combinators (the standard library
doesn't ship them) instead of a `for` loop — and that alone already costs
roughly 6x the hand-written loop. Flow adds only ~1.3x on top of that
(~8.3 µs vs ~6.6 µs/op).

In absolute terms the overhead is roughly 7 ns per element, so what
matters is elements × calls per second. A handler filtering a few thousand
items per request pays microseconds, noise next to any I/O. A scan over
millions of elements per frame or per request pays tens of milliseconds.

That per-element overhead also shrinks in relative terms once the pipeline does
real work. Keeping the same filter/map/sum shape but varying the cost of the
map step (a hash-like loop of N iterations per element, so its N=0 baseline is
an identity map rather than the doubling measured above) over 1,000 ints:

| Per-element work | Flow        | Loop        | Flow/Loop |
| ---------------- | ----------- | ----------- | --------- |
| 0 (identity map) | ~8.6 µs/op  | ~1.3 µs/op  | ~6.7x     |
| 10 iterations    | ~24.4 µs/op | ~16.6 µs/op | ~1.5x     |
| 100 iterations   | ~357 µs/op  | ~351 µs/op  | ~1.0x     |

The fixed ~7 ns/element overhead doesn't grow with the workload — the same
closures and `iter.Seq` machinery run either way — so for a pipeline doing
non-trivial work per element, Flow's overhead disappears into noise.

Where Flow *does* pay off is **early exit on a lazy source**: a
short-circuiting terminal (`First`, `Find`, `Any`) — or `Take`, which stops
the scan the same way without ending the chain — only generates the elements
it needs, so an expensive `FromFn` generator runs a handful of times instead
of N. Generating 1,000,000 elements through an expensive generator but
keeping only the first 3:

| Approach                     | Time       |
| ---------------------------- | ---------- |
| Flow (`FromFn(...).Take(3)`) | ~1.1 µs/op |
| Materialize all, then slice  | ~321 ms/op |

Here laziness turns an O(N) computation into O(1). Reach for Flow when the
pipeline reads clearly and either the data set is small or a lazy source lets
you stop early.

> Reproduce the figures in this document with one `-bench` pattern per
> process. Benchmarks sharing a process share its GC pressure and heap
> growth. This suite has a concrete case of that mattering, written up in
> `BenchmarkTakeDropStrategies`' own doc comment; that benchmark sources no
> figure here, but it shows the effect is real. The commands below keep the
> same one-pattern-per-process discipline as a precaution, even though no
> comparable distortion has been observed among them specifically:
>
> ```
> go test -bench '^BenchmarkVsLoop$' -benchmem -run '^$' .
> go test -bench '^BenchmarkVsLoopWorkload$' -benchmem -run '^$' .
> go test -bench '^BenchmarkEarlyExit$' -benchmem -run '^$' .
> go test -bench 'BenchmarkSortByStrategies/n=(100|10000)$' -benchmem -run '^$' .
> go test -bench 'BenchmarkSortByStrategies/n=1000000$' -benchmem -benchtime=6x -run '^$' .
> ```
>
> The `SortBy` table's n=1,000,000 rows need that `-benchtime=6x`: each
> iteration there costs hundreds of milliseconds to seconds, so the default
> iteration count is only 1-3 samples — too few to tell the two strategies
> apart. Two default-`-benchtime` runs on this machine put the identity/int
> time ratio anywhere from 0.8x to 1.24x before six iterations settled it near
> the ~1.0x reported above. The n=100 and n=10,000 rows in that same table
> came from the separate default-`-benchtime` command above instead, where the
> iteration count is already in the hundreds to hundreds of thousands and
> stable without forcing it.
