package funq

import (
	"cmp"
	"math"
	"testing"
)

func TestLessThan(t *testing.T) {
	t.Parallel()
	isTrue(t, LessThan("c")("a"))
	isFalse(t, LessThan("c")("c"))
}

func TestGreaterThan(t *testing.T) {
	t.Parallel()
	isTrue(t, GreaterThan("c")("cc"))
	isFalse(t, GreaterThan("c")("c"))
	isFalse(t, GreaterThan("c")("a"))
}

func TestAtMost(t *testing.T) {
	t.Parallel()
	isTrue(t, AtMost("c")("a"))
	isTrue(t, AtMost("c")("c"))
	isFalse(t, AtMost("c")("cc"))
}

func TestAtLeast(t *testing.T) {
	t.Parallel()
	isTrue(t, AtLeast("c")("cc"))
	isTrue(t, AtLeast("c")("c"))
	isFalse(t, AtLeast("c")("a"))
}

func TestIs(t *testing.T) {
	t.Parallel()
	isFalse(t, Equal(1)(0))
	isTrue(t, Equal(1)(1))
}

func TestBetween(t *testing.T) {
	t.Parallel()
	isTrue(t, Between(5, 10)(5))
	isTrue(t, Between(5, 10)(10))
	isTrue(t, Between(10, 5)(7))
	isFalse(t, Between(5, 10)(4))

	t.Run("NaN", func(t *testing.T) {
		t.Parallel()
		isFalse(t, Between(0.0, 10.0)(math.NaN()))
		isFalse(t, Between(math.NaN(), 10.0)(5.0))
		isFalse(t, Between(0.0, math.NaN())(5.0))
		isFalse(t, Between(math.NaN(), math.NaN())(math.NaN()))
	})

	t.Run("single point", func(t *testing.T) {
		t.Parallel()
		isTrue(t, Between(5, 5)(5))
		isFalse(t, Between(5, 5)(4))
		isTrue(t, Between("c", "c")("c"))
		isFalse(t, Between("c", "c")("cc"))

		negZero := math.Copysign(0, -1)
		isTrue(t, Between(negZero, 0.0)(0.0))
		isTrue(t, Between(negZero, 0.0)(negZero))
		isFalse(t, Between(negZero, 0.0)(1.0))
	})
}

// betweenBounds implements Between's single-point case as the unconditional
// two-bound comparison Between used before it routed x==y to Equal (see
// Between's implementation comment). It exists only for BenchmarkBetween to
// measure against.
func betweenBounds[T cmp.Ordered](x, y T) Fp[T, bool] {
	lo, hi := min(x, y), max(x, y)
	return func(t T) bool { return lo <= t && t <= hi }
}

// BenchmarkBetween measures the single-point (x==y) case's Equal fastpath
// against the two-bound comparison it replaces, for both types named in
// Between's implementation comment. Absolute ns/op vary by run and machine,
// so only the relative ordering below should be relied on.
//
// Result: fastpath wins both types. For string it wins clearly: one memequal
// against two cmpstring calls. For float64 it wins only slightly: one FCMPD
// against two — close enough that a run under different load can
// occasionally show the two within noise of each other.
func BenchmarkBetween(b *testing.B) {
	b.Run("float64/equal_fastpath", func(b *testing.B) {
		pred := Between(5.0, 5.0)
		for b.Loop() {
			_ = pred(5.0)
		}
	})
	b.Run("float64/two_bounds", func(b *testing.B) {
		pred := betweenBounds(5.0, 5.0)
		for b.Loop() {
			_ = pred(5.0)
		}
	})
	b.Run("string/equal_fastpath", func(b *testing.B) {
		pred := Between("needle", "needle")
		for b.Loop() {
			_ = pred("needle")
		}
	})
	b.Run("string/two_bounds", func(b *testing.B) {
		pred := betweenBounds("needle", "needle")
		for b.Loop() {
			_ = pred("needle")
		}
	})
}

func TestOneOf(t *testing.T) {
	t.Parallel()
	isTrue(t, OneOf(1, 3, 5)(3))
	isFalse(t, OneOf(1, 3, 5)(2))
	isFalse(t, OneOf[int]()(0))
	isTrue(t, OneOf(1)(1))
	isFalse(t, OneOf(1)(2))
}
