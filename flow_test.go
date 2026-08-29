package funq

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"
)

// eq asserts that a Flow yields the expected slice, both directly and after
// Cache (which exercises the eager, materialized representation).
func eq[T any](t *testing.T, want []T, got Flow[T]) {
	t.Helper()
	assertEqual(t, want, got.Slice())
	assertEqual(t, want, got.Cache().Slice())
}

func TestFrom(t *testing.T) {
	t.Parallel()
	eq(t, []int{}, From[int]())
	eq(t, []int{}, From([]int{}...))
	eq(t, []int{1}, From(1))
	eq(t, []int{1, 2, 3}, From(1, 2, 3))
}

func TestFromAliasesSpreadSlice(t *testing.T) {
	t.Parallel()
	xs := []int{1, 2, 3}
	f := From(xs...)
	xs[0] = 99
	assertEqual(t, []int{99, 2, 3}, f.Slice(), "From(xs...) must alias xs, not copy it")

	out := f.Slice()
	out[0] = -1
	assertEqual(t, []int{99, 2, 3}, f.Slice(), "Slice must return a fresh copy")
}

func TestFromFn(t *testing.T) {
	t.Parallel()
	eq(t, []int{}, FromFn(0, Identity))
	eq(t, []int{}, FromFn(-1, Identity))
	eq(t, []int{0, 1, 2, 3, 4}, FromFn(5, Identity))
	eq(t, []int{0, 2, 4, 6, 8}, FromFn(5, mul2))
}

func TestFromSeq(t *testing.T) {
	t.Parallel()
	eq(t, []int{1, 2, 3}, FromSeq(slices.Values([]int{1, 2, 3})))
	eq(t, []int{}, FromSeq(slices.Values([]int{})))

	// FromSeq(f.Seq()) round-trips: Seq is the outbound direction FromSeq
	// mirrors.
	f := FromFn(5, Identity).Filter(even)
	assertEqual(t, f.Slice(), FromSeq(f.Seq()).Slice())

	// A nil seq is an empty Flow with a statically known (not just
	// eventually-zero) count, unlike the internal fromSeq(nil).
	n := FromSeq[int](nil)
	assertEqual(t, 0, n.Count())
	isTrue(t, n.IsEmpty())
	eq(t, []int{}, n)

	calls := 0
	src := func(yield func(int) bool) {
		for i := 0; ; i++ {
			calls++
			if !yield(i) {
				return
			}
		}
	}
	assertEqual(t, []int{0, 1, 2}, FromSeq(src).Take(3).Slice())
	assertEqual(t, 3, calls, "Take(3) must stop ranging over seq after the 3rd element")
	assertEqual(t, Some(0), FromSeq(src).First())

	strs := FromSeq(slices.Values([]int{1, 2, 3})).Map(strconv.Itoa).Slice()
	assertEqual(t, []string{"1", "2", "3"}, strs)
}

func TestZeroValueFlowIsEmpty(t *testing.T) {
	t.Parallel()
	var f Flow[int]
	isTrue(t, f.IsEmpty())
	assertEqual(t, 0, f.Count())
	assertEqual(t, []int{}, f.Slice())
}

// flowProps are the two properties the [Flow] doc comment tells callers they
// can read off the way a Flow was built.
type flowProps struct {
	indexed    bool
	knownCount bool
}

func propsOf[T any](f Flow[T]) flowProps {
	return flowProps{indexed: f.at != nil, knownCount: f.size != sizeUnknown}
}

// TestFlowDocProperties pins the [Flow] doc comment's per-operation lists of
// which constructions carry a statically known count and which leave the Flow
// forward-only. Hand-maintained and caller-facing, they are not reducible to
// a shorter rule and would otherwise drift from the code with nothing failing.
func TestFlowDocProperties(t *testing.T) {
	t.Parallel()
	base := FromFn(5, Identity)
	indexedKnown := flowProps{indexed: true, knownCount: true}
	forwardOnly := flowProps{indexed: false, knownCount: false}

	cases := []struct {
		name string
		got  flowProps
		want flowProps
	}{
		// establish / re-establish a statically known count
		{"From", propsOf(From(1, 2, 3)), indexedKnown},
		{"FromFn", propsOf(base), indexedKnown},
		{"OptionalAsFlow", propsOf(Some(1).AsFlow()), indexedKnown},
		{"CacheReestablishes", propsOf(base.Filter(even).Cache()), indexedKnown},

		// carry the count through
		{"Map", propsOf(base.Map(mul2)), indexedKnown},
		{"Take", propsOf(base.Take(3)), indexedKnown},
		{"Drop", propsOf(base.Drop(2)), indexedKnown},
		{"TakeWhile", propsOf(base.TakeWhile(LessThan(3))), indexedKnown},
		{"DropWhile", propsOf(base.DropWhile(LessThan(3))), indexedKnown},
		{"Reverse", propsOf(base.Reverse()), indexedKnown},
		{"ConcatOfKnown", propsOf(From(1, 2).Concat(From(3, 4))), indexedKnown},
		{"ReverseMaterializesSequential", propsOf(base.asSeq().Reverse()), indexedKnown},

		// give it up
		{"FilterGivesUpCount", propsOf(base.Filter(even)), flowProps{indexed: true}},

		// leave the Flow forward-only (property 2)
		{"FromSeq", propsOf(FromSeq(slices.Values([]int{1, 2, 3}))), forwardOnly},
		{"FlatMap", propsOf(base.FlatMap(func(v int) Flow[int] { return From(v) })), forwardOnly},
		{"MapIndexed", propsOf(base.MapIndexed(func(i, v int) int { return i + v })), forwardOnly},
		{"DistinctBy", propsOf(base.DistinctBy(Identity)), forwardOnly},
		{"Distinct", propsOf(Distinct(base)), forwardOnly},
		{"Zip", propsOf(Zip(base, base)), forwardOnly},
		{"Chunk", propsOf(Chunk[int](2)(base)), forwardOnly},
		{"ConcatOfUnknown", propsOf(From(1, 2).Concat(From(3, 4).Filter(even))), forwardOnly},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertEqual(t, tc.want, tc.got)
		})
	}
}

func TestFilter(t *testing.T) {
	t.Parallel()
	eq(t, []int{}, From[int]().Filter(True))
	eq(t, []int{0, 2, 4, 6, 8}, FromFn(10, Identity).Filter(even))
	eq(t, []int{}, FromFn(10, Identity).Filter(False))
	eq(t, []int{0, 1, 4}, FromFn(5, Identity).Filter(Not(OneOf(2, 3))))
	eq(t, []int{2, 4}, From(1, 2, 3, 4).asSeq().Filter(even))
}

func TestMapSameType(t *testing.T) {
	t.Parallel()
	eq(t, []int{0, 2, 4, 6, 8}, FromFn(5, Identity).Map(mul2))
	eq(t, []int{0, 1, 2, 3, 4}, FromFn(10, Identity).Filter(even).Map(div2))
	eq(t, []int{4, 3, 2, 1, 0}, FromFn(10, Identity).Filter(even).Reverse().Map(div2))
}

func TestMapTypeChanging(t *testing.T) {
	t.Parallel()
	got := From(1, 2, 3).
		Map(func(n int) int { return n * 10 }).
		Map(strconv.Itoa).
		Slice()
	assertEqual(t, []string{"10", "20", "30"}, got)

	got2 := From(1, 2).asSeq().Map(strconv.Itoa).Slice()
	assertEqual(t, []string{"1", "2"}, got2)
}

func TestMapIndexed(t *testing.T) {
	t.Parallel()
	type iv struct {
		i int
		v int
	}
	pair := func(i, v int) iv { return iv{i, v} }

	eq(t, []iv{}, From[int]().MapIndexed(pair))
	eq(t, []iv{{0, 10}, {1, 20}, {2, 30}}, From(10, 20, 30).MapIndexed(pair))
	eq(t, []iv{{0, 10}, {1, 20}}, From(10, 20).asSeq().MapIndexed(pair))
	// logical position, not physical: holes from Filter are not counted
	eq(t, []iv{{0, 0}, {1, 4}, {2, 8}}, FromFn(10, Identity).Filter(func(n int) bool { return n%4 == 0 }).MapIndexed(pair))
	// logical position after Reverse restarts from 0 in the new order
	eq(t, []iv{{0, 4}, {1, 3}, {2, 2}, {3, 1}, {4, 0}}, FromFn(5, Identity).Reverse().MapIndexed(pair))
	// First stops after the first element, exercising the early-termination
	// (!yield) branch of MapIndexed's generator.
	assertEqual(t, Some(iv{0, 10}), From(10, 20, 30).MapIndexed(pair).First())
}

func TestForEachIndexed(t *testing.T) {
	t.Parallel()
	type iv struct {
		i int
		v int
	}
	var got []iv
	FromFn(3, Identity).ForEachIndexed(func(i, v int) { got = append(got, iv{i, v}) })
	assertEqual(t, []iv{{0, 0}, {1, 1}, {2, 2}}, got)

	got = nil
	From(10, 20, 30).asSeq().ForEachIndexed(func(i, v int) { got = append(got, iv{i, v}) })
	assertEqual(t, []iv{{0, 10}, {1, 20}, {2, 30}}, got)

	got = nil
	FromFn(9, Identity).Filter(func(n int) bool { return n%4 == 0 }).ForEachIndexed(func(i, v int) { got = append(got, iv{i, v}) })
	assertEqual(t, []iv{{0, 0}, {1, 4}, {2, 8}}, got)
}

func TestFlatMap(t *testing.T) {
	t.Parallel()
	eq(t, []int{0, 1, 2, 3, 4}, From(5).FlatMap(func(i int) Flow[int] { return FromFn(i, Identity) }))
	eq(t, []int{}, From[int]().FlatMap(func(i int) Flow[int] { return From(i) }))
	got := FromFn(6, Identity).FlatMap(func(i int) Flow[int] {
		if i%2 == 0 {
			return empty[int]()
		}
		return FromFn(i, Identity)
	})
	eq(t, []int{0, 0, 1, 2, 0, 1, 2, 3, 4}, got)
	got2 := From(1, 2).FlatMap(func(n int) Flow[string] { return From(strconv.Itoa(n)) }).Slice()
	assertEqual(t, []string{"1", "2"}, got2)

	// First stops after the first flattened element, exercising the
	// early-termination (!yield) branch of FlatMap's generator; fn must not
	// run for outer elements past the one that produced it.
	calls := 0
	got3 := FromFn(3, Identity).FlatMap(func(i int) Flow[int] {
		calls++
		return FromFn(2, func(j int) int { return i*10 + j })
	}).First()
	assertEqual(t, Some(0), got3)
	assertEqual(t, 1, calls, "FlatMap must not evaluate fn past the first surviving element")
}

func TestFold(t *testing.T) {
	t.Parallel()
	got := From(1, 2, 3).Fold("", func(acc string, n int) string { return acc + strconv.Itoa(n) })
	assertEqual(t, "123", got)
	assertEqual(t, 0, From[int]().Fold(0, add))
}

func TestTo(t *testing.T) {
	t.Parallel()
	// A free function that could not be a method stays in the chain, and the
	// chain continues on the resulting Flow.
	eq(t, []int{2, 2, 1},
		From(1, 2, 3, 4, 5).To(Chunk[int](2)).Map(func(c []int) int { return len(c) }))
	// Generic free functions infer their type argument from the receiver.
	eq(t, []int{1, 2, 3}, From(1, 2, 2, 3).To(Distinct))
	// The result type is unconstrained, not just another Flow.
	assertEqual(t, 6, From(1, 2, 3).To(sum))
	assertEqual(t, 0, From[int]().To(sum))
}

func TestReduce(t *testing.T) {
	t.Parallel()
	assertEqual(t, None[int](), From[int]().Reduce(add))
	assertEqual(t, None[int](), FromFn(10, Identity).Filter(False).Reduce(add))
	assertEqual(t, Some(45), FromFn(10, Identity).Reduce(add))
	assertEqual(t, Some(45), FromFn(10, Identity).Reverse().Reduce(add))
}

func TestSlice(t *testing.T) {
	t.Parallel()
	eq(t, []int{}, From[int]())
	eq(t, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, FromFn(10, Identity))
	eq(t, []int{9, 8, 7, 6, 5, 4, 3, 2, 1, 0}, FromFn(10, Identity).Reverse())
}

func TestCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		flow Flow[int]
		want int
	}{
		{"Empty", From[int](), 0},
		{"FilteredEmpty", FromFn(10, Identity).Filter(False), 0},
		{"Single", From(42), 1},
		{"Filtered", FromFn(10, Identity).Filter(even), 5},
		{"Drop5", FromFn(10, Identity).Drop(5), 5},
		{"Drop10", FromFn(10, Identity).Drop(10), 0},
		{"Take5", FromFn(10, Identity).Take(5), 5},
		{"Take10", FromFn(10, Identity).Take(10), 10},
		{"DropWhile5", FromFn(10, Identity).DropWhile(LessThan(5)), 5},
		{"DropWhile10", FromFn(10, Identity).DropWhile(LessThan(10)), 0},
		{"TakeWhile5", FromFn(10, Identity).TakeWhile(LessThan(5)), 5},
		{"TakeWhile10", FromFn(10, Identity).TakeWhile(LessThan(10)), 10},
		{"TakeWhile0", FromFn(10, Identity).TakeWhile(False), 0},
		{"DropWhile0", FromFn(10, Identity).DropWhile(False), 10},
		// The backward phys+1 boundary translation in each is only exercised by
		// walking a reversed indexed Flow.
		{"ReverseTakeWhile", FromFn(10, Identity).Reverse().TakeWhile(GreaterThan(5)), 4},
		{"ReverseDropWhile", FromFn(10, Identity).Reverse().DropWhile(GreaterThan(5)), 6},
		// A Filter upstream leaves holes and sizeUnknown; the guard in
		// TakeWhile must not mistake the physical boundary width (6, counting
		// the holes) for the live count (3).
		{"FilterTakeWhile", FromFn(10, Identity).Filter(even).TakeWhile(LessThan(6)), 3},
		{"Reverse", FromFn(10, Identity).Reverse(), 10},
		{"Complex", FromFn(100, Identity).Filter(even).Map(mul2).Drop(10).Take(20), 20},
		{"Seq", From(1, 2, 3).asSeq(), 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertEqual(t, tc.want, tc.flow.Count())
			assertEqual(t, tc.want, tc.flow.Count(), "Count must be repeatable")
		})
	}
}

func TestSeq(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		flow Flow[int]
		want []int
	}{
		{"Empty", From[int](), []int{}},
		{"Multiple", FromFn(3, Identity), []int{0, 1, 2}},
		{"Filtered", FromFn(3, Identity).Filter(even), []int{0, 2}},
		{"Reversed", FromFn(3, Identity).Reverse(), []int{2, 1, 0}},
		{"ReverseWithFilter", FromFn(5, Identity).Filter(even).Reverse(), []int{4, 2, 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := []int{}
			for v := range tc.flow.Seq() {
				got = append(got, v)
			}
			assertEqual(t, tc.want, got)

			rev := []int{}
			for v := range tc.flow.Reverse().Seq() {
				rev = append(rev, v)
			}
			want := slices.Clone(tc.want)
			slices.Reverse(want)
			assertEqual(t, want, rev)
		})
	}
}

func TestAny(t *testing.T) {
	t.Parallel()
	isFalse(t, From[int]().Any(True))
	isFalse(t, FromFn(10, Identity).Any(GreaterThan(9)))
	isTrue(t, FromFn(10, Identity).Any(GreaterThan(8)))
}

func TestAll(t *testing.T) {
	t.Parallel()
	isTrue(t, From[int]().All(False))
	isTrue(t, FromFn(10, Identity).Filter(False).All(False))
	isTrue(t, FromFn(10, Identity).All(LessThan(10)))
	isFalse(t, FromFn(10, Identity).All(LessThan(9)))
}

func TestFind(t *testing.T) {
	t.Parallel()
	assertEqual(t, None[int](), From[int]().Find(True))
	assertEqual(t, Some(9), FromFn(10, Identity).Find(Equal(9)))
	assertEqual(t, None[int](), FromFn(10, Identity).Find(Equal(10)))
}

func TestFirstLast(t *testing.T) {
	t.Parallel()
	assertEqual(t, None[int](), From[int]().First())
	assertEqual(t, None[int](), From[int]().Last())
	assertEqual(t, Some(0), FromFn(10, Identity).First())
	assertEqual(t, Some(9), FromFn(10, Identity).Last())
	assertEqual(t, Some(9), FromFn(10, Identity).Reverse().First())
	assertEqual(t, Some(0), FromFn(10, Identity).Reverse().Last())
	assertEqual(t, Some(0), FromFn(10, Identity).Filter(even).First())
	assertEqual(t, Some(8), FromFn(10, Identity).Filter(even).Last())

	// Sequential representation, where Last walks the whole Flow instead of
	// taking the indexed O(1) Reverse().First() shortcut.
	assertEqual(t, None[int](), From[int]().asSeq().First())
	assertEqual(t, None[int](), From[int]().asSeq().Last())
	assertEqual(t, Some(0), FromFn(10, Identity).asSeq().First())
	assertEqual(t, Some(9), FromFn(10, Identity).asSeq().Last())
	assertEqual(t, Some(0), FromFn(10, Identity).asSeq().Filter(even).First())
	assertEqual(t, Some(8), FromFn(10, Identity).asSeq().Filter(even).Last())
	assertEqual(t, None[int](), FromFn(10, Identity).asSeq().Filter(False).Last())
	assertEqual(t, Some(42), From(42).asSeq().Last())
	// Reverse materializes a sequential Flow back into an indexed one, so
	// asSeq has to come after it to keep Last on the sequential path.
	assertEqual(t, Some(0), FromFn(10, Identity).Reverse().asSeq().Last())
	assertEqual(t, Some(0), FromFn(10, Identity).asSeq().Reverse().Last())
}

func TestDrop(t *testing.T) {
	t.Parallel()
	eq(t, []int{}, From[int]().Drop(1))
	eq(t, []int{0, 1, 2}, FromFn(3, Identity).Drop(0))
	eq(t, []int{2}, FromFn(3, Identity).Drop(2))
	eq(t, []int{}, FromFn(3, Identity).Drop(5))
	eq(t, []int{7, 6, 5, 4, 3, 2, 1, 0}, FromFn(10, Identity).Reverse().Drop(2))
	eq(t, []int{6, 8}, FromFn(10, Identity).Filter(even).Drop(3))
	eq(t, []int{2, 3}, From(0, 1, 2, 3).asSeq().Drop(2))
}

func TestTake(t *testing.T) {
	t.Parallel()
	eq(t, []int{}, FromFn(3, Identity).Take(0))
	eq(t, []int{0}, FromFn(3, Identity).Take(1))
	eq(t, []int{0, 1, 2}, FromFn(3, Identity).Take(5))
	eq(t, []int{9, 8}, FromFn(10, Identity).Reverse().Take(2))
	eq(t, []int{0, 2, 4}, FromFn(10, Identity).Filter(even).Take(3))
	eq(t, []int{0, 1}, From(0, 1, 2, 3).asSeq().Take(2))
	// n far beyond the Flow must not preallocate for n
	eq(t, []int{0, 2, 4}, FromFn(6, Identity).Filter(even).Take(math.MaxInt))
}

// TestTakeShortCircuitsLazySource guards the README's headline lazy-evaluation
// claim, which a boundary-scanning Take broke. Locating the boundary needed
// the (k+1)-th surviving element, so a Filter matching exactly k elements
// scanned the whole source, and the returned Flow then scanned it again on
// every terminal call — 2,000,000 generator calls to produce 10 elements.
func TestTakeShortCircuitsLazySource(t *testing.T) {
	t.Parallel()
	const n = 1_000_000
	want := []int{0, 1, 4, 9, 16, 25, 36, 49, 64, 81}

	count := func(fn func(Flow[int]) []int) (int, []int) {
		calls := 0
		got := fn(FromFn(n, func(i int) int { calls++; return i * i }).Filter(LessThan(100)))
		return calls, got
	}

	// Exactly as many survivors as requested: the scan must stop at the 10th,
	// not run on looking for an 11th.
	calls, got := count(func(f Flow[int]) []int { return f.Take(10).Slice() })
	assertEqual(t, want, got)
	assertEqual(t, 10, calls, "Take(10) must generate only the elements it keeps")

	// Fewer survivors than requested: reaching the end is the only way to
	// establish that, but it must happen once, not once per terminal call.
	calls, got = count(func(f Flow[int]) []int {
		out := f.Take(20)
		out.Slice()
		return out.Slice()
	})
	assertEqual(t, want, got)
	assertEqual(t, n, calls, "Take must not re-run the pipeline per terminal call")

	// Repeated terminals over a satisfied Take stay free.
	calls, got = count(func(f Flow[int]) []int {
		out := f.Take(10)
		out.Slice()
		out.Count()
		return out.Slice()
	})
	assertEqual(t, want, got)
	assertEqual(t, 10, calls)
}

func TestDropWhile(t *testing.T) {
	t.Parallel()
	eq(t, []int{5, 6, 7, 8, 9}, FromFn(10, Identity).DropWhile(LessThan(5)))
	eq(t, []int{}, FromFn(10, Identity).DropWhile(LessThan(10)))
	eq(t, []int{2, 3}, From(0, 1, 2, 3).asSeq().DropWhile(LessThan(2)))
}

func TestTakeWhile(t *testing.T) {
	t.Parallel()
	eq(t, []int{0, 1, 2, 3, 4}, FromFn(10, Identity).TakeWhile(LessThan(5)))
	eq(t, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, FromFn(10, Identity).TakeWhile(LessThan(10)))
	eq(t, []int{0, 1}, From(0, 1, 2, 3).asSeq().TakeWhile(LessThan(2)))
}

func TestReverse(t *testing.T) {
	t.Parallel()
	eq(t, []int{}, From[int]().Reverse())
	eq(t, []int{9, 8, 7, 6, 5, 4, 3, 2, 1, 0}, FromFn(10, Identity).Reverse())
	eq(t, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, FromFn(10, Identity).Reverse().Reverse())
	eq(t, []int{3, 2, 1}, From(1, 2, 3).asSeq().Reverse())
}

func TestIsEmpty(t *testing.T) {
	t.Parallel()
	isTrue(t, From[int]().IsEmpty())
	isTrue(t, FromFn(10, Identity).Filter(False).IsEmpty())
	isFalse(t, From(1).IsEmpty())
	isFalse(t, From(1).Reverse().IsEmpty())
}

func TestForEach(t *testing.T) {
	t.Parallel()
	sum := 0
	FromFn(5, Identity).ForEach(func(n int) { sum += n })
	assertEqual(t, 10, sum)
}

func TestComplexChain(t *testing.T) {
	t.Parallel()
	result := FromFn(20, Identity).
		Filter(even).
		Map(add1).
		Filter(GreaterThan(5)).
		Take(5).
		Reverse().
		Slice()
	assertEqual(t, []int{15, 13, 11, 9, 7}, result)
}

func TestSortFunc(t *testing.T) {
	t.Parallel()
	desc := func(a, b int) int { return b - a }
	eq(t, []int{}, From[int]().SortFunc(desc))
	eq(t, []int{1}, From(1).SortFunc(desc))
	eq(t, []int{5, 3, 2, 1}, From(3, 1, 5, 2).SortFunc(desc))
	eq(t, []int{4, 3, 2, 1}, From(2, 4, 1, 3).asSeq().SortFunc(desc))
	// stable: equal keys keep relative order
	got := From(kv{1, "a"}, kv{0, "b"}, kv{1, "c"}, kv{0, "d"}).
		SortFunc(func(a, b kv) int { return a.k - b.k }).
		Slice()
	assertEqual(t, []kv{{0, "b"}, {0, "d"}, {1, "a"}, {1, "c"}}, got)
}

func TestSortBy(t *testing.T) {
	t.Parallel()
	eq(t, []int{}, From[int]().SortBy(Identity))
	eq(t, []int{1, 2, 3, 5}, From(3, 1, 5, 2).SortBy(Identity))
	eq(t, []int{-1, 2, -3}, From(2, -3, -1).SortBy(abs))
	eq(t, []int{1, 2, 3, 5}, From(3, 1, 5, 2).asSeq().SortBy(Identity))
	// stable: equal keys keep relative order
	got := From(kv{1, "a"}, kv{0, "b"}, kv{1, "c"}, kv{0, "d"}).
		SortBy(func(x kv) int { return x.k }).
		Slice()
	assertEqual(t, []kv{{0, "b"}, {0, "d"}, {1, "a"}, {1, "c"}}, got)
}

// TestSortByCallsKeyOncePerElement pins the guarantee in SortBy's doc comment.
// Passing key to a comparator instead would call it O(n log n) times.
func TestSortByCallsKeyOncePerElement(t *testing.T) {
	t.Parallel()
	for _, n := range []int{0, 1, 2, 100, 1000} {
		src := make([]int, n)
		for i := range src {
			src[i] = n - i
		}
		calls := 0
		From(src...).SortBy(func(v int) int { calls++; return v }).Slice()
		assertEqual(t, n, calls, fmt.Sprintf("n=%d", n))
	}
}

// TestSortByMatchesComparatorSort cross-checks SortBy against the comparator
// formulation it replaced, over randomized inputs with many duplicate keys so
// that any loss of stability shows up.
func TestSortByMatchesComparatorSort(t *testing.T) {
	t.Parallel()
	// Fixed seed so a failure reproduces, as in TestDifferentialIndexedVsSequential.
	r := rand.New(rand.NewPCG(0x5024, 0xB1AD))
	key := func(x kv) int { return x.k }
	for trial := range 500 {
		xs := make([]kv, r.IntN(64))
		for i := range xs {
			xs[i] = kv{r.IntN(5), strconv.Itoa(i)}
		}
		want := sortByComparator(From(xs...), key).Slice()
		assertEqual(t, want, From(xs...).SortBy(key).Slice(), fmt.Sprintf("trial %d: %v", trial, xs))
	}
}

func TestDistinct(t *testing.T) {
	t.Parallel()
	eq(t, []int{}, Distinct(From[int]()))
	eq(t, []int{1, 2, 3}, Distinct(From(1, 2, 3)))
	eq(t, []int{1, 2, 3}, Distinct(From(1, 2, 2, 1, 3, 3, 3)))
	eq(t, []int{1, 2}, Distinct(From(1, 1, 2).asSeq()))
	// composes with Take after the fact
	eq(t, []int{1, 2}, Distinct(From(1, 1, 2, 2, 3, 3)).Take(2))
}

func TestDistinctBy(t *testing.T) {
	t.Parallel()
	eq(t, []int{}, From[int]().DistinctBy(Identity))
	eq(t, []int{-3, 1, 2}, From(-3, 1, 2, 3, -1, -2).DistinctBy(abs))
	// composes with Take after the fact
	eq(t, []int{-3, 1}, From(-3, 1, 2, 3, -1, -2).DistinctBy(abs).Take(2))
}

func TestGroupBy(t *testing.T) {
	t.Parallel()
	assertEqual(t, map[int][]int{}, From[int]().GroupBy(Identity))
	got := From(1, 2, 3, 4, 5, 6).GroupBy(func(n int) int { return n % 2 })
	assertEqual(t, map[int][]int{0: {2, 4, 6}, 1: {1, 3, 5}}, got)
}

func TestConcat(t *testing.T) {
	t.Parallel()
	eq(t, []int{1, 2, 3}, From(1, 2, 3).Concat())
	eq(t, []int{1, 2, 3, 4}, From(1, 2, 3).Concat(From(4)))
	eq(t, []int{1, 2, 3, 4, 5, 6}, From(1, 2, 3).Concat(From(4, 5), From(6)))
	eq(t, []int{1, 2}, From[int]().Concat(From(1, 2)))
	eq(t, []int{1, 2}, From(1, 2).Concat(From[int]()))
}

func TestConcatStaysIndexed(t *testing.T) {
	t.Parallel()
	// All inputs indexed with known size: the result must stay indexed with
	// the combined size, and indexed-only behavior (O(1) Reverse, exact
	// Count without traversal, reversed inputs) must hold.
	f := From(1, 2).Concat(From(3, 4).Reverse(), FromFn(2, func(y int) int { return y + 5 }))
	if f.at == nil {
		t.Errorf("want non-nil, got nil: Concat of known-size indexed Flows must stay indexed")
	}
	assertEqual(t, 6, f.size)
	eq(t, []int{1, 2, 4, 3, 5, 6}, f)
	eq(t, []int{6, 5, 3, 4, 2, 1}, f.Reverse())
	eq(t, []int{4, 3}, f.Drop(2).Take(2))

	// Any sequential or unknown-size input forces the sequential fallback.
	if at := From(1).Concat(From(2).asSeq()).at; at != nil {
		t.Errorf("want nil, at is non-nil")
	}
	if at := From(1).Concat(From(2, 3).Filter(True)).at; at != nil {
		t.Errorf("want nil, at is non-nil")
	}
	eq(t, []int{1, 2}, From(1).Concat(From(2).asSeq()))
}

func TestToMap(t *testing.T) {
	t.Parallel()
	assertEqual(t, map[int]int{}, From[int]().ToMap(Identity))
	assertEqual(t, map[int]int{0: 0, 1: 1, 2: 2}, FromFn(3, Identity).ToMap(Identity))
	// duplicate keys: first occurrence wins
	assertEqual(t, map[int]string{1: "a", 2: "bb"}, From("a", "bb", "c").ToMap(func(s string) int { return len(s) }))
	assertEqual(t, map[int]int{1: 1, 2: 2}, From(1, 2).asSeq().ToMap(Identity))
}

func TestPartition(t *testing.T) {
	t.Parallel()
	matched, rest := From[int]().Partition(True)
	eq(t, []int{}, matched)
	eq(t, []int{}, rest)

	matched, rest = FromFn(6, Identity).Partition(even)
	eq(t, []int{0, 2, 4}, matched)
	eq(t, []int{1, 3, 5}, rest)

	// order preserved on both sides
	matched, rest = From(5, 1, 4, 2, 3).asSeq().Partition(GreaterThan(2))
	eq(t, []int{5, 4, 3}, matched)
	eq(t, []int{1, 2}, rest)

	// upstream pipeline runs exactly once
	calls := 0
	src := FromFn(4, func(i int) int { calls++; return i })
	matched, rest = src.Partition(even)
	eq(t, []int{0, 2}, matched)
	eq(t, []int{1, 3}, rest)
	assertEqual(t, 4, calls)
}

func TestMinBy(t *testing.T) {
	t.Parallel()
	assertEqual(t, None[int](), From[int]().MinBy(Identity))
	assertEqual(t, Some(1), From(3, 1, 2).MinBy(Identity))
	// tie: first one wins
	assertEqual(t, Some(-1), From(-1, 1, 5).MinBy(abs))
	assertEqual(t, Some(1), From(3, 1, 2).asSeq().MinBy(Identity))
}

func TestMaxBy(t *testing.T) {
	t.Parallel()
	assertEqual(t, None[int](), From[int]().MaxBy(Identity))
	assertEqual(t, Some(3), From(3, 1, 2).MaxBy(Identity))
	// tie: first one wins
	assertEqual(t, Some(-3), From(-3, 3, 1).MaxBy(abs))
	assertEqual(t, Some(3), From(1, 3, 2).asSeq().MaxBy(Identity))
}

// TestMinMaxByNaN pins the total order MinBy/MaxBy share with [Flow.SortBy]: a
// NaN key orders below every non-NaN key wherever it appears, so MinBy agrees
// with SortBy's first element and MaxBy with its last.
func TestMinMaxByNaN(t *testing.T) {
	t.Parallel()
	nan := math.NaN()
	same := func(a, b float64) bool { return a == b || (math.IsNaN(a) && math.IsNaN(b)) }
	for _, xs := range [][]float64{{nan, 1, 2}, {1, nan, 2}, {1, 2, nan}, {nan, nan}, {nan}, {1, 2}} {
		sorted := From(xs...).SortBy(Identity)
		gotMin, gotMax := From(xs...).MinBy(Identity), From(xs...).MaxBy(Identity)
		isTrue(t, same(sorted.First().MustGet(), gotMin.MustGet()), fmt.Sprintf("MinBy %v: %v", xs, gotMin))
		isTrue(t, same(sorted.Last().MustGet(), gotMax.MustGet()), fmt.Sprintf("MaxBy %v: %v", xs, gotMax))
	}
}

// TestMinMaxByNaNTie covers what TestMinMaxByNaN cannot: two NaN keys are
// indistinguishable as float64 values, so only a tagged element shows which
// one won.
func TestMinMaxByNaNTie(t *testing.T) {
	t.Parallel()
	type fv struct {
		k float64
		v string
	}
	key := func(x fv) float64 { return x.k }
	nan := math.NaN()
	f := From(fv{nan, "a"}, fv{1, "b"}, fv{nan, "c"})
	assertEqual(t, "a", f.MinBy(key).MustGet().v)
	assertEqual(t, "b", f.MaxBy(key).MustGet().v)
}

func TestChunk(t *testing.T) {
	t.Parallel()
	eq(t, [][]int{}, Chunk[int](3)(From[int]()))
	eq(t, [][]int{{0, 1, 2}, {3, 4}}, Chunk[int](3)(FromFn(5, Identity)))
	eq(t, [][]int{{0}, {1}, {2}}, Chunk[int](1)(FromFn(3, Identity)))
	eq(t, [][]int{{1, 2}, {3}}, Chunk[int](2)(From(1, 2, 3).asSeq()))
	if got, panicked := didPanic(func() { Chunk[int](0) }); !panicked {
		t.Errorf("want fn to panic with %#v, it did not panic", "funq: Chunk called with n < 1")
	} else if got != "funq: Chunk called with n < 1" {
		t.Errorf("want panic value %#v, got %#v", "funq: Chunk called with n < 1", got)
	}

	src := From(1, 2, 3, 4)
	chunked := Chunk[int](2)(src)
	chunks := chunked.Slice()
	chunks[0][0] = 99
	assertEqual(t, []int{1, 2, 3, 4}, src.Slice(), "mutating a chunk must not alias the source Flow")
	assertEqual(t, [][]int{{1, 2}, {3, 4}}, chunked.Slice(), "mutating a chunk must not affect a fresh traversal")

	// First stops after the first full chunk, exercising the
	// early-termination (!yield) branch of Chunk's generator.
	assertEqual(t, Some([]int{0, 1}), Chunk[int](2)(FromFn(6, Identity)).First())
}

func TestZip(t *testing.T) {
	t.Parallel()
	eq(t, []Pair[int, string]{{1, "a"}, {2, "b"}}, Zip(From(1, 2), From("a", "b")))
	// shorter side wins
	eq(t, []Pair[int, string]{{1, "a"}}, Zip(From(1, 2, 3), From("a")))
	eq(t, []Pair[int, string]{}, Zip(From[int](), From("a")))
	eq(t, []Pair[int, int]{{1, 10}, {2, 20}},
		Zip(From(1, 2).asSeq(), From(10, 20, 30).asSeq()))
	// mixed representations: indexed a with sequential b takes the swapped-
	// driver path, so cover both length orderings there and its mirror
	eq(t, []Pair[int, int]{{1, 10}, {2, 20}},
		Zip(From(1, 2), From(10, 20, 30).asSeq()))
	eq(t, []Pair[int, int]{{1, 10}, {2, 20}},
		Zip(From(1, 2, 3), From(10, 20).asSeq()))
	eq(t, []Pair[int, int]{{1, 10}, {2, 20}},
		Zip(From(1, 2).asSeq(), From(10, 20, 30)))
	// First stops after the first pair, exercising the early-termination
	// (!yield) branch of the general (non-swapped-driver) path: both sides
	// sequential, so neither takes the a.at != nil && b.at == nil shortcut.
	assertEqual(t, Some(Pair[int, int]{1, 10}), Zip(From(1, 2).asSeq(), From(10, 20, 30).asSeq()).First())
}
