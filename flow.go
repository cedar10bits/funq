package funq

import (
	"cmp"
	"iter"
	"slices"
)

// Flow is an immutable, type-safe sequence of values.
//
// A Flow is a concrete value type (not an interface), which lets its
// transformation methods carry their own type parameter and therefore change
// the element type while keeping the chain fluent (see [Flow.Map],
// [Flow.FlatMap], [Flow.Fold]). Whether a Flow is eager or lazy is a property
// of its source: From wraps concrete data, FromFn computes on demand. That
// describes how elements are produced, not how many times a pipeline built
// on either one runs — see below.
//
// Intermediate operations build a pipeline; terminal operations run it. A
// terminal operation ends the chain: it returns something other than a Flow
// ([Flow.Slice], [Flow.Count], [Flow.Reduce], ...), or, in [Flow.Partition]'s
// case, two Flows that are already evaluated. [Flow.Seq] and [Flow.To] are
// not terminal — Seq hands back an iterator that runs the pipeline when
// ranged over, and To only applies a function to the Flow itself. A
// pipeline cannot be relied on to run only once: a terminal operation that
// needs the elements may re-run it from the source. [Flow.Count] and
// [Flow.IsEmpty] are the exception — they answer from the statically known
// count when there is one, without running anything. That leaves the
// caller two rules:
//
//  1. Keep the functions passed to Map/Filter/... pure: this package does not
//     guarantee when, how many times, or in what order they are invoked. An
//     impure one makes that schedule observable — a predicate that logs,
//     counts, or warms a cache fires once per element on every terminal
//     operation that traverses, and again whenever an intermediate operation
//     evaluates part of the pipeline early.
//  2. To traverse an expensive pipeline more than once, materialize it with
//     [Flow.Cache] so it runs exactly once.
//
// Under these rules evaluation timing is unobservable, and the implementation
// exploits that: some intermediate operations evaluate part of the pipeline
// when they are called. [Flow.Cache] and the sort methods always do;
// [Flow.Take], [Flow.Drop], [Flow.TakeWhile], [Flow.DropWhile] and
// [Flow.Reverse] do depending on how the Flow was built, and each method's
// doc states when. Materialize, in those docs, means what Cache does: the
// upstream pipeline runs once, there, and the result is a Flow of known
// count that later terminal operations do not re-run.
// docs/performance.md has the benchmarks behind those choices; the timings
// are today's behavior, not a contract.
//
// Two properties decide the conditional cases, and a caller can read both off
// the way the Flow was built:
//
//   - Whether its element count is statically known. [From], [FromFn] and
//     [Optional.AsFlow] establish it and materializing re-establishes it;
//     [Flow.Map], [Flow.Take], [Flow.Drop], [Flow.TakeWhile], [Flow.DropWhile],
//     [Flow.Reverse] and a [Flow.Concat] of inputs that all have one carry it;
//     [Flow.Filter] gives it up.
//   - Whether it is still random-access. [FromSeq] builds a forward-only Flow
//     directly; [Flow.FlatMap], [Flow.MapIndexed],
//     [Flow.DistinctBy], [Distinct], [Zip], [Chunk] and a [Flow.Concat] whose
//     inputs do not all have a known count leave the Flow forward-only
//     instead, with no statically known count either, until it is
//     materialized again. [Flow.Reverse] materializes such a Flow, and
//     [Flow.Count], [Flow.IsEmpty] and [Flow.Last] have to traverse it.
//
// A step that can fail belongs in [Groove], not in a Flow: Map/Filter/...
// take [Fp], not [Fe]. Convert it to [Fp] first (e.g. with [IgnoreError] or
// [PanicOnError]).
//
// The zero value is a valid empty Flow.
//
// A Flow never mutates its source, but does not defensively copy it either:
// From aliases the slice it is spread from (see [From]), so immutability of
// what a Flow yields depends on the caller leaving that source alone.
//
// A Flow is not safe for concurrent use: the re-runs behind rule 1 mean that
// sharing one across goroutines invokes its generator and transform functions
// concurrently, a data race as soon as any of them touch shared state.
// Confine a Flow to a single goroutine, or materialize it with [Flow.Cache]
// and share the resulting slice instead.
type Flow[T any] struct {
	at   func(int) (T, bool) // indexed access; nil means use seq
	seq  iter.Seq[T]         // sequential source; used when at == nil
	head int                 // first index (forward) or one-past-first (reverse)
	tail int                 // boundary index; head <= tail is forward, else reverse
	size int                 // element count, or sizeUnknown when not statically known
}

// sizeUnknown marks a Flow whose element count is not statically known, e.g.
// after Filter (which may drop elements) or for any sequential Flow.
//
// Invariant: a Flow with a known size is hole-free, i.e. at(i) reports ok for
// every i in [head, tail). Take/Drop rely on this to adjust bounds in O(1);
// TakeWhile/DropWhile rely on it to recover the surviving count from the
// physical width of the boundary each finds, once the input's own size was
// known.
const sizeUnknown = -1

// forward and backward are the step directions used to walk an indexed
// Flow's physical index: head <= tail is forward, else backward.
const (
	forward  = 1
	backward = -1
)

// From creates an eager Flow backed directly by the given elements, without
// copying. It returns an empty Flow when no elements are provided.
//
// When called with a slice spread (From(xs...)), the Flow aliases xs: the
// caller hands over ownership, and mutating xs afterward changes what the
// Flow (and any Flow derived from it) yields. Callers who cannot promise
// that should pass a copy (From(slices.Clone(xs)...)). Calls listing the
// elements directly (From(1, 2, 3)) allocate a fresh slice and are unaffected.
func From[T any](s ...T) Flow[T] {
	return fromSlice(s)
}

// fromSlice wraps s directly without copying. Callers must not retain s.
func fromSlice[T any](s []T) Flow[T] {
	n := len(s)
	return Flow[T]{
		at:   func(i int) (T, bool) { return s[i], true },
		head: 0,
		tail: n,
		size: n,
	}
}

// FromFn creates a lazy Flow of n elements, generating element i with fn(i).
// Elements are produced on demand. n <= 0 yields an empty Flow.
func FromFn[T any](n int, fn func(int) T) Flow[T] {
	n = max(n, 0)
	return Flow[T]{
		at:   func(i int) (T, bool) { return fn(i), true },
		head: 0,
		tail: n,
		size: n,
	}
}

// FromSeq creates a lazy Flow over the elements of seq: the way into funq from
// the standard library's iterator vocabulary, and the mirror image of
// [Flow.Seq]'s outbound direction. The result is forward-only with no
// statically known element count (see the [Flow] documentation's second
// bullet). FromSeq(nil) is the exception, returning the empty Flow, whose
// count is statically known to be zero.
//
// seq must be restartable: per the [Flow] documentation's rule 1, a terminal
// operation may range over it more than once. A single-use iter.Seq — one
// backed by a channel or a bufio.Scanner, for instance — must be [Flow.Cache]d
// first, or later terminal operations will see it already exhausted.
func FromSeq[T any](seq iter.Seq[T]) Flow[T] {
	if seq == nil {
		return empty[T]()
	}
	return fromSeq(seq)
}

func fromSeq[T any](seq iter.Seq[T]) Flow[T] {
	return Flow[T]{seq: seq, size: sizeUnknown}
}

func empty[T any]() Flow[T] { return Flow[T]{} }

// Seq returns a forward iterator over the elements of the Flow, in iteration
// order: the way out of funq into the standard library's iterator vocabulary
// (range-over-func, [slices.Collect], [slices.Sorted], ...). [FromSeq] is the
// way back in.
//
// The iterator is a view of the Flow, not a snapshot: each range over it may
// re-run the pipeline. Call [Flow.Cache] first when the pipeline is
// expensive and the iterator is consumed more than once. Breaking out of the
// range stops the pipeline where it is.
func (f Flow[T]) Seq() iter.Seq[T] {
	return func(yield func(T) bool) {
		if f.at != nil {
			// Walk the indexed representation directly rather than through
			// each/pull: Seq underlies nearly every terminal operation, so the
			// two closure hops those helpers add are measurable per element.
			dir, i, tail := forward, f.head, f.tail
			if f.head > f.tail {
				dir, i, tail = backward, f.head-1, f.tail-1
			}
			for ; i != tail; i += dir {
				if v, ok := f.at(i); ok && !yield(v) {
					return
				}
			}
			return
		}
		if f.seq == nil {
			return
		}
		for v := range f.seq {
			if !yield(v) {
				return
			}
		}
	}
}

// pull returns a stateful cursor over the live elements of an indexed Flow,
// in iteration order. Each call returns the physical index the next live
// element was found at, the element itself, and true; or false once
// exhausted. It is the shared traversal primitive behind each and Zip.
//
// pull must only be called when f.at != nil.
func (f Flow[T]) pull() func() (int, T, bool) {
	dir, i, tail := forward, f.head, f.tail
	if f.head > f.tail {
		dir, i, tail = backward, f.head-1, f.tail-1
	}
	return func() (idx int, v T, ok bool) {
		for i != tail {
			if v, ok = f.at(i); ok {
				idx = i
				i += dir
				return idx, v, true
			}
			i += dir
		}
		return i, v, false
	}
}

// each scans the live elements of an indexed Flow in iteration order, calling
// fn with the logical index (0-based, skipping holes) and value. fn returns
// false to stop. It returns the physical boundary suitable for narrowing the
// Flow's bounds, and whether iteration completed without stopping.
//
// each delegates index-walking to pull and adds logical-index counting plus
// direction-aware translation of pull's physical index into that boundary.
//
// each must only be called when f.at != nil.
func (f Flow[T]) each(fn func(idx int, v T) bool) (boundary int, completed bool) {
	dir := forward
	if f.head > f.tail {
		dir = backward
	}
	next := f.pull()
	logical := 0
	for {
		phys, v, ok := next()
		if !ok {
			return f.tail, true
		}
		if !fn(logical, v) {
			if dir == forward {
				return phys, false
			}
			return phys + 1, false
		}
		logical++
	}
}

// To applies fn to the Flow itself and returns fn's result. It is postfix
// function application (F#'s |> pipe), which keeps a chain fluent through
// the free functions that cannot be methods, such as [Chunk] and [Distinct]:
//
//	From(1, 2, 3, 4, 5).To(Chunk[int](2)).Map(...)  // continues as Flow[[]int]
//	From(1, 2, 2, 3).To(Distinct).Slice()           // [1 2 3]
//
// Unlike [Flow.Map], which transforms each element with a func(T) U, To
// passes the whole Flow to fn exactly once. The result may be any type, not
// just another Flow.
func (f Flow[T]) To[U any](fn func(Flow[T]) U) U {
	return fn(f)
}

// Map transforms each element with fn, possibly changing the element type;
// see the [Flow] documentation for why methods like this can carry their own
// type parameter.
func (f Flow[T]) Map[U any](fn func(T) U) Flow[U] {
	if f.at != nil {
		at := f.at
		return Flow[U]{
			at: func(i int) (U, bool) {
				v, ok := at(i)
				if !ok {
					var zero U
					return zero, false
				}
				return fn(v), true
			},
			head: f.head,
			tail: f.tail,
			size: f.size,
		}
	}
	if f.seq == nil {
		return empty[U]()
	}
	seq := f.seq
	return fromSeq(func(yield func(U) bool) {
		for v := range seq {
			if !yield(fn(v)) {
				return
			}
		}
	})
}

// MapIndexed transforms each element with fn, which also receives the
// element's 0-based position in iteration order.
//
// The position is logical, not a source index: it counts elements as they
// are yielded, so it stays 0, 1, 2, ... after Filter or Reverse. Unlike
// [Flow.Map], MapIndexed always leaves the Flow forward-only (see [Flow]).
func (f Flow[T]) MapIndexed[U any](fn func(int, T) U) Flow[U] {
	src := f
	return fromSeq(func(yield func(U) bool) {
		i := 0
		for v := range src.Seq() {
			if !yield(fn(i, v)) {
				return
			}
			i++
		}
	})
}

// FlatMap maps each element to a Flow and concatenates the results.
func (f Flow[T]) FlatMap[U any](fn func(T) Flow[U]) Flow[U] {
	src := f
	return fromSeq(func(yield func(U) bool) {
		for v := range src.Seq() {
			for u := range fn(v).Seq() {
				if !yield(u) {
					return
				}
			}
		}
	})
}

// Filter keeps only the elements that satisfy pred.
//
// Filter is lazy however the Flow was built: pred runs once per element on
// every traversal of the result, so use [Flow.Cache] when pred is expensive
// and the result is traversed more than once. Giving up the statically known
// count is also what makes a later [Flow.Drop] or [Flow.Take] scan eagerly.
//
// Chaining several Filter calls nests one closure per call around f.at,
// which measured roughly on par with dropping to the sequential
// representation at a chain depth of two, and clearly slower by a depth of
// four (BenchmarkFilterRepresentation) — combine the predicates with [And]
// into a single Filter call instead; that matches or beats the sequential
// chain's throughput at those depths while keeping the indexed representation
// (see the note below).
func (f Flow[T]) Filter(pred func(T) bool) Flow[T] {
	// Keeping the indexed representation (holes, see sizeUnknown) beats the
	// sequential representation for a single Filter call; the doc comment
	// above measures how that flips as calls chain
	// (BenchmarkFilterRepresentation). Each chained call nests another closure
	// inside f.at, while the sequential chain's nested range-over-func
	// generators scale better as the chain grows. This branch is kept
	// anyway, even at the depths where it loses, because indexed carries more
	// than raw Filter throughput: it is what lets [Flow.Reverse] stay O(1)
	// instead of materializing, and lets
	// [Flow.Drop]/[Flow.Take]/[Flow.DropWhile]/[Flow.TakeWhile] narrow
	// head/tail instead of rebuilding the pipeline.
	if f.at != nil {
		at := f.at
		return Flow[T]{
			at:   func(i int) (T, bool) { v, ok := at(i); return v, ok && pred(v) },
			head: f.head,
			tail: f.tail,
			size: sizeUnknown,
		}
	}
	if f.seq == nil {
		return f
	}
	seq := f.seq
	return fromSeq(func(yield func(T) bool) {
		for v := range seq {
			if pred(v) && !yield(v) {
				return
			}
		}
	})
}

// DistinctBy returns a Flow that yields each element of f once, keeping the
// first occurrence for each key extracted by key.
//
// DistinctBy is the method form of [Distinct]: its comparable constraint
// attaches to its own type parameter K rather than to T, which is why it can
// be a method (see [Distinct] for why the T-is-the-key form cannot).
// Distinct(f) is equivalent to f.DistinctBy(Identity).
func (f Flow[T]) DistinctBy[K comparable](key func(T) K) Flow[T] {
	return fromSeq(func(yield func(T) bool) {
		seen := make(map[K]struct{})
		for v := range f.Seq() {
			k := key(v)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			if !yield(v) {
				return
			}
		}
	})
}

// Drop skips the first n elements.
//
// Drop is O(1) and fully lazy while the element count is statically known,
// and lazy on a forward-only Flow, where it skips as it traverses; [Flow]
// describes both properties. In between — count unknown but the Flow not yet
// forward-only, e.g. straight after [Flow.Filter] — Drop scans the upstream
// pipeline once at construction time to locate the boundary and keeps only
// that boundary, so any predicate involved runs then and again on every later
// terminal operation; [Flow.Cache] before the Drop collapses that to a single
// run.
func (f Flow[T]) Drop(n int) Flow[T] {
	if n <= 0 {
		return f
	}
	if f.at == nil {
		if f.seq == nil {
			return f
		}
		seq := f.seq
		return fromSeq(func(yield func(T) bool) {
			i := 0
			for v := range seq {
				if i < n {
					i++
					continue
				}
				if !yield(v) {
					return
				}
			}
		})
	}
	if f.size != sizeUnknown {
		if n >= f.size {
			return empty[T]()
		}
		step := n
		if f.head > f.tail {
			step = -n
		}
		return Flow[T]{at: f.at, head: f.head + step, tail: f.tail, size: f.size - n}
	}
	// Scanning eagerly beat staying lazy (which costs the at(i) fast path for
	// the whole downstream traversal) even on a single terminal call; see
	// BenchmarkTakeDropStrategies.
	boundary, completed := f.each(func(idx int, _ T) bool { return idx < n })
	if completed {
		return empty[T]()
	}
	return Flow[T]{at: f.at, head: boundary, tail: f.tail, size: sizeUnknown}
}

// Take keeps the first n elements.
//
// Take branches as [Flow.Drop] does and on the same conditions, with one
// difference in the scanning case: it keeps the elements the scan produced
// rather than the boundary, so the result is materialized (see [Flow]) and
// the upstream predicate or generator runs once per scanned element in total.
// That scan reaches no further than the n-th surviving element, so a
// short-circuiting Take over a large lazy source stays cheap; it runs to the
// end only when fewer than n survive, since that is the only way to establish
// there are no more.
func (f Flow[T]) Take(n int) Flow[T] {
	if n <= 0 {
		return empty[T]()
	}
	if f.at == nil {
		if f.seq == nil {
			return f
		}
		seq := f.seq
		return fromSeq(func(yield func(T) bool) {
			i := 0
			for v := range seq {
				if !yield(v) {
					return
				}
				if i++; i >= n {
					return
				}
			}
		})
	}
	if f.size != sizeUnknown {
		if n >= f.size {
			return f
		}
		step := n
		if f.head > f.tail {
			step = -n
		}
		return Flow[T]{at: f.at, head: f.head, tail: f.head + step, size: n}
	}
	// Keeping what the scan produces, rather than a boundary the pipeline has
	// to reproduce per terminal call, wins on both measured shapes: see
	// BenchmarkTakeDropStrategies and BenchmarkTakeShortCircuit.
	//
	// The physical range width is an exact upper bound on the live element
	// count (as in Slice), so it caps the preallocation when n is far larger
	// than the Flow, e.g. Take(math.MaxInt).
	width := f.tail - f.head
	if width < 0 {
		width = -width
	}
	out := make([]T, 0, min(n, width))
	next := f.pull()
	for len(out) < n {
		_, v, ok := next()
		if !ok {
			break
		}
		out = append(out, v)
	}
	return fromSlice(out)
}

// DropWhile skips leading elements while pred is true.
//
// pred determines the boundary, so DropWhile scans the upstream pipeline once
// at construction time exactly as [Flow.Drop] does in its scanning case, but
// whether or not the element count is known; only a forward-only Flow stays
// fully lazy. When the input's count was statically known, the result's is
// too — recovered in O(1) from the width of the scanned boundary, the same
// way [Flow.Drop] adjusts its bounds.
func (f Flow[T]) DropWhile(pred func(T) bool) Flow[T] {
	if f.at == nil {
		if f.seq == nil {
			return f
		}
		seq := f.seq
		return fromSeq(func(yield func(T) bool) {
			dropping := true
			for v := range seq {
				if dropping {
					if pred(v) {
						continue
					}
					dropping = false
				}
				if !yield(v) {
					return
				}
			}
		})
	}
	boundary, completed := f.each(func(_ int, v T) bool { return pred(v) })
	if completed {
		return empty[T]()
	}
	size := sizeUnknown
	if f.size != sizeUnknown {
		// The surviving range is [boundary, f.tail): hole-free, since a known
		// size implies as much (sizeUnknown's doc comment), so its width is the
		// exact count.
		size = f.tail - boundary
		if size < 0 {
			size = -size
		}
	}
	return Flow[T]{at: f.at, head: boundary, tail: f.tail, size: size}
}

// TakeWhile keeps leading elements while pred is true.
//
// TakeWhile scans eagerly under the same condition and for the same reason as
// [Flow.DropWhile]: pred determines the boundary, and when the input's count
// was statically known, the result's is too, recovered the same way. When
// pred held for every element it returns the Flow unchanged, statically known
// count and all.
//
// Unlike DropWhile, the range TakeWhile keeps is exactly the range its
// construction scan already walked, not the disjoint remainder — so an
// expensive pred or upstream pipeline runs once at construction and again in
// full on every later terminal operation. [Flow.Cache] before the TakeWhile
// collapses that to a single run, the same remedy as [Flow.Drop]'s.
func (f Flow[T]) TakeWhile(pred func(T) bool) Flow[T] {
	if f.at == nil {
		if f.seq == nil {
			return f
		}
		seq := f.seq
		return fromSeq(func(yield func(T) bool) {
			for v := range seq {
				if !pred(v) || !yield(v) {
					return
				}
			}
		})
	}
	boundary, completed := f.each(func(_ int, v T) bool { return pred(v) })
	if completed {
		return f
	}
	size := sizeUnknown
	if f.size != sizeUnknown {
		// The surviving range is [f.head, boundary): hole-free, since a known
		// size implies as much (sizeUnknown's doc comment), so its width is the
		// exact count.
		size = boundary - f.head
		if size < 0 {
			size = -size
		}
	}
	return Flow[T]{at: f.at, head: f.head, tail: boundary, size: size}
}

// Reverse returns the elements in reverse order.
//
// Reverse is O(1) and fully lazy unless the Flow is forward-only (see
// [Flow]): such a Flow cannot be walked backwards, so Reverse materializes it
// at construction time instead.
func (f Flow[T]) Reverse() Flow[T] {
	if f.at != nil {
		return Flow[T]{at: f.at, head: f.tail, tail: f.head, size: f.size}
	}
	if f.seq == nil {
		return f
	}
	s := f.Slice()
	slices.Reverse(s)
	return fromSlice(s)
}

// SortFunc sorts the elements using cmp, which reports a negative number to
// mean a sorts before b, a positive number to mean a sorts after b, and zero
// to mean they're equal (the same contract as [slices.SortFunc]). The sort is
// stable.
//
// Sorting needs every element up front, so SortFunc materializes the Flow
// immediately (see [Flow]).
func (f Flow[T]) SortFunc(cmp func(a, b T) int) Flow[T] {
	s := f.Slice()
	slices.SortStableFunc(s, cmp)
	return fromSlice(s)
}

// SortBy sorts the elements in ascending order of the key extracted by key.
// The sort is stable, as in [Flow.SortFunc].
//
// key is called exactly once per element. It costs three slices of len n
// that [Flow.SortFunc] does not: the keys, a permutation of indices, and a
// fresh output slice.
//
// Keys order through [cmp.Compare], so a NaN key sorts before every non-NaN
// key — the same order [Flow.MinBy] and [Flow.MaxBy] use.
//
// SortBy materializes the Flow immediately; see [Flow.SortFunc].
func (f Flow[T]) SortBy[K cmp.Ordered](key func(T) K) Flow[T] {
	// Sorting a permutation of indices beats the rejected alternative, handing
	// key to the comparator, in every shape measured but one: with an identity
	// key over ints the two draw even on time, and the permutation costs more
	// memory. The margin grows with the cost of key and the size of the
	// element, which is the trade this takes. See docs/performance.md and
	// BenchmarkSortByStrategies.
	s := f.Slice()
	keys := make([]K, len(s))
	idx := make([]int, len(s))
	for i, v := range s {
		keys[i] = key(v)
		idx[i] = i
	}
	// idx starts in ascending order, so a stable sort leaves equal keys in
	// their original relative order.
	slices.SortStableFunc(idx, func(a, b int) int { return cmp.Compare(keys[a], keys[b]) })
	out := make([]T, len(s))
	for i, j := range idx {
		out[i] = s[j]
	}
	return fromSlice(out)
}

// Concat returns a Flow that yields the elements of f followed by the
// elements of each of others, in order.
//
// Concat evaluates nothing at construction time. When every input's element
// count is statically known (see [Flow] — a Flow left empty by [Flow.Filter]
// does not qualify), the result's is too, preserving O(1) [Flow.Reverse] and
// constant-time [Flow.Take] and [Flow.Drop]; otherwise the result is
// forward-only.
//
// Pass every Flow in one call. Reading an element from the result costs
// O(#inputs) and concatenating a result nests that cost, so accumulating in a
// loop (f = f.Concat(x)) makes a traversal quadratic in the number of
// iterations. Collect the parts first and call f.Concat(parts...) once, or
// insert [Flow.Cache] to flatten a chain already built that way.
func (f Flow[T]) Concat(others ...Flow[T]) Flow[T] {
	all := make([]Flow[T], 0, len(others)+1)
	all = append(all, f)
	all = append(all, others...)
	if out, ok := concatIndexed(all); ok {
		return out
	}
	return fromSeq(func(yield func(T) bool) {
		for _, flow := range all {
			for v := range flow.Seq() {
				if !yield(v) {
					return
				}
			}
		}
	})
}

// concatIndexed builds an indexed concatenation when every input has a known
// size. A known size implies a hole-free at (see sizeUnknown), so each input's
// logical positions map to physical indices by a fixed base and direction, and
// the combined at can dispatch by offset. It reports false when any input is
// sequential or of unknown size, forcing the sequential fallback.
//
// The dispatch scans the segment table linearly, so lookups cost O(#inputs);
// Concat is expected to join a handful of Flows, not thousands.
func concatIndexed[T any](all []Flow[T]) (Flow[T], bool) {
	type segment struct {
		start int // logical index of the segment's first element
		base  int // physical index of that element in its source
		dir   int // forward or backward step within the source
		at    func(int) (T, bool)
	}
	segs := make([]segment, 0, len(all))
	total := 0
	for _, fl := range all {
		if fl.size == sizeUnknown {
			return Flow[T]{}, false
		}
		if fl.size == 0 {
			continue
		}
		base, dir := fl.head, forward
		if fl.head > fl.tail {
			base, dir = fl.head-1, backward
		}
		segs = append(segs, segment{start: total, base: base, dir: dir, at: fl.at})
		total += fl.size
	}
	at := func(i int) (T, bool) {
		k := len(segs) - 1
		for segs[k].start > i {
			k--
		}
		s := segs[k]
		return s.at(s.base + (i-s.start)*s.dir)
	}
	return Flow[T]{at: at, head: 0, tail: total, size: total}, true
}

// Cache materializes the Flow (see [Flow]) so later traversals do not
// recompute it.
//
// Cache always copies every element into a fresh slice, even from an already
// eager Flow, so on a pure pipeline with a single terminal operation it buys
// nothing and adds an allocation.
func (f Flow[T]) Cache() Flow[T] {
	return fromSlice(f.Slice())
}

// asSeq normalizes f to the sequential representation: a no-op if f is
// already sequential, otherwise its elements pulled through Seq() into a
// fresh sequential Flow. It is Cache's counterpart for the other internal
// representation, forcing the lazy evaluation path even when f is indexed.
func (f Flow[T]) asSeq() Flow[T] {
	if f.at == nil {
		return f
	}
	return fromSeq(f.Seq())
}

// ForEach applies fn to each element for its side effects.
func (f Flow[T]) ForEach(fn func(T)) {
	for v := range f.Seq() {
		fn(v)
	}
}

// ForEachIndexed applies fn to each element for its side effects, along with
// the element's 0-based position in iteration order (logical, as in
// [Flow.MapIndexed]).
func (f Flow[T]) ForEachIndexed(fn func(int, T)) {
	i := 0
	for v := range f.Seq() {
		fn(i, v)
		i++
	}
}

// Fold reduces the Flow into a single accumulator value, starting from init.
// Unlike Reduce, the accumulator may be of a different type than the elements.
func (f Flow[T]) Fold[U any](init U, fn func(U, T) U) U {
	acc := init
	for v := range f.Seq() {
		acc = fn(acc, v)
	}
	return acc
}

// Reduce combines all elements with fn, returning None for an empty Flow.
func (f Flow[T]) Reduce(fn func(T, T) T) Optional[T] {
	var acc T
	found := false
	for v := range f.Seq() {
		if !found {
			acc, found = v, true
		} else {
			acc = fn(acc, v)
		}
	}
	if !found {
		return None[T]()
	}
	return Some(acc)
}

// Slice materializes the Flow into a new slice.
func (f Flow[T]) Slice() []T {
	if f.size != sizeUnknown {
		// A known size is exact (hole-free invariant), so fill by index and
		// skip append's bounds checks.
		out := make([]T, f.size)
		i := 0
		for v := range f.Seq() {
			out[i] = v
			i++
		}
		return out
	}
	// For an indexed Flow of unknown size (e.g. after Filter), the physical
	// range width is an exact upper bound on the element count: pre-allocate
	// it to avoid append regrowth, trading possible over-allocation when the
	// filter drops many elements. A sequential Flow offers no such bound.
	capHint := 0
	if f.at != nil {
		capHint = f.tail - f.head
		if capHint < 0 {
			capHint = -capHint
		}
	}
	out := make([]T, 0, capHint)
	for v := range f.Seq() {
		out = append(out, v)
	}
	return out
}

// Count returns the number of elements in the Flow.
func (f Flow[T]) Count() int {
	if f.size != sizeUnknown {
		return f.size
	}
	n := 0
	for range f.Seq() {
		n++
	}
	return n
}

// IsEmpty reports whether the Flow has no elements.
func (f Flow[T]) IsEmpty() bool {
	if f.size != sizeUnknown {
		return f.size == 0
	}
	for range f.Seq() {
		return false
	}
	return true
}

// Any reports whether any element satisfies pred.
//
// For a membership test on a comparable element type, use the free function
// [Contains].
func (f Flow[T]) Any(pred func(T) bool) bool {
	for v := range f.Seq() {
		if pred(v) {
			return true
		}
	}
	return false
}

// All reports whether every element satisfies pred (true for an empty Flow).
func (f Flow[T]) All(pred func(T) bool) bool {
	for v := range f.Seq() {
		if !pred(v) {
			return false
		}
	}
	return true
}

// Find returns the first element satisfying pred, wrapped in Optional.
func (f Flow[T]) Find(pred func(T) bool) Optional[T] {
	for v := range f.Seq() {
		if pred(v) {
			return Some(v)
		}
	}
	return None[T]()
}

// First returns the first element, wrapped in Optional.
func (f Flow[T]) First() Optional[T] {
	for v := range f.Seq() {
		return Some(v)
	}
	return None[T]()
}

// Last returns the last element, wrapped in Optional.
func (f Flow[T]) Last() Optional[T] {
	// An indexed Flow reverses in O(1), so First on it is cheap. A sequential
	// Flow would otherwise be fully materialized by Reverse, so iterate once
	// and keep the most recent value instead.
	if f.at != nil {
		return f.Reverse().First()
	}
	last := None[T]()
	for v := range f.Seq() {
		last = Some(v)
	}
	return last
}

// MinBy returns the element with the smallest key, wrapped in Optional. When
// several elements share the smallest key, the first one wins. It returns
// None for an empty Flow.
//
// Pass [Identity] as key to compare the elements themselves. There is no
// element-wise Min method, for the same reason as [Distinct]. A free
// function is not provided either, since the name is taken by the builtin
// `min`.
//
// A NaN key orders before every non-NaN key, as under [cmp.Compare] and
// [Flow.SortBy], so a NaN key wins wherever it appears in the Flow. Among
// several NaN keys the first wins, as for any other tie.
func (f Flow[T]) MinBy[K cmp.Ordered](key func(T) K) Optional[T] {
	best := None[T]()
	var bestKey K
	for v := range f.Seq() {
		k := key(v)
		if best.IsEmpty() || k < bestKey || (isNaN(k) && !isNaN(bestKey)) {
			best, bestKey = Some(v), k
		}
	}
	return best
}

// MaxBy returns the element with the largest key, wrapped in Optional. When
// several elements share the largest key, the first one wins. It returns
// None for an empty Flow.
//
// Pass [Identity] as key to compare the elements themselves; see [Flow.MinBy]
// for why there is no element-wise Max method.
//
// A NaN key orders before every non-NaN key (see [Flow.MinBy]), so a NaN key
// never wins unless every key is NaN.
func (f Flow[T]) MaxBy[K cmp.Ordered](key func(T) K) Optional[T] {
	best := None[T]()
	var bestKey K
	for v := range f.Seq() {
		k := key(v)
		if best.IsEmpty() || k > bestKey || (isNaN(bestKey) && !isNaN(k)) {
			best, bestKey = Some(v), k
		}
	}
	return best
}

// isNaN reports whether k is a NaN without pulling in the math package. It is
// always false for a non-floating-point K. Mirrors cmp.isNaN, which the
// standard library keeps unexported.
func isNaN[K cmp.Ordered](k K) bool { return k != k }

// GroupBy groups elements by the key extracted by key. Each group preserves
// the relative order in which its elements appeared in the Flow.
func (f Flow[T]) GroupBy[K comparable](key func(T) K) map[K][]T {
	out := make(map[K][]T)
	for v := range f.Seq() {
		k := key(v)
		out[k] = append(out[k], v)
	}
	return out
}

// ToMap collects the elements into a map keyed by key. When two elements map
// to the same key, the first occurrence wins, consistent with
// [Flow.DistinctBy].
func (f Flow[T]) ToMap[K comparable](key func(T) K) map[K]T {
	out := make(map[K]T)
	for v := range f.Seq() {
		k := key(v)
		if _, ok := out[k]; ok {
			continue
		}
		out[k] = v
	}
	return out
}

// Partition splits the elements into those satisfying pred and those that do
// not, preserving relative order within each side. Both results are
// materialized (see [Flow]), so the upstream pipeline runs exactly once.
func (f Flow[T]) Partition(pred func(T) bool) (matched, rest Flow[T]) {
	var yes, no []T
	for v := range f.Seq() {
		if pred(v) {
			yes = append(yes, v)
		} else {
			no = append(no, v)
		}
	}
	return fromSlice(yes), fromSlice(no)
}

// Pair holds two values of possibly different types, e.g. the result of Zip.
type Pair[A, B any] struct {
	First  A
	Second B
}

// Zip pairs up the elements of a and b by position, stopping as soon as
// either Flow runs out of elements.
//
// Zip cannot be a method: a method on Flow[T] whose signature instantiates
// Flow with a type argument derived from T (here Flow[Pair[T, U]]) is
// rejected with an "instantiation cycle" error.
func Zip[T, U any](a Flow[T], b Flow[U]) Flow[Pair[T, U]] {
	// That restriction is a longstanding one in gc and go/types, not mandated
	// by the spec and not specific to parameterized methods: it predates Go
	// 1.27 and also rejects a non-generic method returning Flow[[]T]. Flow[T]'s
	// method set would reference Flow[Pair[T, U]], whose method set references
	// Flow[Pair[Pair[T, U], V]], without bound, so the compiler rejects it.
	return fromSeq(func(yield func(Pair[T, U]) bool) {
		// One side is ranged over (the driver), the other pulled one element
		// at a time. An indexed side can be pulled directly, so prefer it as
		// the pulled side: iter.Pull's coroutine adapter, the only way to
		// pull a sequential side, costs an order of magnitude more per
		// element (see BenchmarkZip). Pairs are positional, so swapping
		// which side drives does not change the result.
		if b.at == nil && a.at != nil {
			pull := a.pull()
			for u := range b.Seq() {
				_, v, ok := pull()
				if !ok || !yield(Pair[T, U]{v, u}) {
					return
				}
			}
			return
		}
		var next func() (U, bool)
		if b.at != nil {
			pull := b.pull()
			next = func() (U, bool) { _, v, ok := pull(); return v, ok }
		} else {
			var stop func()
			next, stop = iter.Pull(b.Seq())
			defer stop()
		}
		for v := range a.Seq() {
			u, ok := next()
			if !ok {
				return
			}
			if !yield(Pair[T, U]{v, u}) {
				return
			}
		}
	})
}

// Distinct returns a Flow that yields each distinct element of f once, in
// first-occurrence order.
//
// Distinct cannot be a method: it requires T itself to satisfy comparable,
// which a parameterized method cannot express (unlike [Flow.DistinctBy],
// whose constraint attaches to its own new type parameter, not the
// receiver's T). Use DistinctBy when T is not comparable, or to dedupe by a
// derived key instead of the whole value. In a chain, [Flow.To] applies it
// postfix: f.To(Distinct).
func Distinct[T comparable](f Flow[T]) Flow[T] {
	return f.DistinctBy(Identity)
}

// Contains reports whether v is among the elements of f. It is equivalent to
// f.Any(Equal(v)).
//
// Contains cannot be a method, for the same reason as [Distinct]. Use
// [Flow.Any] with a custom predicate when T is not comparable or the match
// is looser than equality.
func Contains[T comparable](f Flow[T], v T) bool {
	return f.Any(Equal(v))
}

// Chunk returns a function that groups a Flow's consecutive elements into
// slices of length n; the final chunk may be shorter. It panics if n is less
// than 1, matching [slices.Chunk]. The returned Flow is lazy. Each yielded
// slice is freshly allocated (it does not alias the Flow's backing storage
// or previously yielded chunks).
//
// Chunk cannot be a Flow method, for the same reason as [Zip]: its return
// type instantiates Flow with a type argument derived from the receiver's T
// (Flow[T] -> Flow[[]T]). The curried form plugs directly into [Flow.To]
// (f.To(Chunk[T](n))) and, matching [Fp]'s shape, into a [Compose] chain;
// wrap it with [NilError] first to use it in a [Groove] pipeline.
func Chunk[T any](n int) func(Flow[T]) Flow[[]T] {
	if n < 1 {
		panic("funq: Chunk called with n < 1")
	}
	return func(f Flow[T]) Flow[[]T] {
		return fromSeq(func(yield func([]T) bool) {
			chunk := make([]T, 0, n)
			for v := range f.Seq() {
				chunk = append(chunk, v)
				if len(chunk) == n {
					if !yield(chunk) {
						return
					}
					chunk = make([]T, 0, n)
				}
			}
			if len(chunk) > 0 {
				yield(chunk)
			}
		})
	}
}
