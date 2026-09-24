package funq

import (
	"math/rand/v2"
	"reflect"
	"testing"
)

type diffOp struct {
	name  string
	apply func(Flow[int]) Flow[int]
}

// diffOps are the compositions FuzzDifferentialIndexedVsSequential draws
// from. Constants inside them are picked against the src domain the fuzz
// target decodes (0..9) so each predicate both accepts and rejects part of a
// typical source.
var diffOps = []diffOp{
	{"Filter(even)", func(f Flow[int]) Flow[int] { return f.Filter(even) }},
	{"Filter(>3)", func(f Flow[int]) Flow[int] { return f.Filter(GreaterThan(3)) }},
	{"Map(+1)", func(f Flow[int]) Flow[int] { return f.Map(add1) }},
	{"Take(3)", func(f Flow[int]) Flow[int] { return f.Take(3) }},
	{"Take(0)", func(f Flow[int]) Flow[int] { return f.Take(0) }},
	{"Drop(2)", func(f Flow[int]) Flow[int] { return f.Drop(2) }},
	{"Drop(100)", func(f Flow[int]) Flow[int] { return f.Drop(100) }},
	{"TakeWhile(<5)", func(f Flow[int]) Flow[int] { return f.TakeWhile(LessThan(5)) }},
	{"DropWhile(<5)", func(f Flow[int]) Flow[int] { return f.DropWhile(LessThan(5)) }},
	{"Reverse", func(f Flow[int]) Flow[int] { return f.Reverse() }},
	{"SortBy(identity)", func(f Flow[int]) Flow[int] { return f.SortBy(Identity) }},
	{"Concat(2,empty)", func(f Flow[int]) Flow[int] { return f.Concat(From(7, 8), From[int]()) }},
	{"DistinctBy(%3)", func(f Flow[int]) Flow[int] { return f.DistinctBy(func(v int) int { return v % 3 }) }},
}

// diffMaxSrcLen and diffMaxOpLen bound what the fuzz target decodes from
// srcBytes and opBytes, matching the domain the seed corpus is drawn from.
const (
	diffMaxSrcLen = 8
	diffMaxOpLen  = 4
)

// The seeds are fixed so plain go test runs a deterministic seed corpus;
// diffSeedIters is capped to keep that run under 0.1s with -race.
const (
	diffSeedIters = 1500
	diffSeed1     = 0x5EED
	diffSeed2     = 0xF10C
)

// FuzzDifferentialIndexedVsSequential asserts that a Flow's two internal
// representations agree under composed operations.
//
// It guards the hole-free invariant on sizeUnknown (see the const in flow.go):
// a composition that carries a stale size across a bounds adjustment would not
// error but silently emit trailing zero values from Slice's known-size path.
// Sequential Flows carry sizeUnknown and always traverse, making them the
// oracle. Per-method tests already pair an indexed case with an asSeq one.
// Composing those methods is what they leave uncovered.
//
// srcBytes and opBytes encode the source elements and the op sequence: each
// srcBytes entry mod 10 is one element, capped at diffMaxSrcLen elements; each
// opBytes entry mod len(diffOps) selects an op, capped at diffMaxOpLen ops.
// The seed corpus is diffSeedIters random compositions plus one single-op
// seed per diffOps entry, so every op is seeded however diffOps grows.
func FuzzDifferentialIndexedVsSequential(f *testing.F) {
	r := rand.New(rand.NewPCG(diffSeed1, diffSeed2))
	for range diffSeedIters {
		src := make([]byte, r.IntN(diffMaxSrcLen+1))
		for i := range src {
			src[i] = byte(r.IntN(10))
		}
		ops := make([]byte, 1+r.IntN(diffMaxOpLen))
		for i := range ops {
			ops[i] = byte(r.IntN(len(diffOps)))
		}
		f.Add(src, ops)
	}
	for i := range diffOps {
		f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7}, []byte{byte(i)})
	}

	f.Fuzz(func(t *testing.T, srcBytes, opBytes []byte) {
		if len(srcBytes) > diffMaxSrcLen {
			srcBytes = srcBytes[:diffMaxSrcLen]
		}
		if len(opBytes) == 0 {
			return
		}
		if len(opBytes) > diffMaxOpLen {
			opBytes = opBytes[:diffMaxOpLen]
		}

		src := make([]int, len(srcBytes))
		for i, v := range srcBytes {
			src[i] = int(v) % 10
		}

		a := From(src...)
		b := From(src...).asSeq()
		names := make([]string, 0, len(opBytes))
		for _, v := range opBytes {
			o := diffOps[int(v)%len(diffOps)]
			names = append(names, o.name)
			// asSeq is reapplied every step because SortBy and Reverse
			// materialize via fromSlice: a single leading one would let b
			// collapse into a's representation and stop being a differential.
			a, b = o.apply(a), o.apply(b.asSeq())
		}

		mismatch := func(what string, indexed, sequential any) {
			t.Helper()
			t.Fatalf("%s mismatch: src=%v ops=%v indexed=%v sequential=%v", what, src, names, indexed, sequential)
		}

		ga, gb := a.Slice(), b.Slice()
		if !reflect.DeepEqual(ga, gb) {
			mismatch("Slice", ga, gb)
		}
		// Checked against its own Slice length, not just the other
		// representation: Count returns a known size without traversing.
		if ca, cb := a.Count(), b.Count(); ca != len(ga) || cb != len(gb) {
			mismatch("Count", ca, cb)
		}
		if ea, eb := a.IsEmpty(), b.IsEmpty(); ea != (len(ga) == 0) || eb != (len(gb) == 0) {
			mismatch("IsEmpty", ea, eb)
		}
		if fa, fb := a.First(), b.First(); !reflect.DeepEqual(fa, fb) {
			mismatch("First", fa, fb)
		}
		// Last is the only method running a different algorithm per
		// representation (Reverse().First() when indexed, a full scan
		// otherwise), so it can disagree on its own.
		if la, lb := a.Last(), b.Last(); !reflect.DeepEqual(la, lb) {
			mismatch("Last", la, lb)
		}
	})
}
