package funq

import (
	"fmt"
	"iter"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// seqFilter and seqMap are the stdlib-flavored combinators the iter package
// itself does not provide. BenchmarkVsLoop's "stdlib" subbench uses them to
// represent the realistic alternative to Flow: function-composition style
// built directly on iter.Seq rather than a hand-written loop.
func seqFilter[T any](s iter.Seq[T], pred func(T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range s {
			if pred(v) && !yield(v) {
				return
			}
		}
	}
}

func seqMap[T, U any](s iter.Seq[T], f func(T) U) iter.Seq[U] {
	return func(yield func(U) bool) {
		for v := range s {
			if !yield(f(v)) {
				return
			}
		}
	}
}

func BenchmarkSmall(b *testing.B) {
	runAllBenchmarks(10, b)
}

func BenchmarkMedium(b *testing.B) {
	runAllBenchmarks(1_000, b)
}

func BenchmarkLarge(b *testing.B) {
	runAllBenchmarks(1_000_000, b)
}

// BenchmarkVsLoop compares a representative Flow pipeline against an equivalent
// hand-written loop, to quantify the per-element overhead of the fluent API.
func BenchmarkVsLoop(b *testing.B) {
	const n = 1_000
	src := make([]int, n)
	for i := range src {
		src[i] = i
	}

	b.Run("flow", func(b *testing.B) {
		for b.Loop() {
			_ = From(src...).Filter(even).Map(mul2).Reduce(add).OrElse(0)
		}
	})
	b.Run("loop", func(b *testing.B) {
		for b.Loop() {
			sum := 0
			for _, v := range src {
				if v%2 == 0 {
					sum += v * 2
				}
			}
			_ = sum
		}
	})
	b.Run("stdlib", func(b *testing.B) {
		for b.Loop() {
			sum := 0
			s := seqMap(seqFilter(slices.Values(src), even), mul2)
			for v := range s {
				sum += v
			}
			_ = sum
		}
	})
}

// BenchmarkVsLoopWorkload extends BenchmarkVsLoop by varying the per-element
// cost of the map stage. As real work grows, the fixed per-call overhead of
// Flow's closures becomes a smaller fraction of the total, so the flow/loop
// ratio shrinks toward parity. The work=0 case is the same shape as
// BenchmarkVsLoop with an identity map instead of doubling.
//
// docs/performance.md holds the measured ratios.
func BenchmarkVsLoopWorkload(b *testing.B) {
	const n = 1_000
	src := make([]int, n)
	for i := range src {
		src[i] = i
	}

	work := func(v, iterations int) int {
		x := v
		for range iterations {
			x = (x*31 + 7) % 1_000_003
		}
		return x
	}

	for _, w := range []int{0, 10, 100} {
		b.Run(fmt.Sprintf("work=%d/flow", w), func(b *testing.B) {
			for b.Loop() {
				_ = From(src...).Filter(even).Map(func(v int) int { return work(v, w) }).Reduce(add).OrElse(0)
			}
		})
		b.Run(fmt.Sprintf("work=%d/loop", w), func(b *testing.B) {
			for b.Loop() {
				sum := 0
				for _, v := range src {
					if v%2 == 0 {
						sum += work(v, w)
					}
				}
				_ = sum
			}
		})
	}
}

// BenchmarkEarlyExit shows where a lazy Flow pays off: only the elements needed
// to satisfy a short-circuiting terminal are generated, so the expensive
// generator runs a handful of times instead of a million. The "eager" baseline
// materializes the whole slice first.
func BenchmarkEarlyExit(b *testing.B) {
	const n = 1_000_000
	expensive := func(i int) int {
		x := i
		for range 50 {
			x = (x*31 + 7) % 1_000_003
		}
		return x
	}

	b.Run("lazy", func(b *testing.B) {
		for b.Loop() {
			_ = FromFn(n, expensive).Take(3).Slice()
		}
	})
	b.Run("eager", func(b *testing.B) {
		for b.Loop() {
			s := make([]int, n)
			for i := range s {
				s[i] = expensive(i)
			}
			_ = s[:3]
		}
	})
}

func BenchmarkDistinctBy(b *testing.B) {
	for _, n := range []int{1_000, 100_000} {
		unique := make([]int, n)
		duplicates := make([]int, n)
		for i := range unique {
			unique[i] = i
			duplicates[i] = i % 16
		}

		b.Run(fmt.Sprintf("n=%d/First", n), func(b *testing.B) {
			b.ReportAllocs()
			var got int
			for b.Loop() {
				got = From(unique...).DistinctBy(Identity).First().OrElse(-1)
			}
			if got != 0 {
				b.Fatalf("First() = %d, want 0", got)
			}
		})
		b.Run(fmt.Sprintf("n=%d/Take10", n), func(b *testing.B) {
			b.ReportAllocs()
			var got int
			for b.Loop() {
				got = From(duplicates...).DistinctBy(Identity).Take(10).Count()
			}
			if got != 10 {
				b.Fatalf("Take(10).Count() = %d, want 10", got)
			}
		})
		b.Run(fmt.Sprintf("n=%d/FullDuplicates", n), func(b *testing.B) {
			b.ReportAllocs()
			var got int
			for b.Loop() {
				got = From(duplicates...).DistinctBy(Identity).Count()
			}
			if got != 16 {
				b.Fatalf("Count() = %d, want 16", got)
			}
		})
		b.Run(fmt.Sprintf("n=%d/FullUnique", n), func(b *testing.B) {
			b.ReportAllocs()
			var got int
			for b.Loop() {
				got = From(unique...).DistinctBy(Identity).Count()
			}
			if got != n {
				b.Fatalf("Count() = %d, want %d", got, n)
			}
		})
	}
}

func BenchmarkMapCapacity(b *testing.B) {
	for _, n := range []int{1_000, 100_000} {
		for _, scenario := range []struct {
			name string
			keys int
		}{
			{"duplicates_16", 16},
			{"one_percent", n / 100},
			{"unique", n},
		} {
			values := make([]int, n)
			pairs := make([]Pair[int, int], n)
			for i := range values {
				values[i] = i
				pairs[i] = Pair[int, int]{First: i % scenario.keys, Second: i}
			}
			valueFlow := From(values...)
			pairFlow := From(pairs...)
			b.Run(fmt.Sprintf("n=%d/%s/ToMap", n, scenario.name), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					valueFlow.ToMap(func(v int) int { return v % scenario.keys })
				}
			})
			b.Run(fmt.Sprintf("n=%d/%s/MapOf", n, scenario.name), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					MapOf(pairFlow)
				}
			})
		}
	}
}

// BenchmarkZip measures Zip across the four representation combinations
// (idx_idx, idx_seq, seq_idx, seq_seq), backing the pulled-side preference
// explained in Zip's own implementation comment in flow.go.
//
// Result (Apple M2, five 200 ms samples, n=10,000): each combination with an
// indexed side lands near the indexed floor, since it can pull that side
// directly:
//
//   - idx_idx: ~5.7 ns
//   - idx_seq: ~6.7 ns
//   - seq_idx: ~7.4 ns
//
// seq_seq, the only combination with neither side indexed, costs an order of
// magnitude more at ~59 ns, including iter.Pull's coroutine costs.
// seq_seq_first is the early-exit case (~0.4 µs). docs/performance.md records
// why Zip keeps iter.Pull rather than collecting b.
func BenchmarkZip(b *testing.B) {
	const n = 10_000
	indexed := func() Flow[int] { return FromFn(n, Identity) }
	sequential := func() Flow[int] { return FromFn(n, Identity).asSeq() }

	cases := []struct {
		name string
		a, b func() Flow[int]
	}{
		{"idx_idx", indexed, indexed},
		{"idx_seq", indexed, sequential},
		{"seq_idx", sequential, indexed},
		{"seq_seq", sequential, sequential},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				for range Zip(tc.a(), tc.b()).Seq() {
				}
			}
		})
	}
	b.Run("seq_seq_first", func(b *testing.B) {
		for b.Loop() {
			Zip(sequential(), sequential()).First()
		}
	})
}

func BenchmarkIndexedFastPath(b *testing.B) {
	for _, sequential := range []bool{false, true} {
		name := "indexed"
		if sequential {
			name = "forward"
		}
		b.Run(name, func(b *testing.B) {
			input := FromFn(1000, Identity)
			if sequential {
				input = input.asSeq()
			}
			b.Run("MapIndexed", func(b *testing.B) {
				b.Run("ReverseFirst", func(b *testing.B) {
					for b.Loop() {
						input.MapIndexed(func(i, v int) int { return i + v }).Reverse().First()
					}
				})
				b.Run("DropSlice", func(b *testing.B) {
					for b.Loop() {
						input.MapIndexed(func(i, v int) int { return i + v }).Drop(990).Slice()
					}
				})
				b.Run("Slice", func(b *testing.B) {
					for b.Loop() {
						input.MapIndexed(func(i, v int) int { return i + v }).Slice()
					}
				})
			})
			b.Run("Zip", func(b *testing.B) {
				b.Run("ReverseFirst", func(b *testing.B) {
					for b.Loop() {
						Zip(input, input).Reverse().First()
					}
				})
				b.Run("DropSlice", func(b *testing.B) {
					for b.Loop() {
						Zip(input, input).Drop(990).Slice()
					}
				})
				b.Run("Slice", func(b *testing.B) {
					for b.Loop() {
						Zip(input, input).Slice()
					}
				})
			})
		})
	}
}

func BenchmarkSortByTraversal(b *testing.B) {
	input := FromFn(10000, func(i int) int { return 10000 - i })
	b.Run("Slice", func(b *testing.B) {
		for b.Loop() {
			input.SortBy(Identity).Slice()
		}
	})
	b.Run("Repeated", func(b *testing.B) {
		sorted := input.SortBy(Identity)
		for b.Loop() {
			for range 10 {
				for range sorted.Seq() {
				}
			}
		}
	})
}

// dropLazy and takeLazy are lazy alternatives to Flow's Drop/Take for the
// sizeUnknown indexed case (e.g. after Filter), where those two scan eagerly.
// dropWhileLazy and takeWhileLazy are lazy alternatives to DropWhile/TakeWhile
// for any indexed case, known size or not, since those two always scan
// eagerly. All four fall back to the sequential path with
// f.asSeq() instead of consuming the pipeline eagerly at construction time.
// They exist only to justify, via BenchmarkTakeDropStrategies and
// BenchmarkTakeShortCircuit, why flow.go keeps the eager implementation
// despite its up-front evaluation cost (see the Flow.Drop and Flow.Take
// doc comments).

func dropLazy[T any](f Flow[T], n int) Flow[T] {
	if f.at == nil || f.size != sizeUnknown {
		return f.Drop(n)
	}
	return f.asSeq().Drop(n)
}

func takeLazy[T any](f Flow[T], n int) Flow[T] {
	if f.at == nil || f.size != sizeUnknown {
		return f.Take(n)
	}
	return f.asSeq().Take(n)
}

func dropWhileLazy[T any](f Flow[T], pred func(T) bool) Flow[T] {
	if f.at == nil {
		return f.DropWhile(pred)
	}
	return f.asSeq().DropWhile(pred)
}

func takeWhileLazy[T any](f Flow[T], pred func(T) bool) Flow[T] {
	if f.at == nil {
		return f.TakeWhile(pred)
	}
	return f.asSeq().TakeWhile(pred)
}

// BenchmarkTakeDropStrategies compares the eager implementation of
// Drop/Take/DropWhile/TakeWhile against a lazy alternative, for a sizeUnknown
// indexed Flow (produced by Filter). Both a small prefix (cut=20) and a large
// one (cut=n/2) are measured, each consumed once and three times, to see
// whether eager's up-front scan pays off across repeated terminal calls.
//
// Result (Apple M2, n=10,000): eager wins in every combination tested,
// including the single-terminal cases the lazy approach was expected to win.
// Converting to the sequential representation loses the indexed at() fast
// path for the whole downstream traversal, which costs more than the extra
// eager scan saves. flow.go therefore keeps the eager implementation for all
// four methods.
//
// Re-measure this benchmark on its own (-bench 'BenchmarkTakeDropStrategies').
// Running the whole benchmark set in one process perturbs it enough to invert
// the cut=5000 Take and TakeWhile single-terminal cases, which is a GC-pressure
// artifact of the shared process rather than a result about the two strategies.
//
// The filter here keeps every element, so the boundary always sits exactly at
// cut and no combination measured below ever scans past it. That blind spot
// hid a regression in Take. BenchmarkTakeShortCircuit covers the
// selective-filter shape instead.
func BenchmarkTakeDropStrategies(b *testing.B) {
	const n = 10_000
	src := make([]int, n)
	for i := range src {
		src[i] = i
	}
	indexedUnknown := func() Flow[int] { return From(src...).Filter(func(int) bool { return true }) }

	consume := func(times int, mk func() Flow[int]) func(b *testing.B) {
		return func(b *testing.B) {
			for b.Loop() {
				out := mk()
				for range times {
					_ = out.Slice()
				}
			}
		}
	}

	for _, cut := range []int{20, n / 2} {
		b.Run(fmt.Sprintf("cut=%d", cut), func(b *testing.B) {
			cases := []struct {
				name  string
				eager func() Flow[int]
				lazy  func() Flow[int]
			}{
				{
					name:  "Drop",
					eager: func() Flow[int] { return indexedUnknown().Drop(cut) },
					lazy:  func() Flow[int] { return dropLazy(indexedUnknown(), cut) },
				},
				{
					name:  "Take",
					eager: func() Flow[int] { return indexedUnknown().Take(cut) },
					lazy:  func() Flow[int] { return takeLazy(indexedUnknown(), cut) },
				},
				{
					name:  "DropWhile",
					eager: func() Flow[int] { return indexedUnknown().DropWhile(LessThan(cut)) },
					lazy:  func() Flow[int] { return dropWhileLazy(indexedUnknown(), LessThan(cut)) },
				},
				{
					name:  "TakeWhile",
					eager: func() Flow[int] { return indexedUnknown().TakeWhile(LessThan(cut)) },
					lazy:  func() Flow[int] { return takeWhileLazy(indexedUnknown(), LessThan(cut)) },
				},
			}
			for _, tc := range cases {
				b.Run(tc.name, func(b *testing.B) {
					b.Run("eager_terminal1x", consume(1, tc.eager))
					b.Run("eager_terminal3x", consume(3, tc.eager))
					b.Run("lazy_terminal1x", consume(1, tc.lazy))
					b.Run("lazy_terminal3x", consume(3, tc.lazy))
				})
			}
		})
	}
}

// BenchmarkTakeShortCircuit measures Take over a selective Filter on a large
// lazy source — the README's headline lazy-evaluation shape, and the one
// BenchmarkTakeDropStrategies does not reach. Exactly `survivors` elements out
// of n pass the filter, and they sit at the front, so how far Take scans is
// what the benchmark exposes. Take(survivors) is the boundary case: an
// implementation needing one element past its count has to scan all n.
//
// Both strategies short-circuit here, so a single terminal call generates the
// same elements either way. What separates them is repeated traversal, since
// the lazy alternative re-runs the pipeline per terminal call while the eager
// one keeps its result.
//
// Result (Apple M2, five 100 ms samples, n=1,000,000, survivors=10):
// at take=10, one terminal costs ~161 ns eager vs ~249 ns lazy; three cost
// ~280 ns vs ~619 ns. Eager Take generates 10 elements once, lazy once per
// terminal. At take=20 neither can short-circuit: one terminal takes ~3.26 ms
// eager vs ~2.96 ms lazy; three take ~3.25 ms vs ~8.89 ms. take=n also measures
// a count far above the survivors, where the retained slice is sparse.
func BenchmarkTakeShortCircuit(b *testing.B) {
	const (
		n         = 1_000_000
		survivors = 10
	)
	// i*i < survivors*survivors holds for exactly the first `survivors` indices.
	src := func() Flow[int] {
		return FromFn(n, func(i int) int { return i * i }).Filter(LessThan(survivors * survivors))
	}
	consume := func(times int, mk func() Flow[int]) func(b *testing.B) {
		return func(b *testing.B) {
			for b.Loop() {
				out := mk()
				for range times {
					_ = out.Slice()
				}
			}
		}
	}
	for _, take := range []int{survivors / 2, survivors, survivors * 2, n} {
		eager := func() Flow[int] { return src().Take(take) }
		lazy := func() Flow[int] { return takeLazy(src(), take) }
		b.Run(fmt.Sprintf("take=%d", take), func(b *testing.B) {
			b.Run("eager_terminal1x", consume(1, eager))
			b.Run("eager_terminal3x", consume(3, eager))
			b.Run("lazy_terminal1x", consume(1, lazy))
			b.Run("lazy_terminal3x", consume(3, lazy))
		})
	}
}

var sliceBenchmarkResult []int

func BenchmarkSliceCompaction(b *testing.B) {
	const n = 65_536
	for _, tc := range []struct {
		name string
		keep func(int) bool
	}{
		{"sparse", func(v int) bool { return v < 1_024 }},
		{"half", func(v int) bool { return v%2 == 0 }},
		{"dense", func(int) bool { return true }},
	} {
		f := FromFn(n, Identity).Filter(tc.keep)
		b.Run(tc.name, func(b *testing.B) {
			b.Run("without_compaction", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					out := make([]int, 0, f.sizeHint())
					for v := range f.Seq() {
						out = append(out, v)
					}
					sliceBenchmarkResult = out
				}
			})
			b.Run("with_compaction", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					sliceBenchmarkResult = f.Slice()
				}
			})
		})
	}
}

func BenchmarkSliceCompactionCopy(b *testing.B) {
	src := make([]int, 1_024, 65_536)
	b.Run("without_compaction", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sliceBenchmarkResult = src
		}
	})
	b.Run("with_compaction", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			sliceBenchmarkResult = clipSparse(src)
		}
	})
}

// BenchmarkTakeScanAllocation compares the preallocation strategies of
// takeIndexedUnknown. B/op is cumulative allocation, so the benchmark says
// nothing about peak heap.
func BenchmarkTakeScanAllocation(b *testing.B) {
	const n = 65_536
	for _, tc := range []struct {
		name string
		keep func(int) bool
	}{
		{"sparse", func(v int) bool { return v < 1_024 }},
		{"half", func(v int) bool { return v%2 == 0 }},
		{"dense", func(int) bool { return true }},
	} {
		f := FromFn(n, Identity).Filter(tc.keep)
		b.Run(tc.name, func(b *testing.B) {
			b.Run("prealloc_clip", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					sliceBenchmarkResult = takeIndexedUnknown(f, n)
				}
			})
			b.Run("append_growth", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var out []int
					next := f.pull()
					for len(out) < n {
						_, v, ok := next()
						if !ok {
							break
						}
						out = append(out, v)
					}
					sliceBenchmarkResult = out
				}
			})
		})
	}
}

// BenchmarkScanWhileCost isolates the cost of the eager scan Flow.DropWhile
// and Flow.TakeWhile run on an indexed Flow: the predicate is one comparison
// and the result is only re-bounded (From's known count lets Count skip
// traversal), so each's physical-index walk is the whole signal.
// BenchmarkTakeDropStrategies holds its boundary to a short prefix, so a
// walk regression surfaces only here, at scan=n-1.
//
// Result (Apple M2, n=1,000,000): scan=20 is ~82 ns/op, too short to read
// the per-element term; scan=999,999 is ~2.55 ms/op for both methods.
func BenchmarkScanWhileCost(b *testing.B) {
	const n = 1_000_000
	ints := make([]int, n)
	for i := range ints {
		ints[i] = i
	}
	for _, scan := range []int{20, n - 1} {
		b.Run(fmt.Sprintf("scan=%d", scan), func(b *testing.B) {
			b.Run("DropWhile", func(b *testing.B) {
				for b.Loop() {
					_ = From(ints...).DropWhile(LessThan(scan)).Count()
				}
			})
			b.Run("TakeWhile", func(b *testing.B) {
				for b.Loop() {
					_ = From(ints...).TakeWhile(LessThan(scan)).Count()
				}
			})
		})
	}
}

// BenchmarkFilterRepresentation compares three ways to apply several
// predicates:
//
//   - chaining Filter calls while staying indexed
//   - chaining Filter calls after dropping to the sequential representation
//   - combining the predicates with [And] into a single indexed Filter call
//
// depth varies the predicate count, since "Filter-heavy" is a claim about
// chains, not a single call. Absolute times vary by run and machine, so only
// the relative ordering below should be relied on.
//
// Result: chained indexed wins only at depth=1, where it is the same single
// Filter call as filter_and plus one avoidable layer of And's loop, so
// filter_and trails there by construction, not because fusion is bad. From
// depth=2 the ordering these two exist to test is chained indexed losing to
// chained sequential, growing worse with depth (roughly on par at depth=2,
// chained sequential clearly ahead by depth=4). filter_and matches or beats
// chained sequential at depth=2 and depth=4 while keeping the indexed
// representation — see the note in Flow.Filter's implementation for why that
// representation matters beyond this benchmark's own throughput numbers.
func BenchmarkFilterRepresentation(b *testing.B) {
	const n = 1_000_000
	src := make([]int, n)
	for i := range src {
		src[i] = i
	}
	allPreds := []func(int) bool{
		func(v int) bool { return v%2 == 0 },
		func(v int) bool { return v%3 != 0 },
		func(v int) bool { return v%5 != 0 },
		func(v int) bool { return v%7 != 0 },
	}

	for _, depth := range []int{1, 2, 4} {
		preds := allPreds[:depth]
		b.Run(fmt.Sprintf("depth=%d", depth), func(b *testing.B) {
			b.Run("indexed", func(b *testing.B) {
				for b.Loop() {
					f := From(src...)
					for _, pred := range preds {
						f = f.Filter(pred)
					}
					_ = f.Count()
				}
			})
			b.Run("sequential", func(b *testing.B) {
				for b.Loop() {
					f := From(src...).asSeq()
					for _, pred := range preds {
						f = f.Filter(pred)
					}
					_ = f.Count()
				}
			})
			b.Run("filter_and", func(b *testing.B) {
				for b.Loop() {
					_ = From(src...).Filter(And(preds...)).Count()
				}
			})
		})
	}
}

// padded is a 64-byte element, large enough that moving it during a sort costs
// noticeably more than moving the index that stands in for it.
type padded struct {
	name string
	_    [48]byte
}

// BenchmarkSortByStrategies compares SortBy's permutation sort against the
// comparator formulation, over the two axes that separate them: how
// expensive key is, and how large an element is to move.
//
// Result: the permutation sort wins on both axes. With an expensive key
// (strings.ToLower) it wins by the widest margin, because key runs once per
// element instead of once per comparison. With a key as cheap as a field
// access it still wins, because the sort swaps indices rather than whole
// elements. Where neither axis applies — ints keyed by Identity — the two
// land close in time, and the permutation sort's cost shows up as memory
// instead: one extra slice of len n (the key/index pairs — SortFunc sorts the
// elements in place and needs none). flow.go therefore keeps it.
//
// docs/performance.md's SortBy tables hold the measured figures. Regenerate
// them with this benchmark rather than hand-updating numbers here.
func BenchmarkSortByStrategies(b *testing.B) {
	strategies := []struct {
		name string
		ints func(Flow[int], func(int) int) Flow[int]
		pads func(Flow[padded], func(padded) string) Flow[padded]
	}{
		// SortBy is wrapped rather than passed as a method expression:
		// Flow[int].SortBy leaves the method's own K uninstantiated, which the
		// gc compiler rejects with an internal error rather than a diagnostic.
		{
			"permutation",
			func(f Flow[int], key func(int) int) Flow[int] { return f.SortBy(key) },
			func(f Flow[padded], key func(padded) string) Flow[padded] { return f.SortBy(key) },
		},
		{"comparator", sortByComparator, sortByComparator},
	}
	for _, n := range []int{100, 10_000, 1_000_000} {
		ints := make([]int, n)
		pads := make([]padded, n)
		// Fixed seed so a run is comparable against the one before it.
		r := rand.New(rand.NewPCG(0x50B7, 0xBEEF))
		for i := range ints {
			ints[i] = r.Int()
			pads[i] = padded{name: fmt.Sprintf("User-%d", r.IntN(n))}
		}
		for _, st := range strategies {
			b.Run(fmt.Sprintf("n=%d/elem=int/key=identity/%s", n, st.name), func(b *testing.B) {
				for b.Loop() {
					st.ints(From(ints...), Identity).Slice()
				}
			})
			b.Run(fmt.Sprintf("n=%d/elem=64B/key=field/%s", n, st.name), func(b *testing.B) {
				for b.Loop() {
					st.pads(From(pads...), func(v padded) string { return v.name }).Slice()
				}
			})
			b.Run(fmt.Sprintf("n=%d/elem=64B/key=tolower/%s", n, st.name), func(b *testing.B) {
				for b.Loop() {
					st.pads(From(pads...), func(v padded) string { return strings.ToLower(v.name) }).Slice()
				}
			})
		}
	}
}

// BenchmarkTrackPlay measures a three-stage Track carrying no [Track.OnBreak]
// — the shape most tracks have — and is the guard on Play's fast path: a
// track that registered no compensation must pay nothing for the rollback
// machinery. Its figures are therefore expected to stay put as that machinery
// changes; BenchmarkTrackPlayOnBreak below is where the cost of rollback
// bookkeeping is allowed to show. Routing this track through the rollback
// path instead costs a closure allocation per registered [Track.OnBreak]
// plus the slice that accumulates them.
//
// docs/performance.md holds the measured figures. Regenerate them with these
// benchmarks rather than hand-updating numbers here.
func BenchmarkTrackPlay(b *testing.B) {
	track := Groove(NilError(mul2)).Jam(NilError(add1)).Jam(NilError(mul3))
	for b.Loop() {
		_, _ = track.Play(42)
	}
}

func BenchmarkTrackPlayOnBreak(b *testing.B) {
	noop := func(int) error { return nil }
	track := Groove(NilError(mul2)).
		OnBreak(noop).
		Jam(NilError(add1)).
		OnBreak(noop).
		Jam(NilError(mul3)).
		OnBreak(noop)
	for b.Loop() {
		_, _ = track.Play(42)
	}
}

func runAllBenchmarks(n int, b *testing.B) {
	b.Helper()
	flatMap := func(f Flow[int]) Flow[int] {
		return f.FlatMap(func(v int) Flow[int] { return From(v) })
	}
	concatOther := FromFn(n, Identity)
	runIter := func(seq iter.Seq[int]) {
		for range seq {
		}
	}

	cases := []struct {
		name string
		api  func(int, Flow[int])
	}{
		{
			name: "Filter",
			api:  func(_ int, f Flow[int]) { _ = f.Filter(even).All(True) },
		},
		{
			name: "Map",
			api:  func(_ int, f Flow[int]) { _ = f.Map(mul2).All(True) },
		},
		{
			name: "Reduce",
			api:  func(_ int, f Flow[int]) { _ = f.Reduce(add) },
		},
		{
			name: "Slice",
			api:  func(_ int, f Flow[int]) { _ = f.Slice() },
		},
		{
			name: "Seq",
			api:  func(_ int, f Flow[int]) { runIter(f.Seq()) },
		},
		{
			name: "Count",
			api:  func(_ int, f Flow[int]) { _ = f.Count() },
		},
		{
			name: "Any",
			api:  func(n int, f Flow[int]) { _ = f.Any(Equal(n - 1)) },
		},
		{
			name: "All",
			api:  func(_ int, f Flow[int]) { _ = f.All(True) },
		},
		{
			name: "Find",
			api:  func(_ int, f Flow[int]) { _ = f.Find(False) },
		},
		{
			name: "First",
			api:  func(_ int, f Flow[int]) { _ = f.First() },
		},
		{
			name: "Last",
			api:  func(_ int, f Flow[int]) { _ = f.Last() },
		},
		{
			name: "Drop",
			api:  func(n int, f Flow[int]) { _ = f.Drop(n / 2).All(True) },
		},
		{
			name: "Take",
			api:  func(n int, f Flow[int]) { _ = f.Take(n / 2).All(True) },
		},
		{
			name: "DropWhile",
			api:  func(n int, f Flow[int]) { _ = f.DropWhile(LessThan(n / 2)).All(True) },
		},
		{
			name: "TakeWhile",
			api:  func(n int, f Flow[int]) { _ = f.TakeWhile(LessThan(n / 2)).All(True) },
		},
		{
			name: "Reverse",
			api:  func(n int, f Flow[int]) { _ = f.Reverse().All(True) },
		},
		{
			name: "IsEmpty",
			api:  func(n int, f Flow[int]) { _ = f.IsEmpty() },
		},
		{
			name: "FlatMap",
			api:  func(i int, f Flow[int]) { _ = flatMap(f).All(True) },
		},
		{
			name: "MapIndexed",
			api:  func(_ int, f Flow[int]) { _ = f.MapIndexed(func(i, v int) int { return i + v }).All(True) },
		},
		{
			name: "Distinct",
			api:  func(_ int, f Flow[int]) { _ = Distinct(f).All(True) },
		},
		{
			name: "DistinctBy",
			api:  func(_ int, f Flow[int]) { _ = f.DistinctBy(Identity).All(True) },
		},
		{
			name: "Partition",
			api: func(_ int, f Flow[int]) {
				matched, rest := f.Partition(even)
				_, _ = matched, rest
			},
		},
		{
			name: "GroupBy",
			api:  func(_ int, f Flow[int]) { _ = f.To(GroupBy(func(v int) int { return v & 15 })) },
		},
		{
			name: "ToMap",
			api:  func(_ int, f Flow[int]) { _ = f.ToMap(Identity) },
		},
		{
			name: "Chunk",
			api:  func(_ int, f Flow[int]) { _ = f.To(Chunk[int](16)).Count() },
		},
		{
			name: "Concat",
			api:  func(_ int, f Flow[int]) { _ = f.Concat(concatOther).All(True) },
		},
		{
			name: "Accumulate",
			api:  func(_ int, f Flow[int]) { _ = f.Accumulate(0, add).All(True) },
		},
	}

	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			flow := FromFn(n, Identity)
			b.Run("lazy", func(b *testing.B) {
				for b.Loop() {
					bc.api(n, flow)
				}
			})
			flow = FromFn(n, Identity).Cache()
			b.Run("slice", func(b *testing.B) {
				for b.Loop() {
					bc.api(n, flow)
				}
			})
		})
	}
}
