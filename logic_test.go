package funq

import (
	"testing"
)

func TestNot(t *testing.T) {
	t.Parallel()
	isTrue(t, Not(False[int])(0))
	isFalse(t, Not(True[int])(0))
}

func TestTrue(t *testing.T) {
	t.Parallel()
	isTrue(t, True(0))
}

func TestFalse(t *testing.T) {
	t.Parallel()
	isFalse(t, False(0))
}

func TestAnd(t *testing.T) {
	t.Parallel()
	isTrue(t, And(GreaterThan(0), even)(2))
	isFalse(t, And(GreaterThan(0), even)(0))
	isFalse(t, And(GreaterThan(0), even)(-2))
}

func TestOr(t *testing.T) {
	t.Parallel()
	isTrue(t, Or(GreaterThan(0), even)(1))
	isTrue(t, Or(GreaterThan(0), even)(-2))
	isFalse(t, Or(GreaterThan(0), even)(-1))
}

func TestXor(t *testing.T) {
	t.Parallel()
	isTrue(t, Xor(GreaterThan(0), even)(0))
	isTrue(t, Xor(GreaterThan(0), even)(3))
	isFalse(t, Xor(GreaterThan(0), even)(2))
	isFalse(t, Xor(GreaterThan(0), even)(-3))
}

func TestNand(t *testing.T) {
	t.Parallel()
	isPositive := func(n int) bool { return n > 0 }
	isEven := func(n int) bool { return n%2 == 0 }
	isFalse(t, Nand(isPositive, isEven)(2))
	isTrue(t, Nand(isPositive, isEven)(3))
}

func TestNor(t *testing.T) {
	t.Parallel()
	isPositive := func(n int) bool { return n > 0 }
	isEven := func(n int) bool { return n%2 == 0 }
	isTrue(t, Nor(isPositive, isEven)(-3))
	isFalse(t, Nor(isPositive, isEven)(3))
	isFalse(t, Nor(isPositive, isEven)(-2))
	isFalse(t, Nor(isPositive, isEven)(2))
}

func TestXnor(t *testing.T) {
	t.Parallel()
	isPositive := func(n int) bool { return n > 0 }
	isEven := func(n int) bool { return n%2 == 0 }
	isTrue(t, Xnor(isPositive, isEven)(2))
	isFalse(t, Xnor(isPositive, isEven)(3))
	isFalse(t, Xnor(isPositive, isEven)(-2))
	isTrue(t, Xnor(isPositive, isEven)(-3))
}

func TestImplies(t *testing.T) {
	t.Parallel()
	isPositive := func(n int) bool { return n > 0 }
	isEven := func(n int) bool { return n%2 == 0 }
	isTrue(t, Implies(isPositive, isEven)(2))
	isFalse(t, Implies(isPositive, isEven)(3))
	isTrue(t, Implies(isPositive, isEven)(-2))
	isTrue(t, Implies(isPositive, isEven)(-3))
}
